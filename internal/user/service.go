package user

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, req CreateUserRequest) (*Profile, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	req.Password = string(hashedPassword)

	return s.repo.Create(ctx, req)
}

func (s *Service) Login(ctx context.Context, username, password string) (int64, error) {
	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserNotFound):
			return 0, ErrUserNotFound
		default:
			return 0, ErrInternalServer
		}
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return 0, ErrInvalidPassword
	}

	return user.ID, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID int64, req UpdateUserRequest) (*Profile, error) {
	return s.repo.Update(ctx, userID, req)
}

func (s *Service) GetProfile(ctx context.Context, userID int64) (*Profile, error) {
	return s.repo.GetByID(ctx, userID)
}
