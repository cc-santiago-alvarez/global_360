package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
)

// Refresh tokens are kept 30 days after expiring for forensic purposes, then purged by TTL.
const refreshTokenRetentionSeconds = 30 * 24 * 3600

// collectionSpec is the database-side contract of a collection: a $jsonSchema
// validator (equivalent to SQL NOT NULL/CHECK constraints) plus its indexes.
type collectionSpec struct {
	name      string
	validator bson.M
	indexes   []mongo.IndexModel
}

// EnsureSchema creates or updates every collection validator and index. It is idempotent.
func EnsureSchema(ctx context.Context, s *Store) error {
	existing, err := s.DB.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, n := range existing {
		have[n] = true
	}
	for _, spec := range collectionSpecs() {
		if have[spec.name] {
			cmd := bson.D{
				{Key: "collMod", Value: spec.name},
				{Key: "validator", Value: spec.validator},
				{Key: "validationLevel", Value: "strict"},
				{Key: "validationAction", Value: "error"},
			}
			if err := s.DB.RunCommand(ctx, cmd).Err(); err != nil {
				return fmt.Errorf("update validator of %s: %w", spec.name, err)
			}
		} else {
			opts := options.CreateCollection().
				SetValidator(spec.validator).
				SetValidationLevel("strict").
				SetValidationAction("error")
			if err := s.DB.CreateCollection(ctx, spec.name, opts); err != nil {
				return fmt.Errorf("create collection %s: %w", spec.name, err)
			}
		}
		if len(spec.indexes) > 0 {
			if _, err := s.coll(spec.name).Indexes().CreateMany(ctx, spec.indexes); err != nil {
				return fmt.Errorf("create indexes of %s: %w", spec.name, err)
			}
		}
	}
	return nil
}

func collectionSpecs() []collectionSpec {
	return append(foundationSpecs(), commerceSpecs()...)
}

