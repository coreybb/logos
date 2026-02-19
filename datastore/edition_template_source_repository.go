package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

// EditionTemplateSourceRepository handles database operations for the
// edition_template_sources join table.
type EditionTemplateSourceRepository struct {
	db *sql.DB
}

// NewEditionTemplateSourceRepository creates a new EditionTemplateSourceRepository.
func NewEditionTemplateSourceRepository(db *sql.DB) *EditionTemplateSourceRepository {
	return &EditionTemplateSourceRepository{db: db}
}

// AddSourceToTemplate associates a reading source with an edition template.
func (r *EditionTemplateSourceRepository) AddSourceToTemplate(ctx context.Context, templateID string, sourceID string, createdAt time.Time) error {
	if _, err := uuid.Parse(templateID); err != nil {
		return fmt.Errorf("invalid edition template ID format: %w", err)
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return fmt.Errorf("invalid reading source ID format: %w", err)
	}
	if createdAt.IsZero() {
		return fmt.Errorf("created_at timestamp must be provided")
	}

	query := `
		INSERT INTO edition_template_sources (edition_template_id, reading_source_id, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (edition_template_id, reading_source_id) DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query, templateID, sourceID, createdAt)
	if err != nil {
		return fmt.Errorf("failed to add source %s to template %s: %w", sourceID, templateID, err)
	}

	return nil
}

// RemoveSourceFromTemplate removes the association between a reading source and an edition template.
func (r *EditionTemplateSourceRepository) RemoveSourceFromTemplate(ctx context.Context, templateID string, sourceID string) error {
	if _, err := uuid.Parse(templateID); err != nil {
		return fmt.Errorf("invalid edition template ID format: %w", err)
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return fmt.Errorf("invalid reading source ID format: %w", err)
	}

	query := `DELETE FROM edition_template_sources WHERE edition_template_id = $1 AND reading_source_id = $2`
	result, err := r.db.ExecContext(ctx, query, templateID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to remove source %s from template %s: %w", sourceID, templateID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for remove source operation (template %s, source %s): %w", templateID, sourceID, err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no source assignment found for template %s and source %s: %w", templateID, sourceID, sql.ErrNoRows)
	}

	return nil
}

// GetSourcesForTemplate retrieves all reading sources assigned to a specific edition template.
func (r *EditionTemplateSourceRepository) GetSourcesForTemplate(ctx context.Context, templateID string) ([]models.ReadingSource, error) {
	if _, err := uuid.Parse(templateID); err != nil {
		return nil, fmt.Errorf("invalid edition template ID format: %w", err)
	}

	query := `
		SELECT rs.id, rs.created_at, rs.name, rs.type, rs.identifier
		FROM reading_sources rs
		JOIN edition_template_sources ets ON rs.id = ets.reading_source_id
		WHERE ets.edition_template_id = $1
		ORDER BY rs.name ASC
	`
	rows, err := r.db.QueryContext(ctx, query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sources for template %s: %w", templateID, err)
	}
	defer rows.Close()

	var sources []models.ReadingSource
	for rows.Next() {
		var source models.ReadingSource
		if err := rows.Scan(&source.ID, &source.CreatedAt, &source.Name, &source.Type, &source.Identifier); err != nil {
			return nil, fmt.Errorf("failed to scan source row for template %s: %w", templateID, err)
		}
		sources = append(sources, source)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating source rows for template %s: %w", templateID, err)
	}

	if sources == nil {
		sources = []models.ReadingSource{}
	}

	return sources, nil
}

// GetSourceIDsForTemplate retrieves just the source IDs assigned to a template.
// This is used by the scheduler to filter readings efficiently.
func (r *EditionTemplateSourceRepository) GetSourceIDsForTemplate(ctx context.Context, templateID string) ([]string, error) {
	if _, err := uuid.Parse(templateID); err != nil {
		return nil, fmt.Errorf("invalid edition template ID format: %w", err)
	}

	query := `SELECT reading_source_id FROM edition_template_sources WHERE edition_template_id = $1`
	rows, err := r.db.QueryContext(ctx, query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query source IDs for template %s: %w", templateID, err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan source ID for template %s: %w", templateID, err)
		}
		ids = append(ids, id)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating source ID rows for template %s: %w", templateID, err)
	}

	if ids == nil {
		ids = []string{}
	}

	return ids, nil
}
