package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

// UserReadingSourceRepository handles database operations for the user_reading_sources join table.
type UserReadingSourceRepository struct {
	db *sql.DB
}

// NewUserReadingSourceRepository creates a new UserReadingSourceRepository.
func NewUserReadingSourceRepository(db *sql.DB) *UserReadingSourceRepository {
	return &UserReadingSourceRepository{db: db}
}

// SubscribeUserToSource creates an association between a user and a reading source,
// indicating the user has subscribed to the source.
// The `createdAt` timestamp should be provided by the caller (typically time.Now().UTC()).
func (r *UserReadingSourceRepository) SubscribeUserToSource(ctx context.Context, userID string, sourceID string, createdAt time.Time) error {
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return fmt.Errorf("invalid reading source ID format: %w", err)
	}
	if createdAt.IsZero() {
		return fmt.Errorf("created_at timestamp must be provided")
	}

	query := `
		INSERT INTO user_reading_sources (user_id, reading_source_id, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, reading_source_id) DO NOTHING
	`
	// Using ON CONFLICT DO NOTHING to gracefully handle cases where the user is already subscribed.
	// This means if the subscription already exists, the operation is a no-op and returns no error.

	_, err := r.db.ExecContext(ctx, query, userID, sourceID, createdAt)
	if err != nil {
		// This could catch other errors besides PK violation if ON CONFLICT wasn't used,
		// but with ON CONFLICT, it's less likely to be a simple duplicate error.
		// Foreign key violations (if user_id or reading_source_id don't exist) would still cause errors.
		return fmt.Errorf("failed to subscribe user %s to source %s: %w", userID, sourceID, err)
	}

	return nil
}

// UnsubscribeUserFromSource removes the association between a user and a reading source.
func (r *UserReadingSourceRepository) UnsubscribeUserFromSource(ctx context.Context, userID string, sourceID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return fmt.Errorf("invalid reading source ID format: %w", err)
	}

	query := `DELETE FROM user_reading_sources WHERE user_id = $1 AND reading_source_id = $2`
	result, err := r.db.ExecContext(ctx, query, userID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe user %s from source %s: %w", userID, sourceID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// Log this, but don't necessarily fail the operation if the delete itself didn't error
		// For example, if the DB doesn't support RowsAffected well with certain drivers/versions.
		// However, for DELETE, it's good to know if something was actually deleted.
		return fmt.Errorf("failed to get rows affected for unsubscribe operation (user %s, source %s): %w", userID, sourceID, err)
	}

	if rowsAffected == 0 {
		// This means the user wasn't subscribed to this source, or IDs were invalid.
		// We can treat this as "not found" to delete, which isn't necessarily an error for idempotency.
		// However, for clarity, returning an error indicates the pre-condition (subscription exists) wasn't met.
		return fmt.Errorf("no subscription found for user %s and source %s to unsubscribe, or IDs invalid: %w", userID, sourceID, sql.ErrNoRows)
	}

	return nil
}

// GetUserSubscribedSources retrieves all ReadingSource models a user is subscribed to.
// This requires joining user_reading_sources with reading_sources.
func (r *UserReadingSourceRepository) GetUserSubscribedSources(ctx context.Context, userID string) ([]models.ReadingSource, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	query := `
		SELECT rs.id, rs.created_at, rs.name, rs.type
		FROM reading_sources rs
		JOIN user_reading_sources urs ON rs.id = urs.reading_source_id
		WHERE urs.user_id = $1
		ORDER BY rs.name ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscribed sources for user %s: %w", userID, err)
	}
	defer rows.Close()

	var sources []models.ReadingSource
	for rows.Next() {
		var source models.ReadingSource
		if err := rows.Scan(&source.ID, &source.CreatedAt, &source.Name, &source.Type); err != nil {
			return nil, fmt.Errorf("failed to scan subscribed source row for user %s: %w", userID, err)
		}
		sources = append(sources, source)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating subscribed source rows for user %s: %w", userID, err)
	}

	if sources == nil {
		sources = []models.ReadingSource{} // Return empty slice, not nil
	}

	return sources, nil
}

// IsUserSubscribed checks if a user is subscribed to a specific source.
// Returns true if subscribed, false otherwise, and an error if the query fails.
func (r *UserReadingSourceRepository) IsUserSubscribed(ctx context.Context, userID string, sourceID string) (bool, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return false, fmt.Errorf("invalid user ID format: %w", err)
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return false, fmt.Errorf("invalid reading source ID format: %w", err)
	}

	query := `SELECT EXISTS (SELECT 1 FROM user_reading_sources WHERE user_id = $1 AND reading_source_id = $2)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, sourceID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check subscription status for user %s and source %s: %w", userID, sourceID, err)
	}
	return exists, nil
}
