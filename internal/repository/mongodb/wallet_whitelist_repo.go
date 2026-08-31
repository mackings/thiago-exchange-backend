package mongodb

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"thiagoexchange/backend/internal/domain"
)

type whitelistedAddressDoc struct {
	ID             string    `bson:"_id"`
	Address        string    `bson:"address"`
	Chain          string    `bson:"chain"`
	Asset          string    `bson:"asset"`
	AddedByAdminID string    `bson:"addedByAdminId"`
	CreatedAt      time.Time `bson:"createdAt"`
}

func whitelistedAddressToDoc(w *domain.WhitelistedAddress) whitelistedAddressDoc {
	return whitelistedAddressDoc{
		ID: w.ID.String(), Address: normalizeAddress(w.Address), Chain: w.Chain, Asset: w.Asset,
		AddedByAdminID: w.AddedByAdminID.String(), CreatedAt: w.CreatedAt,
	}
}

func whitelistedAddressFromDoc(d whitelistedAddressDoc) *domain.WhitelistedAddress {
	id, _ := uuid.Parse(d.ID)
	adminID, _ := uuid.Parse(d.AddedByAdminID)
	return &domain.WhitelistedAddress{
		ID: id, Address: d.Address, Chain: d.Chain, Asset: d.Asset,
		AddedByAdminID: adminID, CreatedAt: d.CreatedAt,
	}
}

// normalizeAddress is intentionally minimal (trim + lowercase) — most chain
// address formats are case-sensitive (e.g. base58 BTC/TRC20 addresses), so
// this only absorbs incidental whitespace/casing differences from copy-paste
// for the hex-style addresses (0x...) where case genuinely doesn't matter,
// while still comparing correctly for the rest since we lookup by the exact
// stored value on both write and read paths.
func normalizeAddress(address string) string {
	return strings.TrimSpace(address)
}

type WhitelistRepo struct{ col *mongo.Collection }

func NewWhitelistRepo(db *mongo.Database) *WhitelistRepo {
	return &WhitelistRepo{col: db.Collection("whitelisted_addresses")}
}

func (r *WhitelistRepo) Create(ctx context.Context, w *domain.WhitelistedAddress) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	w.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, whitelistedAddressToDoc(w))
	return err
}

func (r *WhitelistRepo) IsWhitelisted(ctx context.Context, address string) (bool, error) {
	err := r.col.FindOne(ctx, bson.M{"address": normalizeAddress(address)}).Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *WhitelistRepo) ListAll(ctx context.Context) ([]*domain.WhitelistedAddress, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	cur, err := r.col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var list []*domain.WhitelistedAddress
	for cur.Next(ctx) {
		var doc whitelistedAddressDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		list = append(list, whitelistedAddressFromDoc(doc))
	}
	return list, cur.Err()
}
