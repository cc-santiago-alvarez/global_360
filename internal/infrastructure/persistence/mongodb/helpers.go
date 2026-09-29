package mongodb

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"global_360/internal/domain/shared"
)

// insertOne inserts doc; unique violations become shared.ErrConflict with conflictMsg.
func insertOne(ctx context.Context, c *mongo.Collection, doc any, conflictMsg string) error {
	_, err := c.InsertOne(ctx, doc)
	return writeErr(err, conflictMsg)
}

// replaceByID replaces the whole document identified by id.
func replaceByID(ctx context.Context, c *mongo.Collection, id string, doc any, conflictMsg, notFoundMsg string) error {
	res, err := c.ReplaceOne(ctx, bson.M{"_id": id}, doc)
	if err != nil {
		return writeErr(err, conflictMsg)
	}
	if res.MatchedCount == 0 {
		return shared.NotFound("%s", notFoundMsg)
	}
	return nil
}

func writeErr(err error, conflictMsg string) error {
	if err == nil {
		return nil
	}
	if mongo.IsDuplicateKeyError(err) {
		return shared.Conflict("%s", conflictMsg)
	}
	return err
}

// findOne decodes a single document; no match becomes shared.ErrNotFound with notFoundMsg.
func findOne[T any](ctx context.Context, c *mongo.Collection, filter any, notFoundMsg string) (*T, error) {
	var doc T
	err := c.FindOne(ctx, filter).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, shared.NotFound("%s", notFoundMsg)
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// findMany decodes every matching document.
func findMany[T any](ctx context.Context, c *mongo.Collection, filter any, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	cur, err := c.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	var docs []T
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// findPage returns one page of documents plus the total count of matches.
func findPage[T any](ctx context.Context, c *mongo.Collection, filter any, sort bson.D, p shared.Pagination) ([]T, int64, error) {
	total, err := c.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetSort(sort).SetSkip(p.Offset()).SetLimit(int64(p.PageSize))
	docs, err := findMany[T](ctx, c, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	return docs, total, nil
}

// mapAll converts documents to domain entities.
func mapAll[D any, E any](docs []D, fn func(*D) *E) []*E {
	out := make([]*E, 0, len(docs))
	for i := range docs {
		out = append(out, fn(&docs[i]))
	}
	return out
}
