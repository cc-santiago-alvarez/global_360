// Command api starts the Global 360 HTTP API. It is the composition root:
// config -> logger -> MongoDB -> repositories -> services -> HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"global_360/internal/application/service"
	"global_360/internal/infrastructure/config"
	"global_360/internal/infrastructure/httpapi"
	"global_360/internal/infrastructure/httpapi/handler"
	"global_360/internal/infrastructure/logger"
	"global_360/internal/infrastructure/persistence/mongodb"
	"global_360/internal/infrastructure/security"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	log := logger.New(cfg.LogLevel)
	slog.SetDefault(log)
	gin.SetMode(cfg.GinMode)

	startCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store, err := mongodb.Connect(startCtx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = store.Close(ctx)
	}()
	if store.SupportsTransactions {
		log.Info("mongodb connected", "database", cfg.MongoDatabase, "transactions", true)
	} else {
		log.Warn("mongodb is a standalone server: multi-document transactions are disabled; entity and audit writes are sequential",
			"database", cfg.MongoDatabase)
	}
	if err := mongodb.EnsureSchema(startCtx, store); err != nil {
		return err
	}

	// Driven adapters
	clock := security.SystemClock{}
	ids := security.UUIDGenerator{}
	hasher := security.NewBcryptHasher(cfg.BcryptCost)
	issuer := security.NewJWTIssuer(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL)

	countries := mongodb.NewCountryRepository(store)
	currencies := mongodb.NewCurrencyRepository(store)
	persons := mongodb.NewPersonRepository(store)
	companies := mongodb.NewCompanyRepository(store)
	users := mongodb.NewUserRepository(store)
	tokens := mongodb.NewRefreshTokenRepository(store)
	permissions := mongodb.NewPermissionRepository(store)
	roles := mongodb.NewRoleRepository(store)
	assignments := mongodb.NewRoleAssignmentRepository(store)
	auditLogs := mongodb.NewAuditLogRepository(store)
	categories := mongodb.NewCategoryRepository(store)
	profiles := mongodb.NewDistributorProfileRepository(store)
	distributorServices := mongodb.NewDistributorServiceRepository(store)
	contactRequests := mongodb.NewContactRequestRepository(store)

	if err := mongodb.NewSeeder(store, hasher, ids, clock, log).Run(startCtx, mongodb.SeedConfig{
		SuperadminEmail: cfg.SeedSuperadminEmail, SuperadminPassword: cfg.SeedSuperadminPassword,
	}); err != nil {
		return err
	}

	// Use cases
	auditSvc := service.NewAuditService(auditLogs, clock)
	deps := service.Deps{Tx: mongodb.NewTxManager(store), Clock: clock, IDs: ids, Audit: auditSvc}
	authz := service.NewAuthorizationService(assignments, roles)
	authSvc, err := service.NewAuthService(users, persons, tokens, roles, hasher, issuer, security.RandomSecrets{}, authz, deps,
		service.AuthConfig{MaxFailedLoginAttempts: cfg.MaxFailedLoginAttempts, RefreshTokenTTL: cfg.RefreshTokenTTL})
	if err != nil {
		return err
	}
	handlers := &handler.Handlers{
		Auth:      authSvc,
		Users:     service.NewUserService(users, persons, companies, countries, roles, assignments, tokens, hasher, deps),
		Companies: service.NewCompanyService(companies, countries, currencies, profiles, deps),
		Roles:     service.NewRoleService(roles, permissions, deps),
		Catalog:   service.NewCatalogService(countries, currencies, deps),
		Audit:     auditSvc,
		DB:        store,

		Categories:   service.NewCategoryService(categories, deps),
		Marketplace:  service.NewMarketplaceService(profiles, distributorServices, categories, countries),
		Distributors: service.NewDistributorProfileService(profiles, distributorServices, companies, categories, countries, deps),
		ContactRequests: service.NewContactRequestService(contactRequests, profiles, distributorServices, companies,
			users, persons, deps),
	}

	srv := httpapi.NewServer(":"+cfg.Port, httpapi.NewRouter(handlers, log))
	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for Ctrl+C / SIGTERM (or a server error) and shut down gracefully.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-quit:
	}
	log.Info("shutting down server")
	ctx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("server stopped")
	return nil
}
