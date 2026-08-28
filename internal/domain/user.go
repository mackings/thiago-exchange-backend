package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type KYCStatus string

const (
	KYCStatusUnverified KYCStatus = "unverified"
	KYCStatusPending    KYCStatus = "pending"
	KYCStatusVerified   KYCStatus = "verified"
	KYCStatusRejected   KYCStatus = "rejected"
)

type User struct {
	ID           uuid.UUID
	Email        string
	Phone        string
	PasswordHash string
	FullName     string
	Role         Role
	KYCStatus    KYCStatus
	Disabled     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, u *User) error
	List(ctx context.Context, limit, offset int) ([]*User, error)
}

type KYCSubmission struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	FullName    string
	IDType      string
	IDNumber    string
	DocumentURL string
	Status      KYCStatus
	ReviewNote  string
	ReviewedBy  *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type KYCRepository interface {
	Create(ctx context.Context, k *KYCSubmission) error
	GetByID(ctx context.Context, id uuid.UUID) (*KYCSubmission, error)
	GetLatestByUser(ctx context.Context, userID uuid.UUID) (*KYCSubmission, error)
	ListPending(ctx context.Context, limit, offset int) ([]*KYCSubmission, error)
	Update(ctx context.Context, k *KYCSubmission) error
}

type PaymentMethod struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Type        string // e.g. "bank_transfer"
	BankName    string
	AccountName string
	AccountNo   string
	CreatedAt   time.Time
}

type PaymentMethodRepository interface {
	Create(ctx context.Context, p *PaymentMethod) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*PaymentMethod, error)
	Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
}
