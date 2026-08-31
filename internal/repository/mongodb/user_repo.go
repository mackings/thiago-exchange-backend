// Package mongodb implements the domain repository interfaces against
// MongoDB. Each file defines a private bson-tagged document type and
// to/from converters, keeping the domain package itself persistence-agnostic.
package mongodb

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"thiagoexchange/backend/internal/domain"
)

type userDoc struct {
	ID                string    `bson:"_id"`
	Email             string    `bson:"email"`
	Phone             string    `bson:"phone"`
	PasswordHash      string    `bson:"passwordHash"`
	FullName          string    `bson:"fullName"`
	Role              string    `bson:"role"`
	KYCStatus         string    `bson:"kycStatus"`
	Disabled          bool      `bson:"disabled"`
	BankName          string    `bson:"bankName,omitempty"`
	BankAccountNumber string    `bson:"bankAccountNumber,omitempty"`
	BankAccountName   string    `bson:"bankAccountName,omitempty"`
	CreatedAt         time.Time `bson:"createdAt"`
	UpdatedAt         time.Time `bson:"updatedAt"`
}

func userToDoc(u *domain.User) userDoc {
	return userDoc{
		ID: u.ID.String(), Email: u.Email, Phone: u.Phone, PasswordHash: u.PasswordHash,
		FullName: u.FullName, Role: string(u.Role), KYCStatus: string(u.KYCStatus),
		Disabled: u.Disabled, BankName: u.BankName, BankAccountNumber: u.BankAccountNumber,
		BankAccountName: u.BankAccountName, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

func userFromDoc(d userDoc) *domain.User {
	id, _ := uuid.Parse(d.ID)
	return &domain.User{
		ID: id, Email: d.Email, Phone: d.Phone, PasswordHash: d.PasswordHash,
		FullName: d.FullName, Role: domain.Role(d.Role), KYCStatus: domain.KYCStatus(d.KYCStatus),
		Disabled: d.Disabled, BankName: d.BankName, BankAccountNumber: d.BankAccountNumber,
		BankAccountName: d.BankAccountName, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type UserRepo struct{ col *mongo.Collection }

func NewUserRepo(db *mongo.Database) *UserRepo {
	return &UserRepo{col: db.Collection("users")}
}

func EnsureUserIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("users").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	now := time.Now()
	u.CreatedAt, u.UpdatedAt = now, now
	_, err := r.col.InsertOne(ctx, userToDoc(u))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var doc userDoc
	err := r.col.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userFromDoc(doc), nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var doc userDoc
	err := r.col.FindOne(ctx, bson.M{"email": email}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userFromDoc(doc), nil
}

func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	u.UpdatedAt = time.Now()
	_, err := r.col.ReplaceOne(ctx, bson.M{"_id": u.ID.String()}, userToDoc(u))
	return err
}

func (r *UserRepo) List(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit)).SetSkip(int64(offset))
	}
	cur, err := r.col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var users []*domain.User
	for cur.Next(ctx) {
		var doc userDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		users = append(users, userFromDoc(doc))
	}
	return users, cur.Err()
}
