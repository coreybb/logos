package models

import "time"

// DeliveryAttempt represents an attempt to deliver an edition,
// logging its status and any potential errors.
type DeliveryAttempt struct {
	ID            string    `json:"id"`
	DeliveryID    string    `json:"delivery_id"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"` // Corresponds to delivery_status ENUM ('delivered', 'failed', 'pending', 'processing')
	ErrorMessage  string    `json:"error_message,omitempty"`
}
