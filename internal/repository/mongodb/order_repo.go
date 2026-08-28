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

type orderDoc struct {
	ID              string    `bson:"_id"`
	AdID            string    `bson:"adId"`
	MerchantID      string    `bson:"merchantId"`
	Side            string    `bson:"side"`
	BuyerID         string    `bson:"buyerId"`
	SellerID        string    `bson:"sellerId"`
	Asset           string    `bson:"asset"`
	Fiat            string    `bson:"fiat"`
	Amount          float64   `bson:"amount"`
	Rate            float64   `bson:"rate"`
	FiatAmount      float64   `bson:"fiatAmount"`
	Status          string    `bson:"status"`
	PayoutAddress   string    `bson:"payoutAddress,omitempty"`
	PayoutChain     string    `bson:"payoutChain,omitempty"`
	DepositTxID     string    `bson:"depositTxId,omitempty"`
	DepositChain    string    `bson:"depositChain,omitempty"`
	DepositAmount   float64   `bson:"depositAmount,omitempty"`
	PaymentDeadline time.Time `bson:"paymentDeadline"`
	PaymentProofURL string    `bson:"paymentProofUrl"`
	CreatedAt       time.Time `bson:"createdAt"`
	UpdatedAt       time.Time `bson:"updatedAt"`
}

func orderToDoc(o *domain.Order) orderDoc {
	return orderDoc{
		ID: o.ID.String(), AdID: o.AdID.String(), MerchantID: o.MerchantID.String(), Side: string(o.Side),
		BuyerID: o.BuyerID.String(), SellerID: o.SellerID.String(),
		Asset: o.Asset, Fiat: o.Fiat, Amount: o.Amount, Rate: o.Rate, FiatAmount: o.FiatAmount,
		Status: string(o.Status), PayoutAddress: o.PayoutAddress, PayoutChain: o.PayoutChain,
		DepositTxID: o.DepositTxID, DepositChain: o.DepositChain, DepositAmount: o.DepositAmount,
		PaymentDeadline: o.PaymentDeadline, PaymentProofURL: o.PaymentProofURL,
		CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

func orderFromDoc(d orderDoc) *domain.Order {
	id, _ := uuid.Parse(d.ID)
	adID, _ := uuid.Parse(d.AdID)
	merchantID, _ := uuid.Parse(d.MerchantID)
	buyerID, _ := uuid.Parse(d.BuyerID)
	sellerID, _ := uuid.Parse(d.SellerID)
	return &domain.Order{
		ID: id, AdID: adID, MerchantID: merchantID, Side: domain.AdSide(d.Side),
		BuyerID: buyerID, SellerID: sellerID, Asset: d.Asset, Fiat: d.Fiat,
		Amount: d.Amount, Rate: d.Rate, FiatAmount: d.FiatAmount, Status: domain.OrderStatus(d.Status),
		PayoutAddress: d.PayoutAddress, PayoutChain: d.PayoutChain,
		DepositTxID: d.DepositTxID, DepositChain: d.DepositChain, DepositAmount: d.DepositAmount,
		PaymentDeadline: d.PaymentDeadline, PaymentProofURL: d.PaymentProofURL,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type OrderRepo struct{ col *mongo.Collection }

func NewOrderRepo(db *mongo.Database) *OrderRepo { return &OrderRepo{col: db.Collection("orders")} }

func (r *OrderRepo) Create(ctx context.Context, o *domain.Order) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	now := time.Now()
	o.CreatedAt, o.UpdatedAt = now, now
	_, err := r.col.InsertOne(ctx, orderToDoc(o))
	return err
}

func (r *OrderRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	var doc orderDoc
	err := r.col.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return orderFromDoc(doc), nil
}

func (r *OrderRepo) Update(ctx context.Context, o *domain.Order) error {
	o.UpdatedAt = time.Now()
	_, err := r.col.ReplaceOne(ctx, bson.M{"_id": o.ID.String()}, orderToDoc(o))
	return err
}

func (r *OrderRepo) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	if limit <= 0 {
		limit = 50
	}
	filter := bson.M{"$or": []bson.M{{"buyerId": userID.String()}, {"sellerId": userID.String()}}}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit)).SetSkip(int64(offset))
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var orders []*domain.Order
	for cur.Next(ctx) {
		var doc orderDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		orders = append(orders, orderFromDoc(doc))
	}
	return orders, cur.Err()
}

type orderMessageDoc struct {
	ID            string    `bson:"_id"`
	OrderID       string    `bson:"orderId"`
	SenderID      string    `bson:"senderId"`
	Body          string    `bson:"body"`
	AttachmentURL string    `bson:"attachmentUrl"`
	CreatedAt     time.Time `bson:"createdAt"`
}

