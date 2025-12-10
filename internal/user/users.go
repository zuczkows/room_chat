package user

import (
	"context"
	"errors"

	"github.com/zuczkows/room-chat/internal/protocol"
	"golang.org/x/crypto/bcrypt"
)

type Users struct {
	repo Repository
}

func NewUsers(repo Repository) *Users {
	return &Users{repo: repo}
}

func (s *Users) Register(ctx context.Context, req protocol.CreateUserRequest) (int64, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}
	req.Password = string(hashedPassword)

	return s.repo.Create(ctx, req)
}

func (s *Users) Login(ctx context.Context, username, password string) (*Profile, error) {
	user, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserNotFound):
			return nil, ErrUserNotFound
		default:
			return nil, ErrInternalServer
		}
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return nil, ErrInvalidPassword
	}

	return user, nil
}

func (s *Users) UpdateProfile(ctx context.Context, userID int64, req protocol.UpdateUserRequest) (*Profile, error) {
	return s.repo.Update(ctx, userID, req)
}
