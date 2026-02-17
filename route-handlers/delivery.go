package routehandlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/models"
	"github.com/coreybb/logos/webutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type DeliveryHandler struct {
	Repo *datastore.DeliveryRepository
}

func NewDeliveryHandler(repo *datastore.DeliveryRepository) *DeliveryHandler {
	return &DeliveryHandler{Repo: repo}
}

type createDeliveryRequest struct {
	EditionID             string `json:"edition_id"`
	DeliveryDestinationID string `json:"delivery_destination_id"`
	Format                string `json:"format"` // e.g., "mobi", "epub", "pdf"
}

// updateDeliveryStatusRequest defines the payload for the PATCH status endpoint.
type updateDeliveryStatusRequest struct {
	Status string `json:"status"` // e.g., "processing", "delivered", "failed"
}

// isValidDeliveryStatus checks if the provided status string is a valid models.DeliveryStatus.
func isValidDeliveryStatus(statusStr string) (models.DeliveryStatus, bool) {
	ds := models.DeliveryStatus(strings.ToLower(statusStr))
	switch ds {
	case models.DeliveryStatusPending, models.DeliveryStatusProcessing, models.DeliveryStatusDelivered, models.DeliveryStatusFailed:
		return ds, true
	default:
		return "", false
	}
}

// HandleCreateDelivery creates a new delivery record.
func (h *DeliveryHandler) HandleCreateDelivery(w http.ResponseWriter, r *http.Request) error {
	var req createDeliveryRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if req.EditionID == "" || req.DeliveryDestinationID == "" || req.Format == "" {
		return webutil.ErrBadRequest("Missing required fields (edition_id, delivery_destination_id, format)")
	}

	editionFormat, ok := models.IsValidEditionFormat(req.Format) // Using centralized validator
	if !ok {
		return webutil.ErrBadRequest(fmt.Sprintf("Invalid format value. Must be one of: %s, %s, %s", models.EditionFormatEPUB, models.EditionFormatMOBI, models.EditionFormatPDF))
	}

	if _, err := uuid.Parse(req.EditionID); err != nil {
		return webutil.ErrBadRequest("Invalid edition_id format")
	}
	if _, err := uuid.Parse(req.DeliveryDestinationID); err != nil {
		return webutil.ErrBadRequest("Invalid delivery_destination_id format")
	}

	newDelivery := models.Delivery{
		ID:                    uuid.NewString(),
		EditionID:             req.EditionID,
		DeliveryDestinationID: req.DeliveryDestinationID,
		CreatedAt:             time.Now().UTC(),
		Format:                editionFormat,
		Status:                models.DeliveryStatusPending,
		FilePath:              "", // To be updated
		FileSize:              0,  // To be updated
		CompletedAt:           nil,
		StartedAt:             nil,
	}

	err := h.Repo.CreateDelivery(r.Context(), &newDelivery)
	if err != nil {
		return fmt.Errorf("failed to create delivery for edition %s: %w", req.EditionID, err)
	}

	webutil.RespondWithJSON(w, http.StatusCreated, newDelivery)
	return nil
}

func (h *DeliveryHandler) HandleGetDelivery(w http.ResponseWriter, r *http.Request) error {
	deliveryID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(deliveryID); err != nil {
		return webutil.ErrBadRequest("Invalid delivery ID format")
	}

	delivery, err := h.Repo.GetDeliveryByID(r.Context(), deliveryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "delivery not found") {
			return webutil.ErrNotFound("Delivery not found")
		}
		return fmt.Errorf("failed to retrieve delivery %s: %w", deliveryID, err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, delivery)
	return nil
}

func (h *DeliveryHandler) HandleUpdateDeliveryStatus(w http.ResponseWriter, r *http.Request) error {
	deliveryID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(deliveryID); err != nil {
		return webutil.ErrBadRequest("Invalid delivery ID format")
	}

	var req updateDeliveryStatusRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	deliveryStatus, ok := isValidDeliveryStatus(req.Status)
	if !ok {
		return webutil.ErrBadRequest(fmt.Sprintf("Invalid status value. Must be one of: %s, %s, %s, %s", models.DeliveryStatusPending, models.DeliveryStatusProcessing, models.DeliveryStatusDelivered, models.DeliveryStatusFailed))
	}

	var startedAt, completedAt *time.Time
	now := time.Now().UTC()

	if deliveryStatus == models.DeliveryStatusProcessing {
		startedAt = &now
	} else if deliveryStatus == models.DeliveryStatusDelivered || deliveryStatus == models.DeliveryStatusFailed {
		// TODO: Refine timestamp logic for StartedAt.
		// For now, only setting CompletedAt. Assumes StartedAt was set by a previous 'processing' update or is handled by repo.
		completedAt = &now
	}

	err := h.Repo.UpdateDeliveryStatus(r.Context(), deliveryID, deliveryStatus, startedAt, completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "not found") {
			return webutil.ErrNotFound("Delivery not found for status update")
		}
		return fmt.Errorf("failed to update status for delivery %s: %w", deliveryID, err)
	}

	updatedDelivery, fetchErr := h.Repo.GetDeliveryByID(r.Context(), deliveryID)
	if fetchErr != nil {
		// The update was successful, but fetching the updated record failed.
		// Return 204 No Content as per previous behavior, but signal error to adapter.
		// Adapter will see no headers written and will log the fetchErr.
		w.WriteHeader(http.StatusNoContent)
		return fmt.Errorf("failed to fetch delivery %s after status update: %w", deliveryID, fetchErr)
	}
	webutil.RespondWithJSON(w, http.StatusOK, updatedDelivery)
	return nil
}