func orderMessageToDoc(m *domain.OrderMessage) orderMessageDoc {
	return orderMessageDoc{
		ID: m.ID.String(), OrderID: m.OrderID.String(), SenderID: m.SenderID.String(),
		Body: m.Body, AttachmentURL: m.AttachmentURL, CreatedAt: m.CreatedAt,
	}
}

func orderMessageFromDoc(d orderMessageDoc) *domain.OrderMessage {
	id, _ := uuid.Parse(d.ID)
	orderID, _ := uuid.Parse(d.OrderID)
	senderID, _ := uuid.Parse(d.SenderID)
	return &domain.OrderMessage{
		ID: id, OrderID: orderID, SenderID: senderID, Body: d.Body,
		AttachmentURL: d.AttachmentURL, CreatedAt: d.CreatedAt,
	}
}

type OrderMessageRepo struct{ col *mongo.Collection }

func NewOrderMessageRepo(db *mongo.Database) *OrderMessageRepo {
	return &OrderMessageRepo{col: db.Collection("order_messages")}
}

func (r *OrderMessageRepo) Create(ctx context.Context, m *domain.OrderMessage) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	m.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, orderMessageToDoc(m))
	return err
}

func (r *OrderMessageRepo) ListByOrder(ctx context.Context, orderID uuid.UUID) ([]*domain.OrderMessage, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})
	cur, err := r.col.Find(ctx, bson.M{"orderId": orderID.String()}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var msgs []*domain.OrderMessage
	for cur.Next(ctx) {
		var doc orderMessageDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		msgs = append(msgs, orderMessageFromDoc(doc))
	}
	return msgs, cur.Err()
}

type disputeDoc struct {
	ID         string    `bson:"_id"`
	OrderID    string    `bson:"orderId"`
	RaisedBy   string    `bson:"raisedBy"`
	Reason     string    `bson:"reason"`
	Status     string    `bson:"status"`
	Resolution string    `bson:"resolution"`
	ResolvedBy *string   `bson:"resolvedBy,omitempty"`
	CreatedAt  time.Time `bson:"createdAt"`
	UpdatedAt  time.Time `bson:"updatedAt"`
}

func disputeToDoc(d *domain.Dispute) disputeDoc {
	doc := disputeDoc{
		ID: d.ID.String(), OrderID: d.OrderID.String(), RaisedBy: d.RaisedBy.String(),
		Reason: d.Reason, Status: string(d.Status), Resolution: string(d.Resolution),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.ResolvedBy != nil {
		s := d.ResolvedBy.String()
		doc.ResolvedBy = &s
	}
	return doc
}

func disputeFromDoc(d disputeDoc) *domain.Dispute {
	id, _ := uuid.Parse(d.ID)
	orderID, _ := uuid.Parse(d.OrderID)
	raisedBy, _ := uuid.Parse(d.RaisedBy)
	out := &domain.Dispute{
		ID: id, OrderID: orderID, RaisedBy: raisedBy, Reason: d.Reason,
		Status: domain.DisputeStatus(d.Status), Resolution: domain.DisputeResolution(d.Resolution),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.ResolvedBy != nil {
		if rid, err := uuid.Parse(*d.ResolvedBy); err == nil {
			out.ResolvedBy = &rid
		}
	}
	return out
}

type DisputeRepo struct{ col *mongo.Collection }

func NewDisputeRepo(db *mongo.Database) *DisputeRepo {
	return &DisputeRepo{col: db.Collection("disputes")}
}

func (r *DisputeRepo) Create(ctx context.Context, d *domain.Dispute) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	now := time.Now()
	d.CreatedAt, d.UpdatedAt = now, now
	_, err := r.col.InsertOne(ctx, disputeToDoc(d))
	return err
}

func (r *DisputeRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Dispute, error) {
	var doc disputeDoc
	err := r.col.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return disputeFromDoc(doc), nil
}

func (r *DisputeRepo) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Dispute, error) {
	var doc disputeDoc
	err := r.col.FindOne(ctx, bson.M{"orderId": orderID.String()}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return disputeFromDoc(doc), nil
}

func (r *DisputeRepo) ListOpen(ctx context.Context, limit, offset int) ([]*domain.Dispute, error) {
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(int64(limit)).SetSkip(int64(offset))
	cur, err := r.col.Find(ctx, bson.M{"status": string(domain.DisputeStatusOpen)}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var list []*domain.Dispute
	for cur.Next(ctx) {
		var doc disputeDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		list = append(list, disputeFromDoc(doc))
	}
	return list, cur.Err()
}

func (r *DisputeRepo) Update(ctx context.Context, d *domain.Dispute) error {
	d.UpdatedAt = time.Now()
	_, err := r.col.ReplaceOne(ctx, bson.M{"_id": d.ID.String()}, disputeToDoc(d))
	return err
}