func foundationSpecs() []collectionSpec {
	return []collectionSpec{
		{
			name: CollCountries,
			validator: schema([]string{"_id", "iso_code", "name", "active", "created_at"}, bson.M{
				"_id":        str(),
				"iso_code":   pattern(`^[A-Z]{2}$`),
				"name":       str(),
				"active":     boolean(),
				"created_at": date(),
			}),
			indexes: []mongo.IndexModel{unique("uq_countries_iso_code", bson.D{{Key: "iso_code", Value: 1}})},
		},
		{
			name: CollCurrencies,
			validator: schema([]string{"_id", "iso_code", "name", "symbol", "decimals", "active", "created_at"}, bson.M{
				"_id":        str(),
				"iso_code":   pattern(`^[A-Z]{3}$`),
				"name":       str(),
				"symbol":     str(),
				"decimals":   integer(0),
				"active":     boolean(),
				"created_at": date(),
			}),
			indexes: []mongo.IndexModel{unique("uq_currencies_iso_code", bson.D{{Key: "iso_code", Value: 1}})},
		},
		{
			name: CollPersons,
			validator: schema([]string{"_id", "document_type", "document_number", "document_country_id", "first_names", "last_names", "email", "phone", "created_at", "updated_at"}, bson.M{
				"_id":                 str(),
				"document_type":       str(),
				"document_number":     str(),
				"document_country_id": str(),
				"first_names":         str(),
				"last_names":          str(),
				"email":               str(),
				"phone":               nullable("string"),
				"created_at":          date(),
				"updated_at":          date(),
			}),
			indexes: []mongo.IndexModel{unique("uq_persons_document", bson.D{
				{Key: "document_type", Value: 1}, {Key: "document_number", Value: 1}, {Key: "document_country_id", Value: 1},
			})},
		},
		{
			name: CollCompanies,
			validator: schema([]string{"_id", "legal_name", "trade_name", "document_type", "document_number", "country_id", "billing_currency_id", "company_type", "status", "contact_email", "contact_phone", "created_at", "updated_at"}, bson.M{
				"_id":                 str(),
				"legal_name":          str(),
				"trade_name":          nullable("string"),
				"document_type":       str(),
				"document_number":     str(),
				"country_id":          str(),
				"billing_currency_id": str(),
				"company_type":        enum(company.Types),
				"status":              enum(company.Statuses),
				"contact_email":       nullable("string"),
				"contact_phone":       nullable("string"),
				"created_at":          date(),
				"updated_at":          date(),
			}),
			indexes: []mongo.IndexModel{
				unique("uq_companies_document", bson.D{
					{Key: "document_type", Value: 1}, {Key: "document_number", Value: 1}, {Key: "country_id", Value: 1},
				}),
				index("idx_companies_country", bson.D{{Key: "country_id", Value: 1}}),
				index("idx_companies_status", bson.D{{Key: "status", Value: 1}}),
			},
		},
		{
			name: CollUsers,
			validator: schema([]string{"_id", "person_id", "company_id", "email", "password_hash", "status", "mfa_enabled", "failed_login_attempts", "last_access_at", "created_at", "updated_at"}, bson.M{
				"_id":                   str(),
				"person_id":             str(),
				"company_id":            nullable("string"),
				"email":                 str(),
				"password_hash":         str(),
				"status":                enum(identity.UserStatuses),
				"mfa_enabled":           boolean(),
				"failed_login_attempts": integer(0),
				"last_access_at":        nullable("date"),
				"created_at":            date(),
				"updated_at":            date(),
			}),
			indexes: []mongo.IndexModel{
				unique("uq_users_email", bson.D{{Key: "email", Value: 1}}),
				index("idx_users_company", bson.D{{Key: "company_id", Value: 1}}),
				index("idx_users_person", bson.D{{Key: "person_id", Value: 1}}),
			},
		},
		{
			name: CollPermissions,
			validator: schema([]string{"_id", "code", "module", "description"}, bson.M{
				"_id":         str(),
				"code":        pattern(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`),
				"module":      str(),
				"description": bson.M{"bsonType": "string"},
			}),
			indexes: []mongo.IndexModel{unique("uq_permissions_code", bson.D{{Key: "code", Value: 1}})},
		},
		{
			name: CollRoles,
			validator: schema([]string{"_id", "name", "description", "scope", "is_system", "active", "permissions", "created_at", "updated_at"}, bson.M{
				"_id":         str(),
				"name":        str(),
				"description": nullable("string"),
				"scope":       enum(access.Scopes),
				"is_system":   boolean(),
				"active":      boolean(),
				"permissions": bson.M{"bsonType": "array", "items": bson.M{"bsonType": "string"}},
				"created_at":  date(),
				"updated_at":  date(),
			}),
			indexes: []mongo.IndexModel{unique("uq_roles_name", bson.D{{Key: "name", Value: 1}})},
		},
		{
			name: CollRoleAssignments,
			validator: schema([]string{"_id", "user_id", "role_id", "company_id", "active", "assigned_at", "assigned_by", "revoked_at", "revoked_by"}, bson.M{
				"_id":         str(),
				"user_id":     str(),
				"role_id":     str(),
				"company_id":  nullable("string"),
				"active":      boolean(),
				"assigned_at": date(),
				"assigned_by": nullable("string"),
				"revoked_at":  nullable("date"),
				"revoked_by":  nullable("string"),
			}),
			indexes: []mongo.IndexModel{
				// One active assignment per (user, role, company); company_id null = global.
				{
					Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "role_id", Value: 1}, {Key: "company_id", Value: 1}},
					Options: options.Index().SetName("uq_role_assignments_active").SetUnique(true).
						SetPartialFilterExpression(bson.M{"active": true}),
				},
				index("idx_role_assignments_user", bson.D{{Key: "user_id", Value: 1}, {Key: "active", Value: 1}}),
				index("idx_role_assignments_company", bson.D{{Key: "company_id", Value: 1}}),
			},
		},
		{
			name: CollAuditLogs,
			validator: schema([]string{"_id", "user_id", "entity", "entity_id", "action", "old_values", "new_values", "source_ip", "user_agent", "created_at"}, bson.M{
				"_id":        bson.M{"bsonType": "objectId"},
				"user_id":    nullable("string"),
				"entity":     str(),
				"entity_id":  str(),
				"action":     enum(audit.Actions),
				"old_values": nullable("object"),
				"new_values": nullable("object"),
				"source_ip":  nullable("string"),
				"user_agent": nullable("string"),
				"created_at": date(),
			}),
			indexes: []mongo.IndexModel{
				index("idx_audit_entity", bson.D{{Key: "entity", Value: 1}, {Key: "entity_id", Value: 1}, {Key: "created_at", Value: -1}}),
				index("idx_audit_user_date", bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}),
				index("idx_audit_date", bson.D{{Key: "created_at", Value: -1}}),
			},
		},
		{
			name: CollRefreshTokens,
			validator: schema([]string{"_id", "user_id", "token_hash", "expires_at", "revoked_at", "created_at"}, bson.M{
				"_id":        str(),
				"user_id":    str(),
				"token_hash": str(),
				"expires_at": date(),
				"revoked_at": nullable("date"),
				"created_at": date(),
				"user_agent": bson.M{"bsonType": "string"},
				"ip":         bson.M{"bsonType": "string"},
			}),
			indexes: []mongo.IndexModel{
				unique("uq_refresh_tokens_hash", bson.D{{Key: "token_hash", Value: 1}}),
				index("idx_refresh_tokens_user", bson.D{{Key: "user_id", Value: 1}}),
				{
					Keys:    bson.D{{Key: "expires_at", Value: 1}},
					Options: options.Index().SetName("ttl_refresh_tokens_expires").SetExpireAfterSeconds(refreshTokenRetentionSeconds),
				},
			},
		},
	}
}

// commerceSpecs are the phase 2 (marketplace) collections.
func commerceSpecs() []collectionSpec {
	stringArray := bson.M{"bsonType": "array", "items": bson.M{"bsonType": "string", "minLength": 1}}
	return []collectionSpec{
		{
			name: CollCategories,
			validator: schema([]string{"_id", "code", "name", "description", "active", "created_at", "updated_at"}, bson.M{
				"_id":         str(),
				"code":        pattern(`^[a-z][a-z0-9_]{2,49}$`),
				"name":        str(),
				"description": nullable("string"),
				"active":      boolean(),
				"created_at":  date(),
				"updated_at":  date(),
			}),
			indexes: []mongo.IndexModel{unique("uq_categories_code", bson.D{{Key: "code", Value: 1}})},
		},
		{
			// _id is the company id: the profile extends the company (1:1).
			name: CollDistributorProfiles,
			validator: schema([]string{
				"_id", "country_id", "display_name", "summary", "description", "logo_url", "website_url",
				"category_ids", "coverage_country_ids", "contact_email", "contact_phone", "whatsapp",
				"company_eligible", "status", "status_reason", "submitted_at", "reviewed_at", "reviewed_by",
				"published_at", "created_at", "updated_at",
			}, bson.M{
				"_id":                  str(),
				"country_id":           str(),
				"display_name":         str(),
				"summary":              bson.M{"bsonType": "string"},
				"description":          bson.M{"bsonType": "string"},
				"logo_url":             nullable("string"),
				"website_url":          nullable("string"),
				"category_ids":         stringArray,
				"coverage_country_ids": stringArray,
				"contact_email":        nullable("string"),
				"contact_phone":        nullable("string"),
				"whatsapp":             nullable("string"),
				"company_eligible":     boolean(),
				"status":               enum(commerce.ProfileStatuses),
				"status_reason":        nullable("string"),
				"submitted_at":         nullable("date"),
				"reviewed_at":          nullable("date"),
				"reviewed_by":          nullable("string"),
				"published_at":         nullable("date"),
				"created_at":           date(),
				"updated_at":           date(),
			}),
			indexes: []mongo.IndexModel{
				index("idx_profiles_listing", bson.D{
					{Key: "status", Value: 1}, {Key: "company_eligible", Value: 1}, {Key: "published_at", Value: -1},
				}),
				index("idx_profiles_categories", bson.D{{Key: "category_ids", Value: 1}}),
				index("idx_profiles_coverage", bson.D{{Key: "coverage_country_ids", Value: 1}}),
				{
					// Marketplace search. Spanish stemming; diacritic and case insensitive.
					Keys: bson.D{{Key: "display_name", Value: "text"}, {Key: "summary", Value: "text"}, {Key: "description", Value: "text"}},
					Options: options.Index().SetName("txt_profiles_search").
						SetWeights(bson.D{{Key: "display_name", Value: 10}, {Key: "summary", Value: 5}, {Key: "description", Value: 1}}).
						SetDefaultLanguage("spanish").SetLanguageOverride("text_language"),
				},
			},
		},
		{
			name: CollDistributorServices,
			validator: schema([]string{"_id", "company_id", "category_id", "name", "description", "origin_country_id", "destination_country_id", "active", "created_at", "updated_at"}, bson.M{
				"_id":                    str(),
				"company_id":             str(),
				"category_id":            str(),
				"name":                   str(),
				"description":            nullable("string"),
				"origin_country_id":      nullable("string"),
				"destination_country_id": nullable("string"),
				"active":                 boolean(),
				"created_at":             date(),
				"updated_at":             date(),
			}),
			indexes: []mongo.IndexModel{
				unique("uq_distributor_services_name", bson.D{{Key: "company_id", Value: 1}, {Key: "name", Value: 1}}),
				index("idx_distributor_services_company", bson.D{{Key: "company_id", Value: 1}, {Key: "active", Value: 1}}),
			},
		},
		{
			name: CollContactRequests,
			validator: schema([]string{
				"_id", "distributor_id", "distributor_name", "service_id", "requester_company_id", "requester_company_name",
				"requester_user_id", "contact_name", "contact_email", "contact_phone", "message", "status",
				"distributor_notes", "created_at", "updated_at",
			}, bson.M{
				"_id":                    str(),
				"distributor_id":         str(),
				"distributor_name":       str(),
				"service_id":             nullable("string"),
				"requester_company_id":   str(),
				"requester_company_name": str(),
				"requester_user_id":      str(),
				"contact_name":           str(),
				"contact_email":          str(),
				"contact_phone":          nullable("string"),
				"message":                str(),
				"status":                 enum(commerce.ContactRequestStatuses),
				"distributor_notes":      nullable("string"),
				"created_at":             date(),
				"updated_at":             date(),
			}),
			indexes: []mongo.IndexModel{
				index("idx_contact_requests_distributor", bson.D{{Key: "distributor_id", Value: 1}, {Key: "created_at", Value: -1}}),
				index("idx_contact_requests_requester", bson.D{{Key: "requester_company_id", Value: 1}, {Key: "created_at", Value: -1}}),
				{
					// One open ("new") request per user and distributor: prevents spam.
					Keys: bson.D{{Key: "requester_user_id", Value: 1}, {Key: "distributor_id", Value: 1}},
					Options: options.Index().SetName("uq_contact_requests_open").SetUnique(true).
						SetPartialFilterExpression(bson.M{"status": string(commerce.ContactRequestNew)}),
				},
			},
		},
		{
			name: CollSchemaMigrations,
			validator: schema([]string{"_id", "applied_at"}, bson.M{
				"_id":        str(),
				"applied_at": date(),
			}),
		},
	}
}

// --- $jsonSchema helpers ---

func schema(required []string, props bson.M) bson.M {
	return bson.M{"$jsonSchema": bson.M{
		"bsonType":   "object",
		"required":   required,
		"properties": props,
	}}
}

// str is a required, non-empty string.
func str() bson.M { return bson.M{"bsonType": "string", "minLength": 1} }

func pattern(re string) bson.M { return bson.M{"bsonType": "string", "pattern": re} }

func boolean() bson.M { return bson.M{"bsonType": "bool"} }

func date() bson.M { return bson.M{"bsonType": "date"} }

func integer(min int) bson.M { return bson.M{"bsonType": bson.A{"int", "long"}, "minimum": min} }

func nullable(bsonType string) bson.M { return bson.M{"bsonType": bson.A{bsonType, "null"}} }

func enum[T ~string](values []T) bson.M {
	vals := make(bson.A, 0, len(values))
	for _, v := range values {
		vals = append(vals, string(v))
	}
	return bson.M{"bsonType": "string", "enum": vals}
}

func unique(name string, keys bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys, Options: options.Index().SetName(name).SetUnique(true)}
}

func index(name string, keys bson.D) mongo.IndexModel {
	return mongo.IndexModel{Keys: keys, Options: options.Index().SetName(name)}
}
