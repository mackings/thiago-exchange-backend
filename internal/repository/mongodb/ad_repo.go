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

type adDoc struct {
	ID                string    `bson:"_id"`
	OwnerID           string    `bson:"ownerId"`
	Side              string    `bson:"side"`
	Asset             string    `bson:"asset"`
	Fiat              string    `bson:"fiat"`
	RateType          string    `bson:"rateType"`
	FixedRate         float64   `bson:"fixedRate"`
	FloatingMarginPct float64   `bson:"floatingMarginPct"`
	MinLimit          float64   `bson:"minLimit"`
	MaxLimit          float64   `bson:"maxLimit"`
	AvailableAmount   float64   `bson:"availableAmount"`
	PaymentMethods    string    `bson:"paymentMethods"`
	Terms             string    `bson:"terms"`
	Status            string    `bson:"status"`
	CreatedAt         time.Time `bson:"createdAt"`
	UpdatedAt         time.Time `bson:"updatedAt"`
}

func adToDoc(a *domain.Ad) adDoc {
	return adDoc{
		ID: a.ID.String(), OwnerID: a.OwnerID.String(), Side: string(a.Side), Asset: a.Asset, Fiat: a.Fiat,
		RateType: string(a.RateType), FixedRate: a.FixedRate, FloatingMarginPct: a.FloatingMarginPct,
		MinLimit: a.MinLimit, MaxLimit: a.MaxLimit, AvailableAmount: a.AvailableAmount,
		PaymentMethods: a.PaymentMethods, Terms: a.Terms, Status: string(a.Status),
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func adFromDoc(d adDoc) *domain.Ad {
	id, _ := uuid.Parse(d.ID)
	ownerID, _ := uuid.Parse(d.OwnerID)
	return &domain.Ad{
		ID: id, OwnerID: ownerID, Side: domain.AdSide(d.Side), Asset: d.Asset, Fiat: d.Fiat,
		RateType: domain.RateType(d.RateType), FixedRate: d.FixedRate, FloatingMarginPct: d.FloatingMarginPct,
		MinLimit: d.MinLimit, MaxLimit: d.MaxLimit, AvailableAmount: d.AvailableAmount,
		PaymentMethods: d.PaymentMethods, Terms: d.Terms, Status: domain.AdStatus(d.Status),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type AdRepo struct{ col *mongo.Collection }

func NewAdRepo(db *mongo.Database) *AdRepo { return &AdRepo{col: db.Collection("ads")} }

func (r *AdRepo) Create(ctx context.Context, a *domain.Ad) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	now := time.Now()
	a.CreatedAt, a.UpdatedAt = now, now
	_, err := r.col.InsertOne(ctx, adToDoc(a))
	return err
}

func (r *AdRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Ad, error) {
	var doc adDoc
	err := r.col.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return adFromDoc(doc), nil
}

func (r *AdRepo) Update(ctx context.Context, a *domain.Ad) error {
	a.UpdatedAt = time.Now()
	_, err := r.col.ReplaceOne(ctx, bson.M{"_id": a.ID.String()}, adToDoc(a))
	return err
}

func (r *AdRepo) List(ctx context.Context, f domain.AdFilter) ([]*domain.Ad, error) {
	filter := bson.M{"status": string(domain.AdStatusActive)}
	if f.Side != nil {
		filter["side"] = string(*f.Side)
	}
	if f.Asset != "" {
		filter["asset"] = f.Asset
	}
	if f.Fiat != "" {
		filter["fiat"] = f.Fiat
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit)).SetSkip(int64(f.Offset))
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var ads []*domain.Ad
	for cur.Next(ctx) {
		var doc adDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		ads = append(ads, adFromDoc(doc))
	}
	return ads, cur.Err()
}

func (r *AdRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]*domain.Ad, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	cur, err := r.col.Find(ctx, bson.M{"ownerId": ownerID.String()}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var ads []*domain.Ad
	for cur.Next(ctx) {
		var doc adDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		ads = append(ads, adFromDoc(doc))
	}
	return ads, cur.Err()
}
