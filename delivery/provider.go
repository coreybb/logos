package delivery

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/models"
	"github.com/google/uuid"
)

// DeliveryProvider is the adapter interface for delivery mechanisms.
// Implement this to add new delivery types (email, API, webhook, etc.).
type DeliveryProvider interface {
	// Type returns the destination type this provider handles (e.g. "email").
	Type() string
	// Deliver sends the file at filePath to recipientAddress.
	Deliver(ctx context.Context, filePath string, fileName string, recipientAddress string) error
}

// DeliveryService orchestrates delivery execution by selecting the
// appropriate provider and managing status transitions and attempt tracking.
type DeliveryService struct {
	providers       map[string]DeliveryProvider
	deliveryRepo    *datastore.DeliveryRepository
	destinationRepo *datastore.DestinationRepository
	attemptRepo     *datastore.DeliveryAttemptRepository
}

func NewDeliveryService(
	deliveryRepo *datastore.DeliveryRepository,
	destinationRepo *datastore.DestinationRepository,
	attemptRepo *datastore.DeliveryAttemptRepository,
	providers ...DeliveryProvider,
) *DeliveryService {
	providerMap := make(map[string]DeliveryProvider, len(providers))
	for _, p := range providers {
		providerMap[p.Type()] = p
	}
	return &DeliveryService{
		providers:       providerMap,
		deliveryRepo:    deliveryRepo,
		destinationRepo: destinationRepo,
		attemptRepo:     attemptRepo,
	}
}

// ExecuteDelivery looks up the destination, selects the right provider,
// sends the file, and updates delivery status and attempt records.
func (s *DeliveryService) ExecuteDelivery(ctx context.Context, d *models.Delivery) error {
	// Look up destination to get type and address.
	dest, emailAddress, err := s.destinationRepo.GetEmailDestinationDetails(ctx, d.DeliveryDestinationID)
	if err != nil {
		return fmt.Errorf("failed to look up destination %s: %w", d.DeliveryDestinationID, err)
	}

	provider, ok := s.providers[dest.Type]
	if !ok {
		return fmt.Errorf("no delivery provider registered for type %q", dest.Type)
	}

	// Resolve recipient address based on destination type.
	var recipientAddress string
	switch dest.Type {
	case "email":
		recipientAddress = emailAddress
	default:
		return fmt.Errorf("unsupported destination type %q", dest.Type)
	}

	// Mark as processing.
	now := time.Now().UTC()
	if err := s.deliveryRepo.UpdateDeliveryStatus(ctx, d.ID, models.DeliveryStatusProcessing, &now, nil); err != nil {
		log.Printf("WARN (DeliveryService): Failed to set processing status for delivery %s: %v", d.ID, err)
	}

	// Build a human-readable file name from the edition format.
	fileName := fmt.Sprintf("edition.%s", d.Format)

	// Execute delivery.
	deliverErr := provider.Deliver(ctx, d.FilePath, fileName, recipientAddress)

	// Record the attempt and update final status.
	completedAt := time.Now().UTC()
	attempt := models.DeliveryAttempt{
		ID:         uuid.NewString(),
		DeliveryID: d.ID,
		CreatedAt:  completedAt,
	}

	if deliverErr != nil {
		attempt.Status = string(models.DeliveryStatusFailed)
		attempt.ErrorMessage = deliverErr.Error()
		_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, d.ID, models.DeliveryStatusFailed, nil, &completedAt)
		log.Printf("ERROR (DeliveryService): Delivery %s failed: %v", d.ID, deliverErr)
	} else {
		attempt.Status = string(models.DeliveryStatusDelivered)
		_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, d.ID, models.DeliveryStatusDelivered, nil, &completedAt)
		log.Printf("INFO (DeliveryService): Delivery %s completed successfully to %s", d.ID, recipientAddress)
	}

	if err := s.attemptRepo.CreateAttempt(ctx, &attempt); err != nil {
		log.Printf("WARN (DeliveryService): Failed to record attempt for delivery %s: %v", d.ID, err)
	}

	return deliverErr
}
