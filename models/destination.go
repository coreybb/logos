package models

import "time"

type DeliveryDestination struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	IsDefault bool      `json:"is_default"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // email, api, webhook
}
