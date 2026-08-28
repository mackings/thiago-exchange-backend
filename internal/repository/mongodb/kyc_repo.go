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

type kycDoc struct {
	ID          string    `bson:"_id"`
	UserID      string    `bson:"userId"`
	FullName    string    `bson:"fullName"`
	IDType      string    `bson:"idType"`
	IDNumber    string    `bson:"idNumber"`
	DocumentURL string    `bson:"documentUrl"`
	Status      string    `bson:"status"`
	ReviewNote  string    `bson:"reviewNote"`
	ReviewedBy  *string   `bson:"reviewedBy,omitempty"`
	CreatedAt   time.Time `bson:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt"`
}

func kycToDoc(k *domain.KYCSubmission) kycDoc {
	d := kycDoc{
		ID: k.ID.String(), UserID: k.UserID.String(), FullName: k.FullName, IDType: k.IDType,
		IDNumber: k.IDNumber, DocumentURL: k.DocumentURL, Status: string(k.Status),
		ReviewNote: k.ReviewNote, CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt,
	}
	if k.ReviewedBy != nil {
		s := k.ReviewedBy.String()
		d.ReviewedBy = &s
	}
	return d
}

func kycFromDoc(d kycDoc) *domain.KYCSubmission {
	id, _ := uuid.Parse(d.ID)
	userID, _ := uuid.Parse(d.UserID)
	k := &domain.KYCSubmission{
		ID: id, UserID: userID, FullName: d.FullName, IDType: d.IDType, IDNumber: d.IDNumber,
		DocumentURL: d.DocumentURL, Status: domain.KYCStatus(d.Status), ReviewNote: d.ReviewNote,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.ReviewedBy != nil {
		if rid, err := uuid.Parse(*d.ReviewedBy); err == nil {
			k.ReviewedBy = &rid
		}
	}
	return k
}

type KYCRepo struct{ col *mongo.Collection }

func NewKYCRepo(db *mongo.Database) *KYCRepo { return &KYCRepo{col: db.Collection("kyc_submissions")} }

func (r *KYCRepo) Create(ctx context.Context, k *domain.KYCSubmission) error {
	if k.ID == uuid.Nil {
		k.ID = uuid.New()
	}
	now := time.Now()
	k.CreatedAt, k.UpdatedAt = now, now
	_, err := r.col.InsertOne(ctx, kycToDoc(k))
	return err
}

func (r *KYCRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.KYCSubmission, error) {
	var doc kycDoc
	err := r.col.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return kycFromDoc(doc), nil
}

func (r *KYCRepo) GetLatestByUser(ctx context.Context, userID uuid.UUID) (*domain.KYCSubmission, error) {
	opts := options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	var doc kycDoc
	err := r.col.FindOne(ctx, bson.M{"userId": userID.String()}, opts).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return kycFromDoc(doc), nil
}

func (r *KYCRepo) ListPending(ctx context.Context, limit, offset int) ([]*domain.KYCSubmission, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit)).SetSkip(int64(offset))
	}
	cur, err := r.col.Find(ctx, bson.M{"status": string(domain.KYCStatusPending)}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var list []*domain.KYCSubmission
	for cur.Next(ctx) {
		var doc kycDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		list = append(list, kycFromDoc(doc))
	}
	return list, cur.Err()
}

func (r *KYCRepo) Update(ctx context.Context, k *domain.KYCSubmission) error {
	k.UpdatedAt = time.Now()
	_, err := r.col.ReplaceOne(ctx, bson.M{"_id": k.ID.String()}, kycToDoc(k))
	return err
}

type paymentMethodDoc struct {
	ID          string    `bson:"_id"`
	UserID      string    `bson:"userId"`
	Type        string    `bson:"type"`
	BankName    string    `bson:"bankName"`
	AccountName string    `bson:"accountName"`
	AccountNo   string    `bson:"accountNo"`
	CreatedAt   time.Time `bson:"createdAt"`
}

func paymentMethodToDoc(p *domain.PaymentMethod) paymentMethodDoc {
	return paymentMethodDoc{
		ID: p.ID.String(), UserID: p.UserID.String(), Type: p.Type, BankName: p.BankName,
		AccountName: p.AccountName, AccountNo: p.AccountNo, CreatedAt: p.CreatedAt,
	}
}

func paymentMethodFromDoc(d paymentMethodDoc) *domain.PaymentMethod {
	id, _ := uuid.Parse(d.ID)
	userID, _ := uuid.Parse(d.UserID)
	return &domain.PaymentMethod{
		ID: id, UserID: userID, Type: d.Type, BankName: d.BankName,
		AccountName: d.AccountName, AccountNo: d.AccountNo, CreatedAt: d.CreatedAt,
	}
}

type PaymentMethodRepo struct{ col *mongo.Collection }

func NewPaymentMethodRepo(db *mongo.Database) *PaymentMethodRepo {
	return &PaymentMethodRepo{col: db.Collection("payment_methods")}
}

func (r *PaymentMethodRepo) Create(ctx context.Context, p *domain.PaymentMethod) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	p.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, paymentMethodToDoc(p))
	return err
}

func (r *PaymentMethodRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.PaymentMethod, error) {
	cur, err := r.col.Find(ctx, bson.M{"userId": userID.String()})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var list []*domain.PaymentMethod
	for cur.Next(ctx) {
		var doc paymentMethodDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		list = append(list, paymentMethodFromDoc(doc))
	}
	return list, cur.Err()
}

func (r *PaymentMethodRepo) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id.String(), "userId": userID.String()})
	return err
}
