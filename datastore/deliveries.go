package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

type DeliveryRepository struct {
	db *sql.DB
}

func NewDeliveryRepository(db *sql.DB) *DeliveryRepository {
	return &DeliveryRepository{db: db}
}

func (r *DeliveryRepository) CreateDelivery(ctx context.Context, delivery *models.Delivery) error {
	if _, err := uuid.Parse(delivery.ID); err != nil {
		return fmt.Errorf("invalid delivery ID format: %w", err)
	}
	if _, err := uuid.Parse(delivery.EditionID); err != nil {
		return fmt.Errorf("invalid edition ID format: %w", err)
	}
	if _, err := uuid.Parse(delivery.DeliveryDestinationID); err != nil {
		return fmt.Errorf("invalid delivery destination ID format: %w", err)
	}

	// Validate EditionFormat (optional here, as it's typed, but good for defense if string could sneak in)
	// For now, assume models.Delivery.Format is already a valid models.EditionFormat.
	// Same for models.Delivery.Status being a valid models.DeliveryStatus.

	query := `
		INSERT INTO deliveries (
			id, edition_id, delivery_destination_id, created_at, completed_at,
			edition_format, file_path, file_size, started_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		delivery.ID, delivery.EditionID, delivery.DeliveryDestinationID, delivery.CreatedAt,
		delivery.CompletedAt, string(delivery.Format), delivery.FilePath, delivery.FileSize, // Convert EditionFormat to string
		delivery.StartedAt, string(delivery.Status), // Convert DeliveryStatus to string
	)
	if err != nil {
		return fmt.Errorf("failed to insert delivery: %w", err)
	}
	return nil
}

func (r *DeliveryRepository) GetDeliveryByID(ctx context.Context, deliveryID string) (*models.Delivery, error) {
	if _, err := uuid.Parse(deliveryID); err != nil {
		return nil, fmt.Errorf("invalid delivery ID format: %w", err)
	}

	query := `
		SELECT id, edition_id, delivery_destination_id, created_at, completed_at,
		       edition_format, file_path, file_size, started_at, status
		FROM deliveries
		WHERE id = $1
	`
	var delivery models.Delivery
	var formatStr string
	var statusStr string

	row := r.db.QueryRowContext(ctx, query, deliveryID)
	err := row.Scan(
		&delivery.ID, &delivery.EditionID, &delivery.DeliveryDestinationID, &delivery.CreatedAt,
		&delivery.CompletedAt, &formatStr, &delivery.FilePath, &delivery.FileSize,
		&delivery.StartedAt, &statusStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("delivery not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get delivery by ID: %w", err)
	}

	delivery.Format = models.EditionFormat(formatStr)
	delivery.Status = models.DeliveryStatus(statusStr)

	return &delivery, nil
}

func (r *DeliveryRepository) UpdateDeliveryStatus(ctx context.Context, deliveryID string, status models.DeliveryStatus, startedAt, completedAt *time.Time) error {
	if _, err := uuid.Parse(deliveryID); err != nil {
		return fmt.Errorf("invalid delivery ID format: %w", err)
	}

	// Optional: Validate the status string if it weren't already typed
	// For now, assume `status` is a valid models.DeliveryStatus value.

	query := `
		UPDATE deliveries
		SET status = $2, started_at = COALESCE($3, started_at), completed_at = COALESCE($4, completed_at)
		WHERE id = $1
	`
	result, err := r.db.ExecContext(ctx, query, deliveryID, string(status), startedAt, completedAt) // Convert DeliveryStatus to string
	if err != nil {
		return fmt.Errorf("failed to update delivery status for ID %s: %w", deliveryID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("WARN: Could not get rows affected for delivery status update %s: %v", deliveryID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("delivery not found for status update: %w", sql.ErrNoRows)
	}

	return nil
}
