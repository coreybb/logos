package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

type EditionRepository struct {
	db *sql.DB
}

func NewEditionRepository(db *sql.DB) *EditionRepository {
	return &EditionRepository{db: db}
}

// CreateEdition inserts a new edition record into the database.
// Note: The Edition model currently only has ID, UserID, Name. The schema
// requires edition_template_id. This needs clarification: should the handler
// provide it? Should the model be updated? Assuming handler provides it for now.
func (r *EditionRepository) CreateEdition(ctx context.Context, edition *models.Edition, templateID string) error {
	if _, err := uuid.Parse(templateID); err != nil {
		return fmt.Errorf("invalid edition_template_id format: %w", err)
	}

	if edition.CreatedAt.IsZero() {
		edition.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO editions (id, user_id, edition_template_id, name, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, edition.ID, edition.UserID, templateID, edition.Name, edition.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert edition: %w", err)
	}
	return nil
}

// GetEditionByID retrieves an edition by its ID.
// It fetches fields present in the editions table.
func (r *EditionRepository) GetEditionByID(ctx context.Context, editionID string) (*models.Edition, error) {
	query := `
		SELECT id, user_id, name, edition_template_id, created_at
		FROM editions
		WHERE id = $1
	`
	var edition models.Edition
	row := r.db.QueryRowContext(ctx, query, editionID)
	err := row.Scan(&edition.ID, &edition.UserID, &edition.Name, &edition.EditionTemplateID, &edition.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("edition not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get edition by ID: %w", err)
	}
	return &edition, nil
}

func (r *EditionRepository) GetEditionsByUserID(ctx context.Context, userID string) ([]models.Edition, error) {
	query := `
		SELECT id, user_id, name, edition_template_id, created_at
		FROM editions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query editions by user ID: %w", err)
	}
	defer rows.Close()

	var editions []models.Edition
	for rows.Next() {
		var edition models.Edition
		if err := rows.Scan(&edition.ID, &edition.UserID, &edition.Name, &edition.EditionTemplateID, &edition.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan edition row: %w", err)
		}
		editions = append(editions, edition)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating edition rows: %w", err)
	}

	// Return empty slice, not nil, if no editions found
	if editions == nil {
		editions = []models.Edition{}
	}

	return editions, nil
}

func (r *EditionRepository) AddReadingToEdition(ctx context.Context, editionID, readingID string) error {
	// Validate IDs
	if _, err := uuid.Parse(editionID); err != nil {
		return fmt.Errorf("invalid edition ID format: %w", err)
	}
	if _, err := uuid.Parse(readingID); err != nil {
		return fmt.Errorf("invalid reading ID format: %w", err)
	}

	query := `
		INSERT INTO edition_readings (edition_id, reading_id, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (edition_id, reading_id) DO NOTHING
	` // Use NOW() for created_at, handle conflicts gracefully
	_, err := r.db.ExecContext(ctx, query, editionID, readingID)
	if err != nil {
		// Consider specific errors like foreign key violations if ON CONFLICT wasn't used
		return fmt.Errorf("failed to add reading to edition: %w", err)
	}
	return nil
}

// GetReadingsForEdition retrieves readings associated with a specific edition.
// This requires joining tables and returning models.Reading.
// This might belong in ReadingRepository or require collaboration between repositories.
// For now, let's put it here, but acknowledge the potential mismatch.
func (r *EditionRepository) GetReadingsForEdition(ctx context.Context, editionID string) ([]models.Reading, error) {
	query := `
		SELECT r.id, r.reading_source_id, r.author, r.created_at, r.content_hash,
		       r.content_body, r.excerpt, r.format, r.published_at, r.storage_path, r.title
		FROM readings r
		JOIN edition_readings er ON r.id = er.reading_id
		WHERE er.edition_id = $1
		ORDER BY r.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, editionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query readings for edition %s: %w", editionID, err)
	}
	defer rows.Close()

	var readings []models.Reading
	for rows.Next() {
		var reading models.Reading
		var formatStr string
		if err := rows.Scan(
			&reading.ID, &reading.SourceID, &reading.Author, &reading.CreatedAt,
			&reading.ContentHash, &reading.ContentBody, &reading.Excerpt, &formatStr, &reading.PublishedAt,
			&reading.StoragePath, &reading.Title,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reading row for edition %s: %w", editionID, err)
		}
		reading.Format = models.ReadingFormat(formatStr)
		readings = append(readings, reading)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reading rows for edition %s: %w", editionID, err)
	}

	if readings == nil {
		readings = []models.Reading{}
	}

	return readings, nil
}

// GetLatestEditionByTemplateID returns the most recent edition for a given template.
// Returns nil, nil if no editions exist for the template.
func (r *EditionRepository) GetLatestEditionByTemplateID(ctx context.Context, templateID string) (*models.Edition, error) {
	if _, err := uuid.Parse(templateID); err != nil {
		return nil, fmt.Errorf("invalid template ID format: %w", err)
	}

	query := `
		SELECT id, user_id, name, edition_template_id, created_at
		FROM editions
		WHERE edition_template_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var edition models.Edition
	row := r.db.QueryRowContext(ctx, query, templateID)
	err := row.Scan(&edition.ID, &edition.UserID, &edition.Name, &edition.EditionTemplateID, &edition.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest edition by template ID: %w", err)
	}
	return &edition, nil
}
