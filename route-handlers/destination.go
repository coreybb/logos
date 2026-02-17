package routehandlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/models"
	"github.com/coreybb/logos/webutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type DestinationHandler struct {
	Repo *datastore.DestinationRepository
}

func NewDestinationHandler(repo *datastore.DestinationRepository) *DestinationHandler {
	return &DestinationHandler{Repo: repo}
}

type createDestinationRequest struct {
	UserID    string `json:"user_id"` // TODO: Get from auth context
	Name      string `json:"name"`
	Type      string `json:"type"` // email, api, webhook
	IsDefault bool   `json:"is_default"`

	// Type-specific fields (only one should be populated based on Type)
	EmailAddress string `json:"email_address,omitempty"`
	// ApiEndpoint string `json:"api_endpoint,omitempty"`
	// ApiKey      string `json:"api_key,omitempty"`
	// WebhookURL  string `json:"webhook_url,omitempty"`
}

type deliveryDestinationResponse struct {
	models.DeliveryDestination
	EmailAddress string `json:"email_address,omitempty"`
	// ApiEndpoint string `json:"api_endpoint,omitempty"`
	// WebhookURL  string `json:"webhook_url,omitempty"`
}

func (h *DestinationHandler) HandleCreateDestination(w http.ResponseWriter, r *http.Request) error {
	var req createDestinationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	// Validation
	if req.UserID == "" { // TODO: Replace with auth context
		return webutil.ErrBadRequest("UserID is required")
	}
	if req.Name == "" {
		return webutil.ErrBadRequest("Destination name is required")
	}
	validTypes := map[string]bool{"email": true, "api": true, "webhook": true} // Allow defined types
	if !validTypes[req.Type] {
		return webutil.ErrBadRequest("Invalid destination type")
	}

	baseDest := models.DeliveryDestination{
		ID:        uuid.NewString(),
		UserID:    req.UserID, // Use provided for now
		CreatedAt: time.Now().UTC(),
		IsDefault: req.IsDefault,
		Name:      req.Name,
		Type:      req.Type,
	}

	var err error
	switch req.Type {
	case "email":
		if req.EmailAddress == "" {
			return webutil.ErrBadRequest("email_address is required for type 'email'")
		}
		err = h.Repo.CreateEmailDestination(r.Context(), &baseDest, req.EmailAddress)
	case "api":
		// err = h.Repo.CreateApiDestination(r.Context(), &baseDest, req.ApiEndpoint, req.ApiKey)
		err = errors.New("API destination type not yet implemented") // Placeholder
	case "webhook":
		// err = h.Repo.CreateWebhookDestination(r.Context(), &baseDest, req.WebhookURL)
		err = errors.New("Webhook destination type not yet implemented") // Placeholder
	default:
		// Should be caught by validation, but belts and suspenders
		err = fmt.Errorf("unhandled destination type: %s", req.Type)
	}

	if err != nil {
		log.Printf("ERROR: Failed to create destination '%s' (type %s): %v", req.Name, req.Type, err)
		// TODO: Map specific errors (e.g., validation) to 400 Bad Request? Or handle specific repo errors?
		// For now, return internal server error, wrapping the original cause if possible.
		if errors.Is(err, sql.ErrNoRows) { // Example: Foreign key violation, etc. might be a bad request
			return webutil.ErrBadRequestWrap("Failed to create destination due to invalid reference", err)
		}
		// Check if it's one of the placeholder errors
		if err.Error() == "API destination type not yet implemented" || err.Error() == "Webhook destination type not yet implemented" {
			return webutil.NewHTTPErrorWrap(http.StatusNotImplemented, "Destination type not yet implemented", err)
		}
		// Generic internal error
		return webutil.ErrInternalServerWrap("Failed to create destination", err)
	}

	// Prepare response (fetch details to confirm creation)
	responsePayload := deliveryDestinationResponse{
		DeliveryDestination: baseDest, // Start with base info
	}
	// Add type-specific info based on request
	if req.Type == "email" {
		responsePayload.EmailAddress = req.EmailAddress
	} // Add cases for other types

	log.Printf("INFO: Destination created: ID=%s, Name=%s, Type=%s", baseDest.ID, baseDest.Name, baseDest.Type)
	webutil.RespondWithJSON(w, http.StatusCreated, responsePayload)
	return nil
}

func (h *DestinationHandler) HandleGetDestinations(w http.ResponseWriter, r *http.Request) error {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		return webutil.ErrBadRequest("user_id query parameter is required")
	}

	destinations, err := h.Repo.GetDestinationsByUserID(r.Context(), userID)
	if err != nil {
		log.Printf("ERROR: Failed to get destinations for user %s: %v", userID, err)
		return webutil.ErrInternalServerWrap("Failed to retrieve destinations", err)
	}

	// Note: This only returns base info. A more detailed endpoint might fetch specifics.
	webutil.RespondWithJSON(w, http.StatusOK, destinations)
	return nil
}

func (h *DestinationHandler) HandleGetDestinationByID(w http.ResponseWriter, r *http.Request) error {
	destID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(destID); err != nil {
		return webutil.ErrBadRequest("Invalid destination ID format")
	}

	// Need to determine the type to call the correct repo method.
	// Option 1: Fetch base first, then fetch details (2 queries)
	// Option 2: Try fetching each type detail until one succeeds (inefficient)
	// Option 3: Create a repo method that joins dynamically (complex SQL)
	// Option 4: Return only base info here, have separate endpoints for details?

	// Let's try fetching email details as an example. A real implementation
	// would need a more robust way to handle multiple types.
	baseDest, email, err := h.Repo.GetEmailDestinationDetails(r.Context(), destID)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || err.Error() == "email destination not found" {
			// Could try fetching other types here, or just return 404
			// Assuming for now only email exists, so 404 is appropriate if email lookup fails
			return webutil.ErrNotFound("Destination not found")
		}
		// Otherwise, it's an internal error
		log.Printf("ERROR: Failed to get destination details for %s: %v", destID, err)
		return webutil.ErrInternalServerWrap("Failed to retrieve destination details", err)
	}

	responsePayload := deliveryDestinationResponse{
		DeliveryDestination: *baseDest,
		EmailAddress:        email,
	}

	webutil.RespondWithJSON(w, http.StatusOK, responsePayload)
	return nil
}
