package models

import (
	"strings"
	"time"
)

// EditionFormat defines the set of allowed formats for an Edition.
type EditionFormat string

const (
	EditionFormatEPUB EditionFormat = "epub"
	EditionFormatMOBI EditionFormat = "mobi"
	EditionFormatPDF  EditionFormat = "pdf"
)

// EditionTemplate represents the structure for an edition template,
// defining how and when an edition should be generated and delivered.
type EditionTemplate struct {
	ID               string        `json:"id"`
	UserID           string        `json:"user_id"`
	CreatedAt        time.Time     `json:"created_at"`
	Name             string        `json:"name"`
	Description      string        `json:"description,omitempty"`
	Format           EditionFormat `json:"format"`            // Corresponds to edition_format ENUM ('epub', 'mobi', 'pdf')
	DeliveryInterval string        `json:"delivery_interval"` // Corresponds to edition_delivery_interval ENUM ('hourly', 'daily', 'weekly', 'monthly')
	DeliveryTime     string        `json:"delivery_time"`     // SQL TIME type, represented as "HH:MM:SS" string
	IsRecurring      bool          `json:"is_recurring"`
}

// IsValidEditionFormat checks if the provided format string is a valid EditionFormat.
// It returns the typed EditionFormat and true if valid, otherwise an empty EditionFormat and false.
func IsValidEditionFormat(formatStr string) (EditionFormat, bool) {
	ef := EditionFormat(strings.ToLower(formatStr))
	switch ef {
	case EditionFormatEPUB, EditionFormatMOBI, EditionFormatPDF:
		return ef, true
	default:
		return "", false
	}
}
