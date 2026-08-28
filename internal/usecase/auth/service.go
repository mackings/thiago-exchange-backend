package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"thiagoexchange/backend/internal/domain"
)

type Service struct {
	users           domain.UserRepository
	jwtSecret       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewService(users domain.UserRepository, jwtSecret string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{users: users, jwtSecret: jwtSecret, accessTokenTTL: accessTTL, refreshTokenTTL: refreshTTL}
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
		Role:         domain.RoleUser,
		KYCStatus:    domain.KYCStatusUnverified,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, TokenPair{}, err
	}
	tokens, err := s.issueTokens(u)
	if err != nil {
		return nil, TokenPair{}, err
	}
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
	tokens, err := s.issueTokens(u)
	if err != nil {
		return nil, TokenPair{}, err
	}
	return u, tokens, nil
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
