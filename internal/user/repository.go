package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zuczkows/room-chat/internal/database"
)

var (
	ErrUserNotFound            = errors.New("user not found")
	ErrNickAlreadyExists       = errors.New("nick already exists")
	ErrUserAlreadyExists       = errors.New("user already exists")
	ErrUserOrNickAlreadyExists = errors.New("user or nick already exists")
	ErrInvalidPassword         = errors.New("invalid password")
	ErrInternalServer          = errors.New("internal server error")
	ErrMissingRequiredFields   = errors.New("required fields are missing")
)

type Repository interface {
	Create(ctx context.Context, req CreateUserRequest) (*Profile, error)
	Update(ctx context.Context, id int64, req UpdateUserRequest) (*Profile, error)
	GetByID(ctx context.Context, id int64) (*Profile, error)
	GetByUsername(ctx context.Context, username string) (*Profile, error)
	Delete(ctx context.Context, id int64) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) Repository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, req CreateUserRequest) (*Profile, error) {
	query := `
        INSERT INTO users (username, password_hash, nick, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
        RETURNING id, username, nick, created_at, updated_at`
	profile := Profile{}
	err := r.db.QueryRowContext(ctx, query, req.Username, req.Password, req.Nick).
		Scan(&profile.ID, &profile.Username, &profile.Nick, &profile.CreatedAt, &profile.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case database.UniqueViolation:
				return nil, ErrUserOrNickAlreadyExists
			case database.NotNullViolation:
				return nil, ErrMissingRequiredFields
			default:
				return nil, fmt.Errorf("%w: %s", ErrInternalServer, pgErr.Message)
			}
		}
		return nil, err
	}

	return &profile, nil

}

func (r *PostgresRepository) Update(ctx context.Context, id int64, req UpdateUserRequest) (*Profile, error) {
	query := `
        UPDATE users
        SET nick = $1, updated_at = NOW()
        WHERE id = $2
        RETURNING id, username, nick, created_at, updated_at`

	profile := Profile{}
	err := r.db.QueryRowContext(ctx, query, req.Nick, id).
		Scan(&profile.ID, &profile.Username, &profile.Nick, &profile.CreatedAt, &profile.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case database.UniqueViolation:
				return nil, ErrNickAlreadyExists
			default:
				return nil, fmt.Errorf("%w: %s", ErrInternalServer, pgErr.Message)
			}
		}
	}
	return &profile, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id int64) (*Profile, error) {
	query := `SELECT id, username, nick, created_at, updated_at FROM users WHERE id = $1`
	profile := Profile{}
	err := r.db.QueryRowContext(ctx, query, id).
		Scan(&profile.ID, &profile.Username, &profile.Nick, &profile.CreatedAt, &profile.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM users WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *PostgresRepository) GetByUsername(ctx context.Context, username string) (*Profile, error) {
	query := `SELECT id, username, nick, created_at, updated_at, password_hash FROM users WHERE username = $1`
	profile := Profile{}
	err := r.db.QueryRowContext(ctx, query, username).Scan(&profile.ID, &profile.Username, &profile.Nick, &profile.CreatedAt, &profile.UpdatedAt, &profile.Password)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, fmt.Errorf("%w: %s", ErrInternalServer, pgErr.Message)
		}
		return nil, err
	}
	return &profile, nil
}
