package models

import "time"

// UserReading represents the association between a user and a reading,
// indicating when the user received or was linked to that reading.
type UserReading struct {
	UserID     string    `json:"user_id"`
	ReadingID  string    `json:"reading_id"`
	CreatedAt  time.Time `json:"created_at"`  // When the link was created in the system
	ReceivedAt time.Time `json:"received_at"` // When the user 'received' or acknowledged the reading
}