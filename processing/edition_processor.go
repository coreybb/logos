package processing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/ebook"
	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

// EditionProcessor handles the business logic for processing an edition,
// including generating the ebook and creating delivery records.
type EditionProcessor struct {
	EditionRepo         *datastore.EditionRepository
	ReadingRepo         *datastore.ReadingRepository
	DeliveryRepo        *datastore.DeliveryRepository
	EditionTemplateRepo *datastore.EditionTemplateRepository
	Generator           *ebook.EditionGenerator
}

// NewEditionProcessor creates a new EditionProcessor.
func NewEditionProcessor(
	editionRepo *datastore.EditionRepository,
	readingRepo *datastore.ReadingRepository,
	deliveryRepo *datastore.DeliveryRepository,
	editionTemplateRepo *datastore.EditionTemplateRepository,
	generator *ebook.EditionGenerator,
) *EditionProcessor {
	return &EditionProcessor{
		EditionRepo:         editionRepo,
		ReadingRepo:         readingRepo,
		DeliveryRepo:        deliveryRepo,
		EditionTemplateRepo: editionTemplateRepo,
		Generator:           generator,
	}
}

// ProcessAndGenerateEdition fetches an edition's content, generates the ebook,
// and creates a pending delivery record.
func (ep *EditionProcessor) ProcessAndGenerateEdition(
	ctx context.Context,
	editionID string,
	targetFormat models.EditionFormat,
	deliveryDestinationID string,
	colorImages bool,
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

	// 3. Prepare EditionMetadata
	var authors []string
	authorSet := make(map[string]struct{})
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
		authorString = "Logos"
	}

	metadata := models.EditionMetadata{
		Title:  edition.Name,
		Author: authorString,
		Date:   edition.CreatedAt.Format("January 2, 2006"),
	}

	// 4. Generate the EPUB
	outputDir := os.TempDir()
	log.Printf("INFO (EditionProcessor): Generating edition %s (%s) with %d readings", edition.ID, edition.Name, len(readings))

	generatedFilePath, fileSize, genErr := ep.Generator.GenerateEdition(
		ctx,
		readings,
		metadata,
		targetFormat,
		outputDir,
		edition.ID,
		colorImages,
	)
	if genErr != nil {
		return nil, fmt.Errorf("failed to generate ebook for edition %s: %w", editionID, genErr)
	}

	// 5. Create Delivery record
	newDelivery := models.Delivery{
		ID:                    uuid.NewString(),
		EditionID:             editionID,
		DeliveryDestinationID: deliveryDestinationID,
		CreatedAt:             time.Now().UTC(),
		Format:                targetFormat,
		FilePath:              generatedFilePath,
		FileSize:              int(fileSize),
		Status:                models.DeliveryStatusPending,
	}

	err = ep.DeliveryRepo.CreateDelivery(ctx, &newDelivery)
	if err != nil {
		log.Printf("ERROR (EditionProcessor): Generated ebook for edition %s at %s, but failed to create delivery record: %v", editionID, generatedFilePath, err)
		return nil, fmt.Errorf("failed to create delivery record for edition %s after generation: %w", editionID, err)
	}

	log.Printf("INFO (EditionProcessor): Successfully processed edition %s. Ebook: %s, Delivery pending: %s", editionID, generatedFilePath, newDelivery.ID)
	return &newDelivery, nil
}
