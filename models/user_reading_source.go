package models

import "time"

// UserReadingSource represents the association between a user and a reading source,
// indicating that a user is subscribed to or interested in a particular source.
type UserReadingSource struct {
	UserID          string    `json:"user_id"`
	ReadingSourceID string    `json:"reading_source_id"`
	CreatedAt       time.Time `json:"created_at"` // When the user subscribed to the source
}
