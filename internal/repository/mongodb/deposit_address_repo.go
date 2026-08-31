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

type depositAddressDoc struct {
	ID             string    `bson:"_id"`
	Asset          string    `bson:"asset"`
	Chain          string    `bson:"chain"`
	Address        string    `bson:"address"`
	Tag            string    `bson:"tag"`
	AddedByAdminID string    `bson:"addedByAdminId"`
	CreatedAt      time.Time `bson:"createdAt"`
	UpdatedAt      time.Time `bson:"updatedAt"`
}

func depositAddressToDoc(d *domain.DepositAddress) depositAddressDoc {
	return depositAddressDoc{
		ID: d.ID.String(), Asset: d.Asset, Chain: d.Chain, Address: normalizeAddress(d.Address), Tag: d.Tag,
		AddedByAdminID: d.AddedByAdminID.String(), CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func depositAddressFromDoc(doc depositAddressDoc) *domain.DepositAddress {
	id, _ := uuid.Parse(doc.ID)
	adminID, _ := uuid.Parse(doc.AddedByAdminID)
	return &domain.DepositAddress{
		ID: id, Asset: doc.Asset, Chain: doc.Chain, Address: doc.Address, Tag: doc.Tag,
		AddedByAdminID: adminID, CreatedAt: doc.CreatedAt, UpdatedAt: doc.UpdatedAt,
	}
}

type DepositAddressRepo struct{ col *mongo.Collection }

func NewDepositAddressRepo(db *mongo.Database) *DepositAddressRepo {
	return &DepositAddressRepo{col: db.Collection("deposit_addresses")}
}

// Upsert keeps a single live document per asset — admin correcting an
// address (wrong chain, address rotated on Bybit's side) replaces it rather
// than piling up stale rows a lookup could accidentally match.
func (r *DepositAddressRepo) Upsert(ctx context.Context, d *domain.DepositAddress) error {
	now := time.Now()
	d.UpdatedAt = now
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	opts := options.Replace().SetUpsert(true)
	_, err := r.col.ReplaceOne(ctx, bson.M{"asset": d.Asset}, depositAddressToDoc(d), opts)
	return err
}

func (r *DepositAddressRepo) GetByAsset(ctx context.Context, asset string) (*domain.DepositAddress, error) {
	var doc depositAddressDoc
	err := r.col.FindOne(ctx, bson.M{"asset": asset}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrDepositAddressNotSet
		}
		return nil, err
	}
	return depositAddressFromDoc(doc), nil
}

func (r *DepositAddressRepo) ListAll(ctx context.Context) ([]*domain.DepositAddress, error) {
	opts := options.Find().SetSort(bson.D{{Key: "asset", Value: 1}})
	cur, err := r.col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var list []*domain.DepositAddress
	for cur.Next(ctx) {
		var doc depositAddressDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		list = append(list, depositAddressFromDoc(doc))
	}
	return list, cur.Err()
}
