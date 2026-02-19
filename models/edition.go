package models

import "time"

type Edition struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	Name              string    `json:"name"`
	EditionTemplateID string    `json:"edition_template_id"`
	CreatedAt         time.Time `json:"created_at"`
}

// EditionMetadata contains metadata for generating an ebook.
type EditionMetadata struct {
	Title    string
	Author   string
	Date     string // Display date for title page
	Language string // Optional, ISO639 code
}
