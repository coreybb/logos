package routehandlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/models"
	"github.com/coreybb/logos/webutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AllowedSenderHandler holds dependencies for managing a user's allowed sender whitelist.
type AllowedSenderHandler struct {
	Repo *datastore.AllowedSenderRepository
}

// NewAllowedSenderHandler creates a new AllowedSenderHandler.
func NewAllowedSenderHandler(repo *datastore.AllowedSenderRepository) *AllowedSenderHandler {
	return &AllowedSenderHandler{Repo: repo}
}

type createAllowedSenderRequest struct {
	Name         string `json:"name"`
	EmailPattern string `json:"email_pattern"`
}

// HandleCreateAllowedSender adds a new allowed sender rule for a user.
// Example route: POST /api/users/{userID}/allowed-senders
func (h *AllowedSenderHandler) HandleCreateAllowedSender(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "userID")
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid userID format in path")
	}

	var req createAllowedSenderRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if req.Name == "" {
		return webutil.ErrBadRequest("Name is required")
	}
	if req.EmailPattern == "" {
		return webutil.ErrBadRequest("Email pattern is required")
	}

	sender := models.AllowedSender{
		ID:           uuid.NewString(),
		UserID:       userID,
		CreatedAt:    time.Now().UTC(),
		Name:         req.Name,
		EmailPattern: strings.ToLower(req.EmailPattern),
	}

	err := h.Repo.CreateAllowedSender(r.Context(), &sender)
	if err != nil {
		if strings.Contains(err.Error(), "violates foreign key constraint") {
			return webutil.ErrNotFound("User not found.")
		}
		log.Printf("ERROR: Failed to create allowed sender for user %s: %v", userID, err)
		return webutil.ErrInternalServerWrap(fmt.Sprintf("Failed to create allowed sender: %v", err), err)
	}

	log.Printf("INFO: Allowed sender created for user %s: %s (%s)", userID, sender.Name, sender.EmailPattern)
	webutil.RespondWithJSON(w, http.StatusCreated, sender)
	return nil
}

// HandleDeleteAllowedSender removes an allowed sender rule for a user.
// Example route: DELETE /api/users/{userID}/allowed-senders/{id}
func (h *AllowedSenderHandler) HandleDeleteAllowedSender(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "userID")
	senderID := chi.URLParam(r, "id")

	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid userID format in path")
	}
	if _, err := uuid.Parse(senderID); err != nil {
		return webutil.ErrBadRequest("Invalid allowed sender ID format in path")
	}

	err := h.Repo.DeleteAllowedSender(r.Context(), senderID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "allowed sender not found") {
			return webutil.ErrNotFound("Allowed sender not found.")
		}
		log.Printf("ERROR: Failed to delete allowed sender %s for user %s: %v", senderID, userID, err)
		return webutil.ErrInternalServerWrap(fmt.Sprintf("Failed to delete allowed sender: %v", err), err)
	}

	log.Printf("INFO: Allowed sender %s deleted for user %s", senderID, userID)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// HandleGetAllowedSenders retrieves all allowed sender rules for a user.
// Example route: GET /api/users/{userID}/allowed-senders
func (h *AllowedSenderHandler) HandleGetAllowedSenders(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "userID")

	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid userID format in path")
	}

	senders, err := h.Repo.GetAllowedSendersByUserID(r.Context(), userID)
	if err != nil {
		log.Printf("ERROR: Failed to get allowed senders for user %s: %v", userID, err)
		return webutil.ErrInternalServerWrap("Failed to retrieve allowed senders", err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, senders)
	return nil
}
