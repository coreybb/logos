package models

import "time"

type ReadingSource struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`       // email, rss, api
	Identifier string    `json:"identifier"` // e.g., sender email for "email" type, feed URL for "rss"
}
