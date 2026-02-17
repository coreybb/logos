package storage

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/coreybb/logos/models" // Added import for models.ReadingFormat
)

// outputDirForStorage defines the base directory for storing processed content locally.
const outputDirForStorage = "_output"

// ContentStorer defines the interface for storing processed content.
type ContentStorer interface {
	// Store saves the content and returns the relative path where it was stored,
	// or an error if storage failed.
	// The format is used to determine the file extension.
	Store(userID, readingID string, contentBytes []byte, format models.ReadingFormat) (relativeStoragePath string, err error)
}

// LocalFileStorer implements ContentStorer for saving content to the local file system.
type LocalFileStorer struct {
	basePath string // Base path for storing files, e.g., "_output"
}

// NewLocalFileStorer creates a new LocalFileStorer.
// If basePath is empty, it defaults to outputDirForStorage.
func NewLocalFileStorer(basePath string) *LocalFileStorer {
	if basePath == "" {
		basePath = outputDirForStorage
	}
	return &LocalFileStorer{basePath: basePath}
}

// Store saves the contentBytes to a file within the local file system structure:
// <basePath>/readings/<userID>/<readingID>.<format_extension>
// It returns the relative path: readings/<userID>/<readingID>.<format_extension>
func (lfs *LocalFileStorer) Store(userID, readingID string, contentBytes []byte, format models.ReadingFormat) (string, error) {
	if userID == "" || readingID == "" {
		return "", fmt.Errorf("userID and readingID cannot be empty for storing content")
	}
	if format == "" { // Check against empty string, as ReadingFormat is string-based
		return "", fmt.Errorf("format cannot be empty for storing content")
	}

	// Convert models.ReadingFormat to string for file extension.
	// No special normalization like "html_reading" to "html" needed here
	// if ReadingFormat constants are already the desired file extensions (e.g., "html", "pdf").
	formatExtension := string(format)
	if formatExtension == "" { // Should be caught by the check above, but defensive.
		return "", fmt.Errorf("invalid format for file extension: cannot be empty string")
	}

	// Ensure there are no problematic characters if format was a complex string, though our enums are safe.
	// For safety, one might still sanitize formatExtension if it could come from untrusted sources,
	// but with our constants, it's fine.
	// cleanFormat := strings.ToLower(strings.TrimSpace(formatExtension))

	relativeDir := filepath.Join("readings", userID)
	fileName := readingID + "." + formatExtension // Use the direct format string
	relativeStoragePath := filepath.Join(relativeDir, fileName)

	fullStorageDir := filepath.Join(lfs.basePath, relativeDir)
	fullStoragePath := filepath.Join(fullStorageDir, fileName)

	if err := os.MkdirAll(fullStorageDir, os.ModePerm); err != nil {
		log.Printf("ERROR (LocalFileStorer): Failed to create storage directory '%s': %v", fullStorageDir, err)
		return "", fmt.Errorf("failed to create storage directory: %w", err)
	}

	if err := os.WriteFile(fullStoragePath, contentBytes, 0644); err != nil {
		log.Printf("ERROR (LocalFileStorer): Failed to write processed content to '%s': %v", fullStoragePath, err)
		return "", fmt.Errorf("failed to save processed content: %w", err)
	}

	log.Printf("INFO (LocalFileStorer): Saved processed content to: %s (Format: %s)", fullStoragePath, format)
	return relativeStoragePath, nil
}
