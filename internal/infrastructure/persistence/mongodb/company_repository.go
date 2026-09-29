package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"global_360/internal/application/port"
	"global_360/internal/domain/company"
)

type companyDoc struct {
	ID                string    `bson:"_id"`
	LegalName         string    `bson:"legal_name"`
	TradeName         *string   `bson:"trade_name"`
	DocumentType      string    `bson:"document_type"`
	DocumentNumber    string    `bson:"document_number"`
	CountryID         string    `bson:"country_id"`
	BillingCurrencyID string    `bson:"billing_currency_id"`
	CompanyType       string    `bson:"company_type"`
	Status            string    `bson:"status"`
	ContactEmail      *string   `bson:"contact_email"`
	ContactPhone      *string   `bson:"contact_phone"`
	CreatedAt         time.Time `bson:"created_at"`
	UpdatedAt         time.Time `bson:"updated_at"`
}

func toCompanyDoc(c *company.Company) companyDoc {
	return companyDoc{
		ID: c.ID, LegalName: c.LegalName, TradeName: c.TradeName, DocumentType: c.DocumentType, DocumentNumber: c.DocumentNumber,
		CountryID: c.CountryID, BillingCurrencyID: c.BillingCurrencyID, CompanyType: string(c.Type), Status: string(c.Status),
		ContactEmail: c.ContactEmail, ContactPhone: c.ContactPhone, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func (d *companyDoc) toDomain() *company.Company {
	return &company.Company{
		ID: d.ID, LegalName: d.LegalName, TradeName: d.TradeName, DocumentType: d.DocumentType, DocumentNumber: d.DocumentNumber,
		CountryID: d.CountryID, BillingCurrencyID: d.BillingCurrencyID, Type: company.Type(d.CompanyType),
		Status: company.Status(d.Status), ContactEmail: d.ContactEmail, ContactPhone: d.ContactPhone,
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type CompanyRepository struct{ s *Store }

func NewCompanyRepository(s *Store) *CompanyRepository { return &CompanyRepository{s: s} }

const companyConflict = "a company with this document already exists in this country"

func (r *CompanyRepository) Create(ctx context.Context, c *company.Company) error {
	return insertOne(ctx, r.s.coll(CollCompanies), toCompanyDoc(c), companyConflict)
}

func (r *CompanyRepository) Update(ctx context.Context, c *company.Company) error {
	return replaceByID(ctx, r.s.coll(CollCompanies), c.ID, toCompanyDoc(c), companyConflict, "company not found")
}

func (r *CompanyRepository) FindByID(ctx context.Context, id string) (*company.Company, error) {
	d, err := findOne[companyDoc](ctx, r.s.coll(CollCompanies), bson.M{"_id": id}, "company not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *CompanyRepository) List(ctx context.Context, f port.CompanyFilter) ([]*company.Company, int64, error) {
	filter := bson.M{}
	if f.RestrictToIDs {
		filter["_id"] = bson.M{"$in": nonNil(f.IDs)}
	}
	if f.Status != nil {
		filter["status"] = string(*f.Status)
	}
	if f.Type != nil {
		filter["company_type"] = string(*f.Type)
	}
	docs, total, err := findPage[companyDoc](ctx, r.s.coll(CollCompanies), filter, bson.D{{Key: "legal_name", Value: 1}}, f.Page)
	if err != nil {
		return nil, 0, err
	}
	return mapAll(docs, (*companyDoc).toDomain), total, nil
}
