package routehandlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/delivery"
	"github.com/coreybb/logos/models"
	"github.com/coreybb/logos/processing"
	"github.com/coreybb/logos/webutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Dfines the (optional) payload for the generate endpoint.
// We might want to allow specifying format or destination if not using template defaults.
type generateEditionRequest struct {
	Format                string `json:"format"`                  // e.g., "mobi", "epub", "pdf" - overrides template's default
	DeliveryDestinationID string `json:"delivery_destination_id"` // Optional: specific destination
}

type EditionHandler struct {
	Repo            *datastore.EditionRepository
	Processor       *processing.EditionProcessor
	DeliveryService *delivery.DeliveryService
}

func NewEditionHandler(repo *datastore.EditionRepository, processor *processing.EditionProcessor, deliveryService *delivery.DeliveryService) *EditionHandler {
	return &EditionHandler{Repo: repo, Processor: processor, DeliveryService: deliveryService}
}

type createEditionRequest struct {
	Name              string `json:"name"`
	UserID            string `json:"user_id"`
	EditionTemplateID string `json:"edition_template_id"`
}

type addReadingToEditionRequest struct {
	ReadingID string `json:"reading_id"`
}

func (h *EditionHandler) HandleGetEditions(w http.ResponseWriter, r *http.Request) error {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		return webutil.ErrBadRequest("user_id query parameter is required")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid user_id format in query parameter")
	}

	editions, err := h.Repo.GetEditionsByUserID(r.Context(), userID)
	if err != nil {
		return fmt.Errorf("failed to retrieve editions for user %s: %w", userID, err)
	}
	if editions == nil {
		editions = []models.Edition{}
	}
	webutil.RespondWithJSON(w, http.StatusOK, editions)
	return nil
}

// Triggers the generation of an ebook for a given edition.
func (h *EditionHandler) HandleGenerateEditionDocument(w http.ResponseWriter, r *http.Request) error {
	editionID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(editionID); err != nil {
		return webutil.ErrBadRequest("Invalid edition ID format")
	}

	var req generateEditionRequest
	// Allow empty body for defaults, or body to specify overrides
	if r.ContentLength > 0 {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			// Ignore EOF error for empty body, otherwise it's a bad request
			if !errors.Is(err, io.EOF) {
				return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
			}
		}
		defer r.Body.Close()
	}

	// For now, we need to determine the target format and destination ID.
	// This logic would be more complex: fetch edition, then its template, then user's default destination.
	// Let's use request parameters if provided, otherwise hardcode defaults for now.

	var targetFormat models.EditionFormat
	if req.Format != "" {
		validFormat, ok := models.IsValidEditionFormat(req.Format)
		if !ok {
			return webutil.ErrBadRequest(fmt.Sprintf("Invalid format value. Must be one of: %s, %s, %s", models.EditionFormatEPUB, models.EditionFormatMOBI, models.EditionFormatPDF))
		}
		targetFormat = validFormat
	} else {
		// TODO: Fetch from Edition's template or a default
		targetFormat = models.EditionFormatMOBI // Default for now
	}

	deliveryDestinationID := req.DeliveryDestinationID
	if deliveryDestinationID == "" {
		// TODO: Fetch user's default destination ID or from template
		// For now, this will cause an error in ProcessAndGenerateEdition if it requires one.
		// Or ProcessAndGenerateEdition can have its own default.
		// Let's assume ProcessAndGenerateEdition needs it.
		// We'll use a placeholder UUID if not provided.
		// In a real scenario, you'd fetch the user's default or the template's default.
		// This is a temporary measure to allow the call to proceed.
		// A proper implementation would fetch this or make it truly optional in the processor.
		// Forcing it via request or having a default in processor might be better.
		// For this handler, let's return an error if not provided, to be explicit.
		return webutil.ErrBadRequest("delivery_destination_id is required for now if not derived from template/user defaults")
	}
	if _, err := uuid.Parse(deliveryDestinationID); err != nil {
		return webutil.ErrBadRequest("Invalid delivery_destination_id format")
	}

	generatedDelivery, err := h.Processor.ProcessAndGenerateEdition(r.Context(), editionID, targetFormat, deliveryDestinationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return webutil.ErrNotFound(err.Error())
		}
		return fmt.Errorf("failed to process and generate edition %s: %w", editionID, err)
	}

	// Execute delivery (send the ebook). Non-fatal: the ebook was generated
	// regardless of whether delivery succeeds.
	if deliverErr := h.DeliveryService.ExecuteDelivery(r.Context(), generatedDelivery); deliverErr != nil {
		log.Printf("WARN (EditionHandler): Delivery %s failed after generation: %v", generatedDelivery.ID, deliverErr)
	}

	webutil.RespondWithJSON(w, http.StatusAccepted, generatedDelivery)
	return nil
}

