package models

import "time"

// EditionReading represents the association between an edition and a reading,
// indicating that a specific reading is part of a particular edition.
type EditionReading struct {
	EditionID string    `json:"edition_id"`
	ReadingID string    `json:"reading_id"`
	CreatedAt time.Time `json:"created_at"` // When the reading was added to the edition
}
