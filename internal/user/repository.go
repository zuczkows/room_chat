package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zuczkows/room-chat/internal/database"
)

type Repository interface {
	Create(ctx context.Context, req CreateUserRequest) (*User, error)
	Update(ctx context.Context, id int64, req UpdateUserRequest) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Delete(ctx context.Context, id int64) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) Repository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, req CreateUserRequest) (*User, error) {
	query := `
        INSERT INTO users (username, password_hash, nick, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
        RETURNING id, username, nick, created_at, updated_at`
	user := User{}
	err := r.db.QueryRowContext(ctx, query, req.Username, req.Password, req.Nick).
		Scan(&user.ID, &user.Username, &user.Nick, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case database.UniqueViolation:
				return nil, errors.New("username or nick already exists")
			case database.NotNullViolation:
				return nil, errors.New("required field is missing")
			default:
				return nil, fmt.Errorf("database error: %s", pgErr.Message)
			}
		}
		return nil, err
	}

	return &user, nil

}

func (r *PostgresRepository) Update(ctx context.Context, id int64, req UpdateUserRequest) (*User, error) {
	query := `
        UPDATE users
        SET nick = $1, updated_at = NOW()
        WHERE id = $2
        RETURNING id, username, nick, created_at, updated_at`

	user := &User{}
	err := r.db.QueryRowContext(ctx, query, req.Nick, id).
		Scan(&user.ID, &user.Username, &user.Nick, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case database.UniqueViolation:
				return nil, errors.New("nick already exists")
			default:
				return nil, fmt.Errorf("database error: %s", pgErr.Message)
			}
		}
		return nil, err
	}
	return user, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id int64) (*User, error) {
	query := `SELECT id, username, nick, created_at, updated_at FROM users WHERE id = $1`
	user := &User{}
	err := r.db.QueryRowContext(ctx, query, id).
		Scan(&user.ID, &user.Username, &user.Nick, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM users WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *PostgresRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `SELECT id, username, nick, created_at, updated_at, password_hash FROM users WHERE username = $1`
	user := &User{}
	err := r.db.QueryRowContext(ctx, query, username).Scan(&user.ID, &user.Username, &user.Nick, &user.CreatedAt, &user.UpdatedAt, &user.Password)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			return nil, fmt.Errorf("database error code: %s message: %s", pgErr.Code, pgErr.Message)
		}
		return nil, err
	}
	return user, nil
}
