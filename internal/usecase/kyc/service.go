package kyc

import (
	"context"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
)

type Service struct {
	kyc   domain.KYCRepository
	users domain.UserRepository
}

func NewService(kyc domain.KYCRepository, users domain.UserRepository) *Service {
	return &Service{kyc: kyc, users: users}
}

type SubmitInput struct {
	UserID      uuid.UUID
	FullName    string
	IDType      string
	IDNumber    string
	DocumentURL string
}

func (s *Service) Submit(ctx context.Context, in SubmitInput) (*domain.KYCSubmission, error) {
	if in.FullName == "" || in.IDType == "" || in.IDNumber == "" || in.DocumentURL == "" {
		return nil, domain.ErrInvalidInput
	}
	sub := &domain.KYCSubmission{
		ID:          uuid.New(),
		UserID:      in.UserID,
		FullName:    in.FullName,
		IDType:      in.IDType,
		IDNumber:    in.IDNumber,
		DocumentURL: in.DocumentURL,
		Status:      domain.KYCStatusPending,
	}
	if err := s.kyc.Create(ctx, sub); err != nil {
		return nil, err
	}
	user, err := s.users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	user.KYCStatus = domain.KYCStatusPending
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Service) MyStatus(ctx context.Context, userID uuid.UUID) (*domain.KYCSubmission, error) {
	return s.kyc.GetLatestByUser(ctx, userID)
}

func (s *Service) ListPending(ctx context.Context) ([]*domain.KYCSubmission, error) {
	return s.kyc.ListPending(ctx, 0, 0)
}

func (s *Service) Review(ctx context.Context, submissionID, adminID uuid.UUID, approve bool, note string) (*domain.KYCSubmission, error) {
	sub, err := s.kyc.GetByID(ctx, submissionID)
	if err != nil {
		return nil, err
	}
	status := domain.KYCStatusRejected
	if approve {
		status = domain.KYCStatusVerified
	}
	sub.Status = status
	sub.ReviewNote = note
	sub.ReviewedBy = &adminID
	if err := s.kyc.Update(ctx, sub); err != nil {
		return nil, err
	}
	user, err := s.users.GetByID(ctx, sub.UserID)
	if err != nil {
		return nil, err
	}
	user.KYCStatus = status
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	return sub, nil
}
