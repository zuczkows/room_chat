package user

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrInvalidPassword   = errors.New("invalid password")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, req CreateUserRequest) (*User, error) {
	user, err := s.repo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, err
	}
	if user != nil {
		return nil, ErrUserAlreadyExists
	}

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
		return 0, err
	}
	if user == nil {
		return 0, ErrUserNotFound
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		fmt.Println(err)
		return 0, ErrInvalidPassword
	}

	return user.ID, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID int64, req UpdateUserRequest) (*User, error) {
	return s.repo.Update(ctx, userID, req)
}

func (s *Service) GetUser(ctx context.Context, userID int64) (*User, error) {
	return s.repo.GetByID(ctx, userID)
}
