package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"global_360/internal/domain/catalog"
)

type countryDoc struct {
	ID        string    `bson:"_id"`
	ISOCode   string    `bson:"iso_code"`
	Name      string    `bson:"name"`
	Active    bool      `bson:"active"`
	CreatedAt time.Time `bson:"created_at"`
}

func toCountryDoc(c *catalog.Country) countryDoc {
	return countryDoc{ID: c.ID, ISOCode: c.ISOCode, Name: c.Name, Active: c.Active, CreatedAt: c.CreatedAt}
}

func (d *countryDoc) toDomain() *catalog.Country {
	return &catalog.Country{ID: d.ID, ISOCode: d.ISOCode, Name: d.Name, Active: d.Active, CreatedAt: d.CreatedAt.UTC()}
}

type CountryRepository struct{ s *Store }

func NewCountryRepository(s *Store) *CountryRepository { return &CountryRepository{s: s} }

func (r *CountryRepository) Create(ctx context.Context, c *catalog.Country) error {
	return insertOne(ctx, r.s.coll(CollCountries), toCountryDoc(c), "a country with this iso_code already exists")
}

func (r *CountryRepository) Update(ctx context.Context, c *catalog.Country) error {
	return replaceByID(ctx, r.s.coll(CollCountries), c.ID, toCountryDoc(c), "a country with this iso_code already exists", "country not found")
}

func (r *CountryRepository) FindByID(ctx context.Context, id string) (*catalog.Country, error) {
	d, err := findOne[countryDoc](ctx, r.s.coll(CollCountries), bson.M{"_id": id}, "country not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *CountryRepository) List(ctx context.Context, onlyActive bool) ([]*catalog.Country, error) {
	filter := bson.M{}
	if onlyActive {
		filter["active"] = true
	}
	docs, err := findMany[countryDoc](ctx, r.s.coll(CollCountries), filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*countryDoc).toDomain), nil
}

type currencyDoc struct {
	ID        string    `bson:"_id"`
	ISOCode   string    `bson:"iso_code"`
	Name      string    `bson:"name"`
	Symbol    string    `bson:"symbol"`
	Decimals  int       `bson:"decimals"`
	Active    bool      `bson:"active"`
	CreatedAt time.Time `bson:"created_at"`
}

func toCurrencyDoc(c *catalog.Currency) currencyDoc {
	return currencyDoc{ID: c.ID, ISOCode: c.ISOCode, Name: c.Name, Symbol: c.Symbol, Decimals: c.Decimals, Active: c.Active, CreatedAt: c.CreatedAt}
}

func (d *currencyDoc) toDomain() *catalog.Currency {
	return &catalog.Currency{ID: d.ID, ISOCode: d.ISOCode, Name: d.Name, Symbol: d.Symbol, Decimals: d.Decimals, Active: d.Active, CreatedAt: d.CreatedAt.UTC()}
}

type CurrencyRepository struct{ s *Store }

func NewCurrencyRepository(s *Store) *CurrencyRepository { return &CurrencyRepository{s: s} }

func (r *CurrencyRepository) Create(ctx context.Context, c *catalog.Currency) error {
	return insertOne(ctx, r.s.coll(CollCurrencies), toCurrencyDoc(c), "a currency with this iso_code already exists")
}

func (r *CurrencyRepository) Update(ctx context.Context, c *catalog.Currency) error {
	return replaceByID(ctx, r.s.coll(CollCurrencies), c.ID, toCurrencyDoc(c), "a currency with this iso_code already exists", "currency not found")
}

func (r *CurrencyRepository) FindByID(ctx context.Context, id string) (*catalog.Currency, error) {
	d, err := findOne[currencyDoc](ctx, r.s.coll(CollCurrencies), bson.M{"_id": id}, "currency not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *CurrencyRepository) List(ctx context.Context, onlyActive bool) ([]*catalog.Currency, error) {
	filter := bson.M{}
	if onlyActive {
		filter["active"] = true
	}
	docs, err := findMany[currencyDoc](ctx, r.s.coll(CollCurrencies), filter, options.Find().SetSort(bson.D{{Key: "iso_code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*currencyDoc).toDomain), nil
}
