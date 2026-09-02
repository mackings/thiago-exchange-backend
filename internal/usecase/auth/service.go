package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

const (
	passwordResetTTL     = 1 * time.Hour
	emailVerificationTTL = 24 * time.Hour
)

type Service struct {
	users           domain.UserRepository
	resets          domain.PasswordResetRepository
	verifications   domain.EmailVerificationRepository
	jwtSecret       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	adminEmails     map[string]bool
	mailer          Mailer
	frontendURL     string
}

func NewService(
	users domain.UserRepository,
	resets domain.PasswordResetRepository,
	verifications domain.EmailVerificationRepository,
	jwtSecret string,
	accessTTL, refreshTTL time.Duration,
	adminEmails []string,
	mailer Mailer,
	frontendURL string,
) *Service {
	set := make(map[string]bool, len(adminEmails))
	for _, e := range adminEmails {
		set[strings.ToLower(e)] = true
	}
	return &Service{
		users: users, resets: resets, verifications: verifications, jwtSecret: jwtSecret, accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL,
		adminEmails: set, mailer: mailer, frontendURL: frontendURL,
	}
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
		// Admins are Thiago's own operators — auto-verify both gates so
		// they aren't blocked from posting ads or trading the same day
		// they sign up.
		u.KYCStatus = domain.KYCStatusVerified
		u.EmailVerified = true
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, TokenPair{}, err
	}
	tokens, err := s.issueTokens(u)
	if err != nil {
		return nil, TokenPair{}, err
	}

	if u.EmailVerified {
		go func() {
			_ = s.mailer.Send(u.Email, "Welcome to Thiago Exchange",
				fmt.Sprintf("Hi %s,\n\nYour Thiago Exchange account is ready. Head to the Market tab to start trading.\n\n— Thiago Exchange", u.FullName))
		}()
	} else {
		go s.sendVerificationEmail(u)
	}

	return u, tokens, nil
}

// sendVerificationEmail issues a fresh token and emails the verify link. The
// email doubles as the welcome email for a brand-new account — there's no
// separate "welcome" send for unverified users, just this one with the
// action they actually need to take.
func (s *Service) sendVerificationEmail(u *domain.User) {
	ctx := context.Background()
	rawToken, err := randomToken()
	if err != nil {
		return
	}
	token := &domain.EmailVerificationToken{
		ID: uuid.New(), UserID: u.ID, TokenHash: hashToken(rawToken),
		ExpiresAt: time.Now().Add(emailVerificationTTL),
	}
	if err := s.verifications.Create(ctx, token); err != nil {
		return
	}
	link := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimSuffix(s.frontendURL, "/"), rawToken)
	_ = s.mailer.Send(u.Email, "Verify your email — Thiago Exchange",
		fmt.Sprintf("Hi %s,\n\nWelcome to Thiago Exchange. Verify your email to start trading (link valid for 24 hours):\n%s\n\nIf you didn't create this account, you can ignore this email.\n\n— Thiago Exchange", u.FullName, link))
}

// VerifyEmail marks the token's owner verified. Returns the user so the
// caller can decide what to do next (e.g. nothing — the frontend just shows
// a success state and the user logs in normally).
func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	token, err := s.verifications.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return domain.ErrInvalidInput
	}
	if token.Used || time.Now().After(token.ExpiresAt) {
		return domain.ErrInvalidInput
	}
	u, err := s.users.GetByID(ctx, token.UserID)
	if err != nil {
		return err
	}
	if !u.EmailVerified {
		u.EmailVerified = true
		if err := s.users.Update(ctx, u); err != nil {
			return err
		}
	}
	return s.verifications.MarkUsed(ctx, token.ID)
}

// ResendVerification always succeeds from the caller's perspective, whether
// or not the email exists or is already verified — same anti-enumeration
// reasoning as RequestPasswordReset.
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil || u.EmailVerified {
		return nil
	}
	go s.sendVerificationEmail(u)
	return nil
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

type UpdateProfileInput struct {
	FullName          string
	Phone             string
	BankName          string
	BankAccountNumber string
	BankAccountName   string
}

func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, in UpdateProfileInput) (*domain.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if in.FullName != "" {
		u.FullName = in.FullName
	}
	u.Phone = in.Phone
	u.BankName = in.BankName
	u.BankAccountNumber = in.BankAccountNumber
	u.BankAccountName = in.BankAccountName
	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// RequestPasswordReset always succeeds from the caller's perspective,
// whether or not the email exists — returning a different result for
// unknown emails would let someone enumerate registered users.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil
	}

	rawToken, err := randomToken()
	if err != nil {
		return err
	}
	reset := &domain.PasswordResetToken{
		ID: uuid.New(), UserID: u.ID, TokenHash: hashToken(rawToken),
		ExpiresAt: time.Now().Add(passwordResetTTL),
	}
	if err := s.resets.Create(ctx, reset); err != nil {
		return err
	}

	link := fmt.Sprintf("%s/reset-password?token=%s", strings.TrimSuffix(s.frontendURL, "/"), rawToken)
	go func() {
		_ = s.mailer.Send(u.Email, "Reset your Thiago Exchange password",
			fmt.Sprintf("Hi %s,\n\nReset your password here (valid for 1 hour):\n%s\n\nIf you didn't request this, you can ignore this email.\n\n— Thiago Exchange", u.FullName, link))
	}()
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if len(newPassword) < 8 {
		return domain.ErrInvalidInput
	}
	reset, err := s.resets.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return domain.ErrInvalidInput
	}
	if reset.Used || time.Now().After(reset.ExpiresAt) {
		return domain.ErrInvalidInput
	}
	u, err := s.users.GetByID(ctx, reset.UserID)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	return s.resets.MarkUsed(ctx, reset.ID)
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
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
