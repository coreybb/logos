package datastore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

type DestinationRepository struct {
	db *sql.DB
}

func NewDestinationRepository(db *sql.DB) *DestinationRepository {
	return &DestinationRepository{db: db}
}

func (r *DestinationRepository) CreateEmailDestination(ctx context.Context, dest *models.DeliveryDestination, emailAddress string) error {
	if _, err := uuid.Parse(dest.ID); err != nil {
		return fmt.Errorf("invalid destination ID format: %w", err)
	}
	if _, err := uuid.Parse(dest.UserID); err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}

	if emailAddress == "" {
		return fmt.Errorf("email address cannot be empty")
	}
	if dest.Type != "email" {
		return fmt.Errorf("destination type must be 'email' for this function")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	// Defer rollback in case of errors
	defer tx.Rollback() // Rollback is safe even if Commit succeeds

	// 1. Unset other default destinations for this user if this one is default
	if dest.IsDefault {
		unsetQuery := `UPDATE delivery_destinations_base SET is_default = false WHERE user_id = $1 AND id != $2`
		if _, err := tx.ExecContext(ctx, unsetQuery, dest.UserID, dest.ID); err != nil {
			return fmt.Errorf("failed to unset other default destinations: %w", err)
		}
	}

	// 2. Insert into base table
	baseQuery := `
		INSERT INTO delivery_destinations_base (id, user_id, created_at, is_default, name, type)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = tx.ExecContext(ctx, baseQuery, dest.ID, dest.UserID, dest.CreatedAt, dest.IsDefault, dest.Name, dest.Type)
	if err != nil {
		return fmt.Errorf("failed to insert base destination: %w", err)
	}

	// 3. Insert into email_destinations table
	emailQuery := `INSERT INTO email_destinations (id, email_address) VALUES ($1, $2)`
	_, err = tx.ExecContext(ctx, emailQuery, dest.ID, emailAddress)
	if err != nil {
		// Could be FK violation if base insert failed silently, though unlikely
		return fmt.Errorf("failed to insert email destination details: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *DestinationRepository) GetDestinationsByUserID(ctx context.Context, userID string) ([]models.DeliveryDestination, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	query := `
		SELECT id, user_id, created_at, is_default, name, type
		FROM delivery_destinations_base
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query destinations for user %s: %w", userID, err)
	}
	defer rows.Close()

	var destinations []models.DeliveryDestination
	for rows.Next() {
		var dest models.DeliveryDestination
		if err := rows.Scan(&dest.ID, &dest.UserID, &dest.CreatedAt, &dest.IsDefault, &dest.Name, &dest.Type); err != nil {
			return nil, fmt.Errorf("failed to scan destination row for user %s: %w", userID, err)
		}
		destinations = append(destinations, dest)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating destination rows for user %s: %w", userID, err)
	}
	if destinations == nil {
		destinations = []models.DeliveryDestination{}
	}
	return destinations, nil
}

func (r *DestinationRepository) GetEmailDestinationDetails(ctx context.Context, destinationID string) (*models.DeliveryDestination, string, error) {
	if _, err := uuid.Parse(destinationID); err != nil {
		return nil, "", fmt.Errorf("invalid destination ID format: %w", err)
	}

	query := `
		SELECT b.id, b.user_id, b.created_at, b.is_default, b.name, b.type, e.email_address
		FROM delivery_destinations_base b
		JOIN email_destinations e ON b.id = e.id
		WHERE b.id = $1 AND b.type = 'email'
	`
	var dest models.DeliveryDestination
	var emailAddress string
	row := r.db.QueryRowContext(ctx, query, destinationID)
	err := row.Scan(
		&dest.ID, &dest.UserID, &dest.CreatedAt, &dest.IsDefault, &dest.Name, &dest.Type,
		&emailAddress,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", fmt.Errorf("email destination not found: %w", err)
		}
		return nil, "", fmt.Errorf("failed to get email destination details: %w", err)
	}
	return &dest, emailAddress, nil
}
