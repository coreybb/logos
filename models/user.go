package models

import "time"

type User struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Email      string    `json:"email"`
	EmailToken string    `json:"-"` // Not exposed in API responses
}
