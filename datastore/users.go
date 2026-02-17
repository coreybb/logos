package datastore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/coreybb/logos/models"
)

type UserRepository struct {
	db *sql.DB // The actual database connection pool
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user *models.User, emailToken string) error {
	// The user model currently doesn't have EmailToken, but the schema does.
	// We need to decide if the token should be part of the model or passed separately.
	// Passing separately seems cleaner as it's often generated just before insertion.
	query := `
		INSERT INTO users (id, created_at, email, email_token)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.db.ExecContext(ctx, query, user.ID, user.CreatedAt, user.Email, emailToken)
	if err != nil {
		// Consider checking for specific DB errors like unique constraint violation if needed.
		return fmt.Errorf("failed to insert user: %w", err)
	}
	return nil
}

// GetUserByID retrieves a user by their ID.
func (r *UserRepository) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	query := `
		SELECT id, created_at, email
		FROM users
		WHERE id = $1
	`
	var user models.User
	row := r.db.QueryRowContext(ctx, query, userID)
	err := row.Scan(&user.ID, &user.CreatedAt, &user.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) GetUsers(ctx context.Context) ([]models.User, error) {
	query := `
		SELECT id, created_at, email
		FROM users
		ORDER BY created_at DESC
	` // Example ordering
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.CreatedAt, &user.Email); err != nil {
			// Log scan error? Return partial list? Fail fast?
			// Failing fast seems reasonable here.
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}
		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rows: %w", err)
	}

	return users, nil
}
