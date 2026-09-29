// Package mongodb implements the persistence ports on MongoDB.
package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Collection names.
const (
	CollCountries       = "countries"
	CollCurrencies      = "currencies"
	CollPersons         = "persons"
	CollCompanies       = "companies"
	CollUsers           = "users"
	CollRoles           = "roles"
	CollPermissions     = "permissions"
	CollRoleAssignments = "role_assignments"
	CollAuditLogs       = "audit_logs"
	CollRefreshTokens   = "refresh_tokens"

	CollCategories          = "categories"
	CollDistributorProfiles = "distributor_profiles"
	CollDistributorServices = "distributor_services"
	CollContactRequests     = "contact_requests"
	CollSchemaMigrations    = "schema_migrations"
)

// Store wraps the client and database.
type Store struct {
	Client *mongo.Client
	DB     *mongo.Database
	// SupportsTransactions is true when connected to a replica set or sharded cluster.
	SupportsTransactions bool
}

// Connect opens the connection, verifies it and detects transaction support.
func Connect(ctx context.Context, uri, database string) (*Store, error) {
	opts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetBSONOptions(&options.BSONOptions{DefaultDocumentM: true})
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}
	s := &Store{Client: client, DB: client.Database(database)}

	var hello bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongodb hello: %w", err)
	}
	_, isReplicaSet := hello["setName"]
	s.SupportsTransactions = isReplicaSet || hello["msg"] == "isdbgrid"
	return s, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.Client.Ping(ctx, readpref.Primary()) }

func (s *Store) Close(ctx context.Context) error { return s.Client.Disconnect(ctx) }

func (s *Store) coll(name string) *mongo.Collection { return s.DB.Collection(name) }

// TxManager implements port.TxManager with MongoDB sessions. On standalone servers
// (no transaction support) it runs the function directly.
type TxManager struct {
	store *Store
}

func NewTxManager(store *Store) *TxManager { return &TxManager{store: store} }

func (m *TxManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if !m.store.SupportsTransactions || mongo.SessionFromContext(ctx) != nil {
		return fn(ctx)
	}
	sess, err := m.store.Client.StartSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer sess.EndSession(context.Background())
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	})
	return err
}
