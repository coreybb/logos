package models

type Edition struct {
	ID                string `json:"id"`
	UserID            string `json:"user_id"`
	Name              string `json:"name"`
	EditionTemplateID string `json:"edition_template_id"`
}

// EditionMetadata contains metadata for generating an ebook.
type EditionMetadata struct {
	Title           string
	Author          string
	Publisher       string   // Optional
	Tags            []string // Optional
	Language        string   // Optional, ISO639 code
	CoverImageBytes []byte   // Optional, raw bytes of the cover image
}
