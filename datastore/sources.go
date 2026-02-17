package datastore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

// SourceRepository handles database operations for reading_sources.
type SourceRepository struct {
	db *sql.DB
}

func NewSourceRepository(db *sql.DB) *SourceRepository {
	return &SourceRepository{db: db}
}

func (r *SourceRepository) CreateReadingSource(ctx context.Context, source *models.ReadingSource) error {
	if _, err := uuid.Parse(source.ID); err != nil {
		return fmt.Errorf("invalid reading source ID format: %w", err)
	}
	// Add validation for Name, Type enum, and Identifier
	if source.Name == "" {
		return fmt.Errorf("reading source name cannot be empty")
	}
	// Type validation (e.g., against an allowed list) could also be here or in handler
	if source.Identifier == "" {
		// Depending on policy, identifier might be optional for some types, or always required.
		// For now, assume it's required.
		return fmt.Errorf("reading source identifier cannot be empty")
	}
	// Consider adding validation for source.Type against allowed values if not done elsewhere

	query := `
		INSERT INTO reading_sources (id, created_at, name, type, identifier)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, source.ID, source.CreatedAt, source.Name, source.Type, source.Identifier)
	if err != nil {
		return fmt.Errorf("failed to insert reading source: %w", err)
	}
	return nil
}

func (r *SourceRepository) GetReadingSourceByID(ctx context.Context, sourceID string) (*models.ReadingSource, error) {
	if _, err := uuid.Parse(sourceID); err != nil {
		return nil, fmt.Errorf("invalid reading source ID format: %w", err)
	}
	query := `SELECT id, created_at, name, type, identifier FROM reading_sources WHERE id = $1`
	var source models.ReadingSource
	row := r.db.QueryRowContext(ctx, query, sourceID)
	err := row.Scan(&source.ID, &source.CreatedAt, &source.Name, &source.Type, &source.Identifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("reading source not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get reading source by ID: %w", err)
	}
	return &source, nil
}

// GetSourceByIdentifierAndType retrieves a reading source by its identifier and type.
// This is useful for finding a specific RSS feed by URL or an email source by sender address.
func (r *SourceRepository) GetSourceByIdentifierAndType(ctx context.Context, identifier string, sourceType string) (*models.ReadingSource, error) {
	if identifier == "" {
		return nil, fmt.Errorf("identifier cannot be empty")
	}
	if sourceType == "" {
		return nil, fmt.Errorf("source type cannot be empty")
	}
	// Optional: validate sourceType against allowed enum values

	query := `SELECT id, created_at, name, type, identifier FROM reading_sources WHERE identifier = $1 AND type = $2`
	var source models.ReadingSource
	row := r.db.QueryRowContext(ctx, query, identifier, sourceType)
	err := row.Scan(&source.ID, &source.CreatedAt, &source.Name, &source.Type, &source.Identifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("reading source not found for identifier '%s' and type '%s': %w", identifier, sourceType, err)
		}
		return nil, fmt.Errorf("failed to get reading source by identifier and type: %w", err)
	}
	return &source, nil
}

func (r *SourceRepository) GetReadingSources(ctx context.Context) ([]models.ReadingSource, error) {
	query := `SELECT id, created_at, name, type, identifier FROM reading_sources ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query reading sources: %w", err)
	}
	defer rows.Close()

	var sources []models.ReadingSource
	for rows.Next() {
		var source models.ReadingSource
		if err := rows.Scan(&source.ID, &source.CreatedAt, &source.Name, &source.Type, &source.Identifier); err != nil {
			return nil, fmt.Errorf("failed to scan reading source row: %w", err)
		}
		sources = append(sources, source)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reading source rows: %w", err)
	}
	if sources == nil {
		sources = []models.ReadingSource{}
	}
	return sources, nil
}
