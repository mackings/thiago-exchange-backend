package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"thiagoexchange/backend/internal/domain"
)

// Mailer is the narrow email-sending capability this usecase needs.
// Defined here (consumer-side) so the usecase doesn't depend on the
// concrete SMTP implementation in internal/infra/mailer.
type Mailer interface {
	Send(to, subject, body string) error
}

type Service struct {
	users           domain.UserRepository
	jwtSecret       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	adminEmails     map[string]bool
	mailer          Mailer
}

func NewService(users domain.UserRepository, jwtSecret string, accessTTL, refreshTTL time.Duration, adminEmails []string, mailer Mailer) *Service {
	set := make(map[string]bool, len(adminEmails))
	for _, e := range adminEmails {
		set[strings.ToLower(e)] = true
	}
	return &Service{users: users, jwtSecret: jwtSecret, accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL, adminEmails: set, mailer: mailer}
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func (s *Service) issueTokens(u *domain.User) (TokenPair, error) {
	access, err := generateToken(s.jwtSecret, u.ID, u.Role, TokenAccess, s.accessTokenTTL)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := generateToken(s.jwtSecret, u.ID, u.Role, TokenRefresh, s.refreshTokenTTL)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

// roleFor grants admin to any address on the ADMIN_EMAILS allowlist —
// Thiago's own operators — everyone else is a regular user.
func (s *Service) roleFor(email string) domain.Role {
	if s.adminEmails[strings.ToLower(email)] {
		return domain.RoleAdmin
	}
	return domain.RoleUser
}

func (s *Service) Register(ctx context.Context, email, phone, password, fullName string) (*domain.User, TokenPair, error) {
	if email == "" || password == "" {
		return nil, TokenPair{}, domain.ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, TokenPair{}, err
	}
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		Phone:        phone,
		PasswordHash: string(hash),
		FullName:     fullName,
		Role:         s.roleFor(email),
		KYCStatus:    domain.KYCStatusUnverified,
	}
	if u.Role == domain.RoleAdmin {
		// Admins are Thiago's own operators — auto-verify so they aren't
		// blocked from posting ads by the KYC gate the same day they sign up.
		u.KYCStatus = domain.KYCStatusVerified
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, TokenPair{}, err
	}
	tokens, err := s.issueTokens(u)
	if err != nil {
		return nil, TokenPair{}, err
	}

	go func() {
		_ = s.mailer.Send(u.Email, "Welcome to Thiago Exchange",
			fmt.Sprintf("Hi %s,\n\nYour Thiago Exchange account is ready. Head to the Market tab to start trading.\n\n— Thiago Exchange", u.FullName))
	}()

	return u, tokens, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (*domain.User, TokenPair, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, TokenPair{}, domain.ErrInvalidCredentials
		}
		return nil, TokenPair{}, err
	}
	if u.Disabled {
		return nil, TokenPair{}, domain.ErrForbidden
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, TokenPair{}, domain.ErrInvalidCredentials
	}

	// Re-check the allowlist on every login so adding/removing an email from
	// ADMIN_EMAILS takes effect without the user needing to re-register.
	if want := s.roleFor(u.Email); want != u.Role {
		u.Role = want
		if want == domain.RoleAdmin {
			u.KYCStatus = domain.KYCStatusVerified
		}
		if err := s.users.Update(ctx, u); err != nil {
			return nil, TokenPair{}, err
		}
	}

	tokens, err := s.issueTokens(u)
	if err != nil {
		return nil, TokenPair{}, err
	}
	return u, tokens, nil
}

func (s *Service) Me(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.users.GetByID(ctx, userID)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := ParseToken(s.jwtSecret, refreshToken)
	if err != nil || claims.Type != TokenRefresh {
		return TokenPair{}, domain.ErrUnauthorized
	}
	u, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return TokenPair{}, domain.ErrUnauthorized
	}
	if u.Disabled {
		return TokenPair{}, domain.ErrForbidden
	}
	return s.issueTokens(u)
}