func (h *EditionHandler) HandleCreateEdition(w http.ResponseWriter, r *http.Request) error {
	var req createEditionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if req.Name == "" {
		return webutil.ErrBadRequest("Edition name is required")
	}
	if req.UserID == "" {
		return webutil.ErrBadRequest("UserID is required")
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format")
	}
	if req.EditionTemplateID == "" {
		return webutil.ErrBadRequest("EditionTemplateID is required")
	}
	if _, err := uuid.Parse(req.EditionTemplateID); err != nil {
		return webutil.ErrBadRequest("Invalid EditionTemplateID format")
	}

	newEdition := models.Edition{
		ID:                uuid.NewString(),
		UserID:            req.UserID,
		Name:              req.Name,
		EditionTemplateID: req.EditionTemplateID,
	}

	err := h.Repo.CreateEdition(r.Context(), &newEdition, req.EditionTemplateID)
	if err != nil {
		// TODO: Check for specific DB errors e.g. FK violation for edition_template_id
		return fmt.Errorf("failed to create edition '%s' for user %s: %w", newEdition.Name, newEdition.UserID, err)
	}

	webutil.RespondWithJSON(w, http.StatusCreated, newEdition)
	return nil
}

func (h *EditionHandler) HandleGetEdition(w http.ResponseWriter, r *http.Request) error {
	editionID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(editionID); err != nil {
		return webutil.ErrBadRequest("Invalid edition ID format")
	}

	edition, err := h.Repo.GetEditionByID(r.Context(), editionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "edition not found") {
			return webutil.ErrNotFound("Edition not found")
		}
		return fmt.Errorf("failed to retrieve edition %s: %w", editionID, err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, edition)
	return nil
}

func (h *EditionHandler) HandleGetEditionReadings(w http.ResponseWriter, r *http.Request) error {
	editionID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(editionID); err != nil {
		return webutil.ErrBadRequest("Invalid edition ID format")
	}

	// Optional: Check if edition exists first
	// _, err := h.Repo.GetEditionByID(r.Context(), editionID)
	// if err != nil { ... }

	readings, err := h.Repo.GetReadingsForEdition(r.Context(), editionID)
	if err != nil {
		return fmt.Errorf("failed to retrieve readings for edition %s: %w", editionID, err)
	}
	if readings == nil {
		readings = []models.Reading{}
	}
	webutil.RespondWithJSON(w, http.StatusOK, readings)
	return nil
}

func (h *EditionHandler) HandleAddReadingToEdition(w http.ResponseWriter, r *http.Request) error {
	editionID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(editionID); err != nil {
		return webutil.ErrBadRequest("Invalid edition ID format")
	}

	var req addReadingToEditionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if req.ReadingID == "" {
		return webutil.ErrBadRequest("reading_id is required")
	}
	if _, err := uuid.Parse(req.ReadingID); err != nil {
		return webutil.ErrBadRequest("Invalid reading_id format")
	}

	err := h.Repo.AddReadingToEdition(r.Context(), editionID, req.ReadingID)
	if err != nil {
		// This could be an FK violation if reading_id or edition_id doesn't exist.
		// The datastore's ON CONFLICT handles duplicates, so that won't error.
		// TODO: Distinguish FK violation (404 or 400) from other errors (500).
		// For now, treating as internal error.
		return fmt.Errorf("failed to add reading %s to edition %s: %w", req.ReadingID, editionID, err)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
