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

	// EmailVerified gates trading (see orders.Service.Create), not login —
	// an unverified user can still browse and sign in, matching how KYC
	// gates trade limits rather than account access.
	EmailVerified bool

	// Bank details admins set on their own account so buyers see a real
	// "pay to this account" instruction on sell-ad orders instead of relying
	// on it being typed into chat. Meaningless for non-admin users.
	BankName          string
	BankAccountNumber string
	BankAccountName   string

	CreatedAt time.Time
	UpdatedAt time.Time
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

// PasswordResetToken is single-use and short-lived. TokenHash stores a
// SHA-256 hash of the raw token that goes out in the reset email — the raw
// token is never persisted, so a database read alone can't be used to
// reset someone's password.
type PasswordResetToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

type PasswordResetRepository interface {
	Create(ctx context.Context, t *PasswordResetToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
}

// EmailVerificationToken follows the same single-use, short-lived,
// hash-only-at-rest shape as PasswordResetToken above.
type EmailVerificationToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

type EmailVerificationRepository interface {
	Create(ctx context.Context, t *EmailVerificationToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*EmailVerificationToken, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
}
