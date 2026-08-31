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

type passwordResetDoc struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"userId"`
	TokenHash string    `bson:"tokenHash"`
	ExpiresAt time.Time `bson:"expiresAt"`
	Used      bool      `bson:"used"`
	CreatedAt time.Time `bson:"createdAt"`
}

func passwordResetToDoc(t *domain.PasswordResetToken) passwordResetDoc {
	return passwordResetDoc{
		ID: t.ID.String(), UserID: t.UserID.String(), TokenHash: t.TokenHash,
		ExpiresAt: t.ExpiresAt, Used: t.Used, CreatedAt: t.CreatedAt,
	}
}

func passwordResetFromDoc(d passwordResetDoc) *domain.PasswordResetToken {
	id, _ := uuid.Parse(d.ID)
	userID, _ := uuid.Parse(d.UserID)
	return &domain.PasswordResetToken{
		ID: id, UserID: userID, TokenHash: d.TokenHash,
		ExpiresAt: d.ExpiresAt, Used: d.Used, CreatedAt: d.CreatedAt,
	}
}

type PasswordResetRepo struct{ col *mongo.Collection }

func NewPasswordResetRepo(db *mongo.Database) *PasswordResetRepo {
	return &PasswordResetRepo{col: db.Collection("password_resets")}
}

func (r *PasswordResetRepo) Create(ctx context.Context, t *domain.PasswordResetToken) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	t.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, passwordResetToDoc(t))
	return err
}

func (r *PasswordResetRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	var doc passwordResetDoc
	err := r.col.FindOne(ctx, bson.M{"tokenHash": tokenHash}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return passwordResetFromDoc(doc), nil
}

func (r *PasswordResetRepo) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id.String()}, bson.M{"$set": bson.M{"used": true}})
	return err
}
