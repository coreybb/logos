package models

import "time"

// EditionTemplateSource represents the association between an edition template
// and a reading source, indicating that content from this source should be
// included in editions generated from this template.
type EditionTemplateSource struct {
	EditionTemplateID string    `json:"edition_template_id"`
	ReadingSourceID   string    `json:"reading_source_id"`
	CreatedAt         time.Time `json:"created_at"`
}
