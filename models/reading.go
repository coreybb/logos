package models

import "time"

// ReadingFormat defines the set of allowed formats for a Reading.
type ReadingFormat string

const (
	ReadingFormatHTML ReadingFormat = "html"
	ReadingFormatPDF  ReadingFormat = "pdf"
	ReadingFormatEPUB ReadingFormat = "epub"
	ReadingFormatMOBI ReadingFormat = "mobi"
	ReadingFormatTXT  ReadingFormat = "txt"
	ReadingFormatMD   ReadingFormat = "md"
	ReadingFormatDOCX ReadingFormat = "docx"
	ReadingFormatRTF  ReadingFormat = "rtf"
)

type Reading struct {
	ID          string        `json:"id"`
	SourceID    string        `json:"reading_source_id"`
	Author      string        `json:"author,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	ContentHash string        `json:"content_hash"`
	ContentBody string        `json:"-"`
	Excerpt     string        `json:"excerpt"`
	PublishedAt *time.Time    `json:"published_at,omitempty"`
	StoragePath string        `json:"storage_path"`
	Title       string        `json:"title"`
	Format      ReadingFormat `json:"format"`
}
