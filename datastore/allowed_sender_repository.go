package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

// AllowedSenderRepository handles database operations for the allowed_senders table.
type AllowedSenderRepository struct {
	db *sql.DB
}

// NewAllowedSenderRepository creates a new AllowedSenderRepository.
func NewAllowedSenderRepository(db *sql.DB) *AllowedSenderRepository {
	return &AllowedSenderRepository{db: db}
}

// CreateAllowedSender inserts a new allowed sender record.
func (r *AllowedSenderRepository) CreateAllowedSender(ctx context.Context, sender *models.AllowedSender) error {
	if _, err := uuid.Parse(sender.ID); err != nil {
		return fmt.Errorf("invalid allowed sender ID format: %w", err)
	}
	if _, err := uuid.Parse(sender.UserID); err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}
	if sender.EmailPattern == "" {
		return fmt.Errorf("email pattern cannot be empty")
	}
	if sender.Name == "" {
		return fmt.Errorf("allowed sender name cannot be empty")
	}

	query := `
		INSERT INTO allowed_senders (id, user_id, created_at, name, email_pattern)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, sender.ID, sender.UserID, sender.CreatedAt, sender.Name, sender.EmailPattern)
	if err != nil {
		return fmt.Errorf("failed to insert allowed sender: %w", err)
	}
	return nil
}

// DeleteAllowedSender removes an allowed sender record for a user.
func (r *AllowedSenderRepository) DeleteAllowedSender(ctx context.Context, senderID string, userID string) error {
	if _, err := uuid.Parse(senderID); err != nil {
		return fmt.Errorf("invalid allowed sender ID format: %w", err)
	}
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}

	query := `DELETE FROM allowed_senders WHERE id = $1 AND user_id = $2`
	result, err := r.db.ExecContext(ctx, query, senderID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete allowed sender %s: %w", senderID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for delete allowed sender %s: %w", senderID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("allowed sender not found (ID: %s, UserID: %s): %w", senderID, userID, sql.ErrNoRows)
	}

	return nil
}

// GetAllowedSendersByUserID retrieves all allowed sender rules for a user.
func (r *AllowedSenderRepository) GetAllowedSendersByUserID(ctx context.Context, userID string) ([]models.AllowedSender, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	query := `
		SELECT id, user_id, created_at, name, email_pattern
		FROM allowed_senders
		WHERE user_id = $1
		ORDER BY name ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query allowed senders for user %s: %w", userID, err)
	}
	defer rows.Close()

	var senders []models.AllowedSender
	for rows.Next() {
		var sender models.AllowedSender
		if err := rows.Scan(&sender.ID, &sender.UserID, &sender.CreatedAt, &sender.Name, &sender.EmailPattern); err != nil {
			return nil, fmt.Errorf("failed to scan allowed sender row for user %s: %w", userID, err)
		}
		senders = append(senders, sender)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating allowed sender rows for user %s: %w", userID, err)
	}

	if senders == nil {
		senders = []models.AllowedSender{}
	}

	return senders, nil
}

// IsAllowedSender checks if a sender email matches any allowed sender pattern for a user.
// Patterns can be exact email addresses or use SQL LIKE wildcards (% for any characters).
// If the user has no allowed senders at all, returns false.
func (r *AllowedSenderRepository) IsAllowedSender(ctx context.Context, userID string, senderEmail string) (bool, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return false, fmt.Errorf("invalid user ID format: %w", err)
	}
	if senderEmail == "" {
		return false, fmt.Errorf("sender email cannot be empty")
	}

	senderEmail = strings.ToLower(senderEmail)

	query := `SELECT EXISTS (SELECT 1 FROM allowed_senders WHERE user_id = $1 AND LOWER($2) LIKE LOWER(email_pattern))`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, senderEmail).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check allowed sender status for user %s: %w", userID, err)
	}
	return exists, nil
}
