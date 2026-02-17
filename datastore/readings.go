package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid" // Assuming UUIDs for IDs
)

// ReadingRepository handles database operations for readings.
type ReadingRepository struct {
	db *sql.DB
}

// NewReadingRepository creates a new ReadingRepository.
func NewReadingRepository(db *sql.DB) *ReadingRepository {
	return &ReadingRepository{db: db}
}

// CreateReading inserts a new reading record.
// It assumes the caller provides all necessary fields including the generated ID.
func (r *ReadingRepository) CreateReading(ctx context.Context, reading *models.Reading) error {
	// Optional: Validate required fields or formats here if not done by caller
	if reading.ID == "" || reading.SourceID == "" || reading.ContentHash == "" || reading.Excerpt == "" || reading.StoragePath == "" || reading.Title == "" {
		return fmt.Errorf("missing required fields for creating reading")
	}
	if _, err := uuid.Parse(reading.ID); err != nil {
		return fmt.Errorf("invalid reading ID format: %w", err)
	}
	if _, err := uuid.Parse(reading.SourceID); err != nil {
		return fmt.Errorf("invalid reading source ID format: %w", err)
	}

	query := `
		INSERT INTO readings (
			id, reading_source_id, author, created_at, content_hash,
			excerpt, format, published_at, storage_path, title
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		reading.ID, reading.SourceID, reading.Author, reading.CreatedAt, reading.ContentHash,
		reading.Excerpt, string(reading.Format), reading.PublishedAt, reading.StoragePath, reading.Title,
	)
	if err != nil {
		// Add specific error checks, e.g., unique constraint on content_hash?
		return fmt.Errorf("failed to insert reading: %w", err)
	}
	return nil
}

// GetReadingByContentHash retrieves a reading by its content hash.
// Returns nil, nil if no reading is found with that hash.
func (r *ReadingRepository) GetReadingByContentHash(ctx context.Context, hash string) (*models.Reading, error) {
	if hash == "" {
		return nil, fmt.Errorf("content hash cannot be empty")
	}
	// SHA-256 hashes are 64 hex characters. Add length check for basic validation.
	if len(hash) != 64 {
		return nil, fmt.Errorf("invalid content hash format (expected 64 hex characters)")
	}

	query := `
		SELECT id, reading_source_id, author, created_at, content_hash,
		       excerpt, format, published_at, storage_path, title
		FROM readings
		WHERE content_hash = $1
		LIMIT 1
	` // LIMIT 1 just in case (though hash should be unique)
	var reading models.Reading
	var formatStr string
	row := r.db.QueryRowContext(ctx, query, hash)
	err := row.Scan(
		&reading.ID, &reading.SourceID, &reading.Author, &reading.CreatedAt,
		&reading.ContentHash, &reading.Excerpt, &formatStr, &reading.PublishedAt,
		&reading.StoragePath, &reading.Title,
	)
	reading.Format = models.ReadingFormat(formatStr)
	if err != nil {
		if err == sql.ErrNoRows {
			// Not finding a reading by hash is not an application error in this context.
			// It means the content is new.
			return nil, nil
		}
		// Other errors (connection issues, etc.) are actual errors.
		return nil, fmt.Errorf("failed to get reading by content hash: %w", err)
	}
	return &reading, nil
}

// GetReadingByID retrieves a reading by its ID.
func (r *ReadingRepository) GetReadingByID(ctx context.Context, readingID string) (*models.Reading, error) {
	if _, err := uuid.Parse(readingID); err != nil {
		return nil, fmt.Errorf("invalid reading ID format: %w", err)
	}

	query := `
		SELECT id, reading_source_id, author, created_at, content_hash,
		       excerpt, format, published_at, storage_path, title
		FROM readings
		WHERE id = $1
	`
	var reading models.Reading
	var formatStr string
	row := r.db.QueryRowContext(ctx, query, readingID)
	err := row.Scan(
		&reading.ID, &reading.SourceID, &reading.Author, &reading.CreatedAt,
		&reading.ContentHash, &reading.Excerpt, &formatStr, &reading.PublishedAt,
		&reading.StoragePath, &reading.Title,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("reading not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get reading by ID: %w", err)
	}
	reading.Format = models.ReadingFormat(formatStr)
	return &reading, nil
}

// GetReadings retrieves a list of readings, possibly paginated later.
// Currently retrieves all readings.
func (r *ReadingRepository) GetReadings(ctx context.Context) ([]models.Reading, error) {
	query := `
		SELECT id, reading_source_id, author, created_at, content_hash,
		       excerpt, format, published_at, storage_path, title
		FROM readings
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query readings: %w", err)
	}
	defer rows.Close()

	var readings []models.Reading
	for rows.Next() {
		var reading models.Reading
		var formatStr string
		if err := rows.Scan(
			&reading.ID, &reading.SourceID, &reading.Author, &reading.CreatedAt,
			&reading.ContentHash, &reading.Excerpt, &formatStr, &reading.PublishedAt,
			&reading.StoragePath, &reading.Title,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reading row: %w", err)
		}
		reading.Format = models.ReadingFormat(formatStr)
		readings = append(readings, reading)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reading rows: %w", err)
	}

	if readings == nil {
		readings = []models.Reading{}
	}

	return readings, nil
}

// GetReadingsByUserID retrieves readings associated with a specific user via the user_readings table.
func (r *ReadingRepository) GetReadingsByUserID(ctx context.Context, userID string) ([]models.Reading, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	query := `
		SELECT r.id, r.reading_source_id, r.author, r.created_at, r.content_hash,
		       r.excerpt, r.format, r.published_at, r.storage_path, r.title
		FROM readings r
		JOIN user_readings ur ON r.id = ur.reading_id
		WHERE ur.user_id = $1
		ORDER BY ur.received_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query readings for user %s: %w", userID, err)
	}
	defer rows.Close()

	var readings []models.Reading
	for rows.Next() {
		var reading models.Reading
		var formatStr string
		if err := rows.Scan(
			&reading.ID, &reading.SourceID, &reading.Author, &reading.CreatedAt,
			&reading.ContentHash, &reading.Excerpt, &formatStr, &reading.PublishedAt,
			&reading.StoragePath, &reading.Title,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reading row for user %s: %w", userID, err)
		}
		reading.Format = models.ReadingFormat(formatStr)
		readings = append(readings, reading)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reading rows for user %s: %w", userID, err)
	}

	if readings == nil {
		readings = []models.Reading{}
	}

	return readings, nil
}

// AddUserReading creates a link between a user and a reading.
func (r *ReadingRepository) AddUserReading(ctx context.Context, userID, readingID string, receivedAt time.Time) error {
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}
	if _, err := uuid.Parse(readingID); err != nil {
		return fmt.Errorf("invalid reading ID format: %w", err)
	}

	query := `
		INSERT INTO user_readings (user_id, reading_id, created_at, received_at)
		VALUES ($1, $2, NOW(), $3)
		ON CONFLICT (user_id, reading_id) DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query, userID, readingID, receivedAt)
	if err != nil {
		return fmt.Errorf("failed to add user reading link: %w", err)
	}
	return nil
}
