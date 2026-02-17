package processing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"io"
	"strings"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/ebook"
	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

const (
	readingsBaseDir   = "_output" // Assuming Reading.StoragePath is relative to this
	editionsOutputDir = "_output/editions"
)

// EditionProcessor handles the business logic for processing an edition,
// including generating the ebook and creating delivery records.
type EditionProcessor struct {
	EditionRepo  *datastore.EditionRepository
	ReadingRepo  *datastore.ReadingRepository
	DeliveryRepo *datastore.DeliveryRepository
	// Potentially EditionTemplateRepo to get default format/destination
	EditionTemplateRepo *datastore.EditionTemplateRepository
	Generator           *ebook.EditionGenerator
	// storageConfig StorageConfig
}

// NewEditionProcessor creates a new EditionProcessor.
func NewEditionProcessor(
	editionRepo *datastore.EditionRepository,
	readingRepo *datastore.ReadingRepository,
	deliveryRepo *datastore.DeliveryRepository,
	editionTemplateRepo *datastore.EditionTemplateRepository,
	generator *ebook.EditionGenerator,
	// storageCfg StorageConfig,
) *EditionProcessor {
	return &EditionProcessor{
		EditionRepo:         editionRepo,
		ReadingRepo:         readingRepo,
		DeliveryRepo:        deliveryRepo,
		EditionTemplateRepo: editionTemplateRepo,
		Generator:           generator,
		// storageConfig: storageCfg,
	}
}

// ProcessAndGenerateEdition fetches an edition's content, generates the ebook,
// and creates a pending delivery record.
func (ep *EditionProcessor) ProcessAndGenerateEdition(
	ctx context.Context,
	editionID string,
	// Explicitly pass targetFormat and destinationID for now to simplify.
	// Later, these could be derived from EditionTemplate or User defaults.
	targetFormat models.EditionFormat,
	deliveryDestinationID string,
) (*models.Delivery, error) {
	// 1. Fetch Edition
	edition, err := ep.EditionRepo.GetEditionByID(ctx, editionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("edition with ID %s not found: %w", editionID, err)
		}
		return nil, fmt.Errorf("failed to fetch edition %s: %w", editionID, err)
	}

	// 2. Fetch associated Readings
	readings, err := ep.EditionRepo.GetReadingsForEdition(ctx, editionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch readings for edition %s: %w", editionID, err)
	}
	if len(readings) == 0 {
		return nil, fmt.Errorf("no readings found for edition %s, cannot generate ebook", editionID)
	}

	// 3. Combine HTML content from all readings into a temporary file
	combinedHTMLPath, cleanupFunc, err := combineReadingsHTML(ctx, readings, editionID)
	if err != nil {
		return nil, fmt.Errorf("failed to combine readings for edition %s: %w", editionID, err)
	}
	defer cleanupFunc() // Ensure temp file is deleted

	// 4. Prepare EditionMetadata
	// Collect authors from all readings for the metadata
	var authors []string
	authorSet := make(map[string]struct{}) // Use a set to avoid duplicate authors
	for _, r := range readings {
		if r.Author != "" {
			if _, exists := authorSet[r.Author]; !exists {
				authors = append(authors, r.Author)
				authorSet[r.Author] = struct{}{}
			}
		}
	}
	authorString := strings.Join(authors, ", ")
	if authorString == "" {
		authorString = "Logos Reader" // Default author if none found
	}

	metadata := models.EditionMetadata{
		Title:  edition.Name, // Use edition name as ebook title
		Author: authorString,
		// Publisher: "Logos Ebooks", // Example
		// Language: "en", // Example
		// TODO: Add cover image support if needed
	}

	// 5. Define outputDir for editions (ensure it's absolute)
	absEditionsOutputDir, err := filepath.Abs(editionsOutputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for editions output directory '%s': %w", editionsOutputDir, err)
	}

	// 6. Call ep.Generator.GenerateEdition()
	log.Printf("INFO (EditionProcessor): Generating edition %s (%s) from combined readings (%s) to format %s", edition.ID, edition.Name, combinedHTMLPath, targetFormat)
	generatedFilePath, fileSize, genErr := ep.Generator.GenerateEdition(
		ctx,
		combinedHTMLPath, // Use the path to the temporary combined HTML file
		metadata,
		targetFormat,
		absEditionsOutputDir, // Pass absolute output directory
		edition.ID,
	)
	if genErr != nil {
		return nil, fmt.Errorf("failed to generate ebook for edition %s: %w", editionID, genErr)
	}

	// 7. If successful, create and save Delivery record
	newDelivery := models.Delivery{
		ID:                    uuid.NewString(),
		EditionID:             editionID,
		DeliveryDestinationID: deliveryDestinationID,
		CreatedAt:             time.Now().UTC(),
		Format:                targetFormat,
		FilePath:              generatedFilePath, // Store the absolute path from generator
		FileSize:              int(fileSize),     // Ensure type compatibility
		Status:                models.DeliveryStatusPending,
		// CompletedAt, StartedAt are nil initially
	}

	err = ep.DeliveryRepo.CreateDelivery(ctx, &newDelivery)
	if err != nil {
		// TODO: Consider cleanup? If delivery record fails, should we delete the generated ebook file?
		// For now, the file exists but there's no delivery record.
		log.Printf("ERROR (EditionProcessor): Generated ebook for edition %s at %s, but failed to create delivery record: %v", editionID, generatedFilePath, err)
		return nil, fmt.Errorf("failed to create delivery record for edition %s after generation: %w", editionID, err)
	}

	log.Printf("INFO (EditionProcessor): Successfully processed edition %s. Ebook: %s, Delivery pending: %s", editionID, generatedFilePath, newDelivery.ID)
	return &newDelivery, nil
}

