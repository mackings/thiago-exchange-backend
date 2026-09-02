package mongodb

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"thiagoexchange/backend/internal/domain"
)

type emailVerificationDoc struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"userId"`
	TokenHash string    `bson:"tokenHash"`
	ExpiresAt time.Time `bson:"expiresAt"`
	Used      bool      `bson:"used"`
	CreatedAt time.Time `bson:"createdAt"`
}

func emailVerificationToDoc(t *domain.EmailVerificationToken) emailVerificationDoc {
	return emailVerificationDoc{
		ID: t.ID.String(), UserID: t.UserID.String(), TokenHash: t.TokenHash,
		ExpiresAt: t.ExpiresAt, Used: t.Used, CreatedAt: t.CreatedAt,
	}
}

func emailVerificationFromDoc(d emailVerificationDoc) *domain.EmailVerificationToken {
	id, _ := uuid.Parse(d.ID)
	userID, _ := uuid.Parse(d.UserID)
	return &domain.EmailVerificationToken{
		ID: id, UserID: userID, TokenHash: d.TokenHash,
		ExpiresAt: d.ExpiresAt, Used: d.Used, CreatedAt: d.CreatedAt,
	}
}

type EmailVerificationRepo struct{ col *mongo.Collection }

func NewEmailVerificationRepo(db *mongo.Database) *EmailVerificationRepo {
	return &EmailVerificationRepo{col: db.Collection("email_verifications")}
}

func (r *EmailVerificationRepo) Create(ctx context.Context, t *domain.EmailVerificationToken) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	t.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, emailVerificationToDoc(t))
	return err
}

func (r *EmailVerificationRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.EmailVerificationToken, error) {
	var doc emailVerificationDoc
	err := r.col.FindOne(ctx, bson.M{"tokenHash": tokenHash}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return emailVerificationFromDoc(doc), nil
}

func (r *EmailVerificationRepo) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id.String()}, bson.M{"$set": bson.M{"used": true}})
	return err
}
