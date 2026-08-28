package mongodb

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"thiagoexchange/backend/internal/domain"
)

type ledgerEntryDoc struct {
	ID               string    `bson:"_id"`
	UserID           string    `bson:"userId"`
	Asset            string    `bson:"asset"`
	Bucket           string    `bson:"bucket"`
	Direction        string    `bson:"direction"`
	Amount           float64   `bson:"amount"`
	Reason           string    `bson:"reason"`
	OrderID          *string   `bson:"orderId,omitempty"`
	CreatedByAdminID *string   `bson:"createdByAdminId,omitempty"`
	Note             string    `bson:"note"`
	CreatedAt        time.Time `bson:"createdAt"`
}

func ledgerEntryToDoc(e *domain.LedgerEntry) ledgerEntryDoc {
	d := ledgerEntryDoc{
		ID: e.ID.String(), UserID: e.UserID.String(), Asset: e.Asset, Bucket: string(e.Bucket),
		Direction: string(e.Direction), Amount: e.Amount, Reason: string(e.Reason),
		Note: e.Note, CreatedAt: e.CreatedAt,
	}
	if e.OrderID != nil {
		s := e.OrderID.String()
		d.OrderID = &s
	}
	if e.CreatedByAdminID != nil {
		s := e.CreatedByAdminID.String()
		d.CreatedByAdminID = &s
	}
	return d
}

func ledgerEntryFromDoc(d ledgerEntryDoc) *domain.LedgerEntry {
	id, _ := uuid.Parse(d.ID)
	userID, _ := uuid.Parse(d.UserID)
	e := &domain.LedgerEntry{
		ID: id, UserID: userID, Asset: d.Asset, Bucket: domain.LedgerBucket(d.Bucket),
		Direction: domain.LedgerDirection(d.Direction), Amount: d.Amount, Reason: domain.LedgerReason(d.Reason),
		Note: d.Note, CreatedAt: d.CreatedAt,
	}
	if d.OrderID != nil {
		if oid, err := uuid.Parse(*d.OrderID); err == nil {
			e.OrderID = &oid
		}
	}
	if d.CreatedByAdminID != nil {
		if aid, err := uuid.Parse(*d.CreatedByAdminID); err == nil {
			e.CreatedByAdminID = &aid
		}
	}
	return e
}

type LedgerRepo struct {
	col    *mongo.Collection
	client *mongo.Client
}

func NewLedgerRepo(db *mongo.Database, client *mongo.Client) *LedgerRepo {
	return &LedgerRepo{col: db.Collection("ledger_entries"), client: client}
}

func (r *LedgerRepo) Create(ctx context.Context, e *domain.LedgerEntry) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	e.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, ledgerEntryToDoc(e))
	return err
}

// CreateTx writes all entries atomically via a session transaction. This
// requires MongoDB to be running as a replica set (even a single-node one) —
// see docker-compose.yml. Ledger correctness (never leaving a lock/unlock or
// release pair half-written) depends on this being a real transaction.
func (r *LedgerRepo) CreateTx(ctx context.Context, entries []*domain.LedgerEntry) error {
	docs := make([]interface{}, len(entries))
	for i, e := range entries {
		if e.ID == uuid.Nil {
			e.ID = uuid.New()
		}
		e.CreatedAt = time.Now()
		docs[i] = ledgerEntryToDoc(e)
	}

	session, err := r.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sc context.Context) (interface{}, error) {
		_, err := r.col.InsertMany(sc, docs)
		return nil, err
	})
	return err
}

func (r *LedgerRepo) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.LedgerEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit)).SetSkip(int64(offset))
	cur, err := r.col.Find(ctx, bson.M{"userId": userID.String()}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var entries []*domain.LedgerEntry
	for cur.Next(ctx) {
		var doc ledgerEntryDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		entries = append(entries, ledgerEntryFromDoc(doc))
	}
	return entries, cur.Err()
}

type balanceGroupResult struct {
	ID struct {
		Asset     string `bson:"asset"`
		Bucket    string `bson:"bucket"`
		Direction string `bson:"direction"`
	} `bson:"_id"`
	Total float64 `bson:"total"`
}

func (r *LedgerRepo) BalancesByUser(ctx context.Context, userID uuid.UUID) ([]domain.Balance, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID.String()}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"asset": "$asset", "bucket": "$bucket", "direction": "$direction"},
			"total": bson.M{"$sum": "$amount"},
		}}},
	}
	cur, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	byAsset := map[string]*domain.Balance{}
	get := func(asset string) *domain.Balance {
		b, ok := byAsset[asset]
		if !ok {
			b = &domain.Balance{Asset: asset}
			byAsset[asset] = b
		}
		return b
	}

	for cur.Next(ctx) {
		var row balanceGroupResult
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		b := get(row.ID.Asset)
		signed := row.Total
		if row.ID.Direction == string(domain.DirectionOut) {
			signed = -signed
		}
		if row.ID.Bucket == string(domain.BucketAvailable) {
			b.Available += signed
		} else {
			b.Locked += signed
		}
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	balances := make([]domain.Balance, 0, len(byAsset))
	for _, b := range byAsset {
		balances = append(balances, *b)
	}
	return balances, nil
}