// combineReadingsHTML reads the HTML content for each reading, combines them into
// a single HTML document, writes it to a temporary file, and returns the path
// to that file along with a cleanup function.
func combineReadingsHTML(ctx context.Context, readings []models.Reading, editionID string) (string, func(), error) {
	var combinedHTML strings.Builder
	combinedHTML.WriteString("<!DOCTYPE html><html><head><meta charset=\"UTF-8\"><title>Edition</title></head><body>")
	combinedHTML.WriteString(fmt.Sprintf("<h1>Edition: %s</h1>", editionID)) // Optional: Add edition title

	var validReadingsCount int
	for _, reading := range readings {
		if reading.StoragePath == "" {
			log.Printf("WARN (EditionProcessor): Skipping reading %s for edition %s: missing storage path", reading.ID, editionID)
			continue
		}
		if reading.Format != models.ReadingFormatHTML {
			log.Printf("WARN (EditionProcessor): Skipping reading %s for edition %s: format is %s, not HTML", reading.ID, editionID, reading.Format)
			continue
		}

		// Construct absolute path relative to readingsBaseDir
		absPath, err := filepath.Abs(filepath.Join(readingsBaseDir, reading.StoragePath))
		if err != nil {
			log.Printf("WARN (EditionProcessor): Skipping reading %s for edition %s: failed to get absolute path for '%s': %v", reading.ID, editionID, reading.StoragePath, err)
			continue // Skip this reading if path fails
		}

		// Check if file exists
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			log.Printf("WARN (EditionProcessor): Skipping reading %s for edition %s: HTML file '%s' not found", reading.ID, editionID, absPath)
			continue // Skip this reading if file doesn't exist
		}

		// Read the HTML content
		htmlFile, err := os.Open(absPath)
		if err != nil {
			log.Printf("WARN (EditionProcessor): Skipping reading %s for edition %s: failed to open HTML file '%s': %v", reading.ID, editionID, absPath, err)
			continue // Skip this reading if opening fails
		}
		defer htmlFile.Close() // Ensure file is closed after reading

		// Add a separator and title for each reading within the combined HTML
		combinedHTML.WriteString(fmt.Sprintf("<hr><h2>%s</h2>", reading.Title))
		if reading.Author != "" {
			combinedHTML.WriteString(fmt.Sprintf("<p><i>By %s</i></p>", reading.Author))
		}

		_, err = io.Copy(&combinedHTML, htmlFile)
		if err != nil {
			log.Printf("WARN (EditionProcessor): Skipping reading %s for edition %s: failed to read HTML content from '%s': %v", reading.ID, editionID, absPath, err)
			htmlFile.Close() // Close explicitly on error before continuing
			continue         // Skip this reading if reading fails
		}
		htmlFile.Close() // Close explicitly after successful read
		validReadingsCount++
	}

	combinedHTML.WriteString("</body></html>")

	if validReadingsCount == 0 {
		return "", func() {}, fmt.Errorf("no valid HTML readings found to combine for edition %s", editionID)
	}

	// Create a temporary file to store the combined HTML
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("edition-%s-*.html", editionID))
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to create temporary HTML file for edition %s: %w", editionID, err)
	}

	// Write the combined HTML to the temporary file
	_, err = tmpFile.WriteString(combinedHTML.String())
	if err != nil {
		tmpFile.Close() // Close before removing
		os.Remove(tmpFile.Name())
		return "", func() {}, fmt.Errorf("failed to write combined HTML to temporary file %s: %w", tmpFile.Name(), err)
	}

	// Close the file before returning its path
	err = tmpFile.Close()
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", func() {}, fmt.Errorf("failed to close temporary file %s: %w", tmpFile.Name(), err)
	}

	// Return the path and a function to remove the temp file
	tmpFilePath := tmpFile.Name()
	cleanupFunc := func() {
		log.Printf("DEBUG (EditionProcessor): Cleaning up temporary combined HTML file: %s", tmpFilePath)
		err := os.Remove(tmpFilePath)
		if err != nil && !os.IsNotExist(err) { // Don't log error if file already removed
			log.Printf("WARN (EditionProcessor): Failed to remove temporary combined HTML file %s: %v", tmpFilePath, err)
		}
	}

	log.Printf("INFO (EditionProcessor): Combined %d readings into temporary file: %s", validReadingsCount, tmpFilePath)
	return tmpFilePath, cleanupFunc, nil
}
