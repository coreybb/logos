package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

// EditionTemplateRepository handles database operations for edition templates.
type EditionTemplateRepository struct {
	db *sql.DB
}

// NewEditionTemplateRepository creates a new EditionTemplateRepository.
func NewEditionTemplateRepository(db *sql.DB) *EditionTemplateRepository {
	return &EditionTemplateRepository{db: db}
}

var (
	// This map is for validating the string representation of EditionFormat
	validEditionFormatStrings = map[string]bool{
		string(models.EditionFormatEPUB): true,
		string(models.EditionFormatMOBI): true,
		string(models.EditionFormatPDF):  true,
	}
	validEditionDeliveryInterval = map[string]bool{"hourly": true, "daily": true, "weekly": true, "monthly": true}
	timeRegex                    = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d):([0-5]\d)$`)
)

// CreateEditionTemplate inserts a new edition template record into the database.
func (r *EditionTemplateRepository) CreateEditionTemplate(ctx context.Context, template *models.EditionTemplate) error {
	if _, err := uuid.Parse(template.ID); err != nil {
		return fmt.Errorf("invalid template ID format: %w", err)
	}
	if _, err := uuid.Parse(template.UserID); err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}

	formatStr := string(template.Format)
	if !validEditionFormatStrings[strings.ToLower(formatStr)] {
		return fmt.Errorf("invalid edition format: %s. Must be one of: %s, %s, %s",
			formatStr, models.EditionFormatEPUB, models.EditionFormatMOBI, models.EditionFormatPDF)
	}
	if !validEditionDeliveryInterval[strings.ToLower(template.DeliveryInterval)] {
		return fmt.Errorf("invalid delivery interval: %s. Must be one of: hourly, daily, weekly, monthly", template.DeliveryInterval)
	}

	if !timeRegex.MatchString(template.DeliveryTime) {
		return fmt.Errorf("invalid delivery time format: %s. Must be HH:MM:SS", template.DeliveryTime)
	}

	if template.Name == "" {
		return fmt.Errorf("template name cannot be empty")
	}
	if template.CreatedAt.IsZero() {
		return fmt.Errorf("template CreatedAt timestamp must be set")
	}

	query := `
		INSERT INTO edition_templates (
			id, user_id, created_at, name, description,
			format, delivery_interval, delivery_time, is_recurring
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.db.ExecContext(ctx, query,
		template.ID,
		template.UserID,
		template.CreatedAt,
		template.Name,
		NewNullString(template.Description),
		strings.ToLower(formatStr), // Use the lowercased string form for DB
		strings.ToLower(template.DeliveryInterval),
		template.DeliveryTime,
		template.IsRecurring,
	)

	if err != nil {
		return fmt.Errorf("failed to insert edition template: %w", err)
	}
	return nil
}

func NewNullString(s string) sql.NullString {
	if len(s) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

func (r *EditionTemplateRepository) GetEditionTemplateByID(ctx context.Context, templateID string, userID string) (*models.EditionTemplate, error) {
	if _, err := uuid.Parse(templateID); err != nil {
		return nil, fmt.Errorf("invalid template ID format: %w", err)
	}
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	query := `
		SELECT id, user_id, created_at, name, description,
		       format, delivery_interval, delivery_time, is_recurring
		FROM edition_templates
		WHERE id = $1 AND user_id = $2
	`
	var t models.EditionTemplate
	var description sql.NullString
	var formatStr string // Scan into string first

	row := r.db.QueryRowContext(ctx, query, templateID, userID)
	err := row.Scan(
		&t.ID,
		&t.UserID,
		&t.CreatedAt,
		&t.Name,
		&description,
		&formatStr, // Scan into string
		&t.DeliveryInterval,
		&t.DeliveryTime,
		&t.IsRecurring,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("edition template not found for id %s and user_id %s: %w", templateID, userID, err)
		}
		return nil, fmt.Errorf("failed to get edition template by ID: %w", err)
	}

	if description.Valid {
		t.Description = description.String
	}
	t.Format = models.EditionFormat(formatStr) // Convert string to models.EditionFormat

	return &t, nil
}

