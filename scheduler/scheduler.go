package scheduler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/delivery"
	"github.com/coreybb/logos/models"
	"github.com/coreybb/logos/processing"
	"github.com/google/uuid"
)

// Scheduler checks recurring edition templates and triggers
// edition creation, ebook generation, and delivery.
type Scheduler struct {
	editionTemplateRepo       *datastore.EditionTemplateRepository
	editionTemplateSourceRepo *datastore.EditionTemplateSourceRepository
	editionRepo               *datastore.EditionRepository
	readingRepo               *datastore.ReadingRepository
	destinationRepo           *datastore.DestinationRepository
	editionProcessor          *processing.EditionProcessor
	deliveryService           *delivery.DeliveryService
}

// New creates a new Scheduler with all required dependencies.
func New(
	editionTemplateRepo *datastore.EditionTemplateRepository,
	editionTemplateSourceRepo *datastore.EditionTemplateSourceRepository,
	editionRepo *datastore.EditionRepository,
	readingRepo *datastore.ReadingRepository,
	destinationRepo *datastore.DestinationRepository,
	editionProcessor *processing.EditionProcessor,
	deliveryService *delivery.DeliveryService,
) *Scheduler {
	return &Scheduler{
		editionTemplateRepo:       editionTemplateRepo,
		editionTemplateSourceRepo: editionTemplateSourceRepo,
		editionRepo:               editionRepo,
		readingRepo:               readingRepo,
		destinationRepo:           destinationRepo,
		editionProcessor:          editionProcessor,
		deliveryService:           deliveryService,
	}
}

