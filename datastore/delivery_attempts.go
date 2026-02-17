package datastore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

type DeliveryAttemptRepository struct {
	db *sql.DB
}

func NewDeliveryAttemptRepository(db *sql.DB) *DeliveryAttemptRepository {
	return &DeliveryAttemptRepository{db: db}
}

func (r *DeliveryAttemptRepository) CreateAttempt(ctx context.Context, attempt *models.DeliveryAttempt) error {
	if _, err := uuid.Parse(attempt.ID); err != nil {
		return fmt.Errorf("invalid attempt ID format: %w", err)
	}
	if _, err := uuid.Parse(attempt.DeliveryID); err != nil {
		return fmt.Errorf("invalid delivery ID format: %w", err)
	}

	query := `
		INSERT INTO delivery_attempts (id, delivery_id, created_at, status, error_message)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query,
		attempt.ID, attempt.DeliveryID, attempt.CreatedAt, attempt.Status, attempt.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to insert delivery attempt: %w", err)
	}
	return nil
}
