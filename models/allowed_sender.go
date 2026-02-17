package models

import "time"

// AllowedSender represents a rule for an email pattern that a user
// has whitelisted for automatic content ingestion.
type AllowedSender struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	CreatedAt    time.Time `json:"created_at"`
	Name         string    `json:"name"`
	EmailPattern string    `json:"email_pattern"`
}