func (r *EditionTemplateRepository) GetEditionTemplatesByUserID(ctx context.Context, userID string) ([]models.EditionTemplate, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	query := `
		SELECT id, user_id, created_at, name, description,
		       format, delivery_interval, delivery_time, is_recurring
		FROM edition_templates
		WHERE user_id = $1
		ORDER BY name ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query edition templates by user ID %s: %w", userID, err)
	}
	defer rows.Close()

	var templates []models.EditionTemplate
	for rows.Next() {
		var t models.EditionTemplate
		var description sql.NullString
		var formatStr string // Scan into string
		if err := rows.Scan(
			&t.ID,
			&t.UserID,
			&t.CreatedAt,
			&t.Name,
			&description,
			&formatStr, // Scan into string
			&t.DeliveryInterval,
			&t.DeliveryTime,
			&t.IsRecurring,
		); err != nil {
			return nil, fmt.Errorf("failed to scan edition template row for user ID %s: %w", userID, err)
		}
		if description.Valid {
			t.Description = description.String
		}
		t.Format = models.EditionFormat(formatStr) // Convert string to models.EditionFormat
		templates = append(templates, t)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating edition template rows for user ID %s: %w", userID, err)
	}

	if templates == nil {
		templates = []models.EditionTemplate{}
	}

	return templates, nil
}

func (r *EditionTemplateRepository) UpdateEditionTemplate(ctx context.Context, template *models.EditionTemplate) error {
	if _, err := uuid.Parse(template.ID); err != nil {
		return fmt.Errorf("invalid template ID format for update: %w", err)
	}
	if _, err := uuid.Parse(template.UserID); err != nil {
		return fmt.Errorf("invalid user ID format for update context: %w", err)
	}

	formatStr := string(template.Format)
	if !validEditionFormatStrings[strings.ToLower(formatStr)] {
		return fmt.Errorf("invalid edition format for update: %s. Must be one of: %s, %s, %s",
			formatStr, models.EditionFormatEPUB, models.EditionFormatMOBI, models.EditionFormatPDF)
	}
	if !validEditionDeliveryInterval[strings.ToLower(template.DeliveryInterval)] {
		return fmt.Errorf("invalid delivery interval for update: %s", template.DeliveryInterval)
	}
	if !timeRegex.MatchString(template.DeliveryTime) {
		return fmt.Errorf("invalid delivery time format for update: %s", template.DeliveryTime)
	}
	if template.Name == "" {
		return fmt.Errorf("template name cannot be empty for update")
	}

	query := `
		UPDATE edition_templates
		SET name = $1,
		    description = $2,
		    format = $3,
		    delivery_interval = $4,
		    delivery_time = $5,
		    is_recurring = $6
		WHERE id = $7 AND user_id = $8
	`
	result, err := r.db.ExecContext(ctx, query,
		template.Name,
		NewNullString(template.Description),
		strings.ToLower(formatStr), // Use lowercased string for DB
		strings.ToLower(template.DeliveryInterval),
		template.DeliveryTime,
		template.IsRecurring,
		template.ID,
		template.UserID,
	)
	if err != nil {
		return fmt.Errorf("failed to update edition template with ID %s: %w", template.ID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for edition template update ID %s: %w", template.ID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("edition template not found for update (ID: %s, UserID: %s), or no changes made: %w", template.ID, template.UserID, sql.ErrNoRows)
	}

	return nil
}

func (r *EditionTemplateRepository) DeleteEditionTemplate(ctx context.Context, templateID string, userID string) error {
	if _, err := uuid.Parse(templateID); err != nil {
		return fmt.Errorf("invalid template ID format for delete: %w", err)
	}
	if _, err := uuid.Parse(userID); err != nil {
		return fmt.Errorf("invalid user ID format for delete context: %w", err)
	}

	query := `DELETE FROM edition_templates WHERE id = $1 AND user_id = $2`
	result, err := r.db.ExecContext(ctx, query, templateID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete edition template with ID %s: %w", templateID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for edition template delete ID %s: %w", templateID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("edition template not found for delete (ID: %s, UserID: %s): %w", templateID, userID, sql.ErrNoRows)
	}

	return nil
}
