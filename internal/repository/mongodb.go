package repository

import (
	"context"
	"fmt"

	"github.com/m2tx/agent_example/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type sessionDocument struct {
	ID      string          `bson:"_id"`
	History []model.Content `bson:"history"`
}

// MongoSessionRepository implements SessionRepository using MongoDB.
type MongoSessionRepository struct {
	collection *mongo.Collection
}

// NewMongoSessionRepository creates a new MongoSessionRepository.
// collectionName defaults to "sessions" if empty.
func NewMongoSessionRepository(db *mongo.Database, collectionName string) *MongoSessionRepository {
	if collectionName == "" {
		collectionName = "sessions"
	}
	return &MongoSessionRepository{
		collection: db.Collection(collectionName),
	}
}

func (r *MongoSessionRepository) Save(ctx context.Context, sessionID string, history []model.Content) error {
	doc := sessionDocument{
		ID:      sessionID,
		History: history,
	}

	filter := bson.M{"_id": sessionID}
	update := bson.M{"$set": doc}
	opts := options.Update().SetUpsert(true)

	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("repository: upsert session %q: %w", sessionID, err)
	}

	return nil
}

func (r *MongoSessionRepository) Load(ctx context.Context, sessionID string) ([]model.Content, error) {
	filter := bson.M{"_id": sessionID}

	var doc sessionDocument
	err := r.collection.FindOne(ctx, filter).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repository: find session %q: %w", sessionID, err)
	}

	return doc.History, nil
}

func (r *MongoSessionRepository) Delete(ctx context.Context, sessionID string) error {
	filter := bson.M{"_id": sessionID}

	_, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("repository: delete session %q: %w", sessionID, err)
	}

	return nil
}
