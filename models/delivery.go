package models

import "time"

// DeliveryStatus defines the set of allowed statuses for a Delivery.
type DeliveryStatus string

const (
	DeliveryStatusPending    DeliveryStatus = "pending"
	DeliveryStatusProcessing DeliveryStatus = "processing"
	DeliveryStatusDelivered  DeliveryStatus = "delivered"
	DeliveryStatusFailed     DeliveryStatus = "failed"
)

type Delivery struct {
	ID                    string         `json:"id"`
	EditionID             string         `json:"edition_id"`
	CreatedAt             time.Time      `json:"created_at"`
	CompletedAt           *time.Time     `json:"completed_at,omitempty"`
	DeliveryDestinationID string         `json:"delivery_destination_id"`
	Format                EditionFormat  `json:"format"`
	FilePath              string         `json:"file_path"`
	FileSize              int            `json:"file_size"`
	StartedAt             *time.Time     `json:"started_at,omitempty"`
	Status                DeliveryStatus `json:"status"`
}