// HandleTick is an HTTP handler that triggers a scheduler tick.
// Used by Cloud Scheduler or manual curl requests.
func (s *Scheduler) HandleTick(w http.ResponseWriter, r *http.Request) {
	log.Println("INFO (Scheduler): Tick triggered via HTTP")

	processed, err := s.Tick(r.Context())
	if err != nil {
		log.Printf("ERROR (Scheduler): Tick failed: %v", err)
		http.Error(w, "scheduler tick failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK: processed %d templates", processed)
}

// Tick runs a single scheduler cycle: checks all recurring templates
// and processes any that are due. Returns the number of templates processed.
func (s *Scheduler) Tick(ctx context.Context) (int, error) {
	templates, err := s.editionTemplateRepo.GetAllRecurringTemplates(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch recurring templates: %w", err)
	}

	if len(templates) == 0 {
		return 0, nil
	}

	processed := 0
	for i := range templates {
		if s.processTemplate(ctx, &templates[i]) {
			processed++
		}
	}

	return processed, nil
}

// processTemplate handles the full pipeline for a single template.
// Returns true if an edition was created and delivered.
func (s *Scheduler) processTemplate(ctx context.Context, template *models.EditionTemplate) bool {
	// 1. Find the last edition created for this template
	latestEdition, err := s.editionRepo.GetLatestEditionByTemplateID(ctx, template.ID)
	if err != nil {
		log.Printf("ERROR (Scheduler): Failed to get latest edition for template %s: %v", template.ID, err)
		return false
	}

	// 2. Determine the "since" cutoff time
	var since time.Time
	if latestEdition != nil {
		since = latestEdition.CreatedAt
	} else {
		since = template.CreatedAt
	}

	// 3. Check if this template is due for a new edition
	now := time.Now().UTC()
	if !isDue(template.DeliveryInterval, template.DeliveryTime, since, now) {
		return false
	}

	// 4. Get source IDs assigned to this template
	sourceIDs, err := s.editionTemplateSourceRepo.GetSourceIDsForTemplate(ctx, template.ID)
	if err != nil {
		log.Printf("ERROR (Scheduler): Failed to get source IDs for template %s: %v", template.ID, err)
		return false
	}
	if len(sourceIDs) == 0 {
		return false
	}

	// 5. Get new readings for this user since the cutoff, filtered by assigned sources
	readings, err := s.readingRepo.GetUserReadingsSinceBySourceIDs(ctx, template.UserID, since, sourceIDs)
	if err != nil {
		log.Printf("ERROR (Scheduler): Failed to get readings for user %s since %v: %v", template.UserID, since, err)
		return false
	}

	if len(readings) == 0 {
		return false
	}

	// 5. Get the user's default delivery destination
	defaultDest, err := s.destinationRepo.GetDefaultDestinationByUserID(ctx, template.UserID)
	if err != nil {
		log.Printf("ERROR (Scheduler): Failed to get default destination for user %s: %v", template.UserID, err)
		return false
	}
	if defaultDest == nil {
		log.Printf("WARN (Scheduler): No default destination for user %s, skipping template %s", template.UserID, template.ID)
		return false
	}

	// 6. Create a new edition
	editionName := fmt.Sprintf("%s - %s", template.Name, now.Format("Jan 2, 2006"))
	edition := models.Edition{
		ID:                uuid.NewString(),
		UserID:            template.UserID,
		Name:              editionName,
		EditionTemplateID: template.ID,
		CreatedAt:         now,
	}

	if err := s.editionRepo.CreateEdition(ctx, &edition, template.ID); err != nil {
		log.Printf("ERROR (Scheduler): Failed to create edition for template %s: %v", template.ID, err)
		return false
	}

	// 7. Add all readings to the edition
	for _, reading := range readings {
		if err := s.editionRepo.AddReadingToEdition(ctx, edition.ID, reading.ID); err != nil {
			log.Printf("ERROR (Scheduler): Failed to add reading %s to edition %s: %v", reading.ID, edition.ID, err)
		}
	}

	log.Printf("INFO (Scheduler): Created edition %s (%s) with %d readings for user %s",
		edition.ID, editionName, len(readings), template.UserID)

	// 8. Generate the ebook
	generatedDelivery, err := s.editionProcessor.ProcessAndGenerateEdition(ctx, edition.ID, template.Format, defaultDest.ID, template.ColorImages)
	if err != nil {
		log.Printf("ERROR (Scheduler): Failed to generate ebook for edition %s: %v", edition.ID, err)
		return false
	}

	// 9. Execute delivery
	if err := s.deliveryService.ExecuteDelivery(ctx, generatedDelivery); err != nil {
		log.Printf("ERROR (Scheduler): Delivery failed for edition %s: %v", edition.ID, err)
		return false
	}

	log.Printf("INFO (Scheduler): Successfully delivered edition %s (%s) to user %s",
		edition.ID, editionName, template.UserID)
	return true
}

// isDue determines whether a template should fire based on its interval,
// delivery time, and when it last ran.
func isDue(interval string, deliveryTime string, since time.Time, now time.Time) bool {
	targetHour, targetMin, targetSec, err := parseDeliveryTime(deliveryTime)
	if err != nil {
		log.Printf("WARN (Scheduler): Invalid delivery time %q: %v", deliveryTime, err)
		return false
	}

	var nextDue time.Time

	switch interval {
	case "every_five_minutes":
		nextDue = since.Add(5 * time.Minute)
		return !now.Before(nextDue)

	case "hourly":
		nextDue = time.Date(since.Year(), since.Month(), since.Day(), since.Hour(), targetMin, targetSec, 0, time.UTC)
		if !nextDue.After(since) {
			nextDue = nextDue.Add(1 * time.Hour)
		}

	case "daily":
		nextDue = time.Date(since.Year(), since.Month(), since.Day(), targetHour, targetMin, targetSec, 0, time.UTC)
		if !nextDue.After(since) {
			nextDue = nextDue.AddDate(0, 0, 1)
		}

	case "weekly":
		nextDue = time.Date(since.Year(), since.Month(), since.Day(), targetHour, targetMin, targetSec, 0, time.UTC)
		if !nextDue.After(since) {
			nextDue = nextDue.AddDate(0, 0, 7)
		}

	case "monthly":
		nextDue = time.Date(since.Year(), since.Month(), since.Day(), targetHour, targetMin, targetSec, 0, time.UTC)
		if !nextDue.After(since) {
			nextDue = nextDue.AddDate(0, 1, 0)
		}

	default:
		return false
	}

	return !now.Before(nextDue)
}

func parseDeliveryTime(deliveryTime string) (hour, min, sec int, err error) {
	// Try HH:MM:SS first
	t, err := time.Parse("15:04:05", deliveryTime)
	if err == nil {
		return t.Hour(), t.Minute(), t.Second(), nil
	}

	// Postgres TIME columns scan as "0000-01-01THH:MM:SSZ"
	t, err = time.Parse(time.RFC3339, deliveryTime)
	if err == nil {
		return t.Hour(), t.Minute(), t.Second(), nil
	}

	return 0, 0, 0, fmt.Errorf("failed to parse delivery time %q", deliveryTime)
}
