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

// EditionTemplateHandler holds dependencies for edition template route handlers.
type EditionTemplateHandler struct {
	Repo *datastore.EditionTemplateRepository
}

// NewEditionTemplateHandler creates a new EditionTemplateHandler.
func NewEditionTemplateHandler(repo *datastore.EditionTemplateRepository) *EditionTemplateHandler {
	return &EditionTemplateHandler{Repo: repo}
}

// createEditionTemplateRequest defines the expected structure for creating an edition template.
type createEditionTemplateRequest struct {
	UserID           string `json:"user_id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Format           string `json:"format"` // e.g., "epub", "mobi", "pdf"
	DeliveryInterval string `json:"delivery_interval"`
	DeliveryTime     string `json:"delivery_time"` // e.g., "07:00:00" (HH:MM:SS)
	IsRecurring      bool   `json:"is_recurring"`
}

// updateEditionTemplateRequest defines the expected structure for updating an edition template.
type updateEditionTemplateRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Format           string `json:"format"`
	DeliveryInterval string `json:"delivery_interval"`
	DeliveryTime     string `json:"delivery_time"`
	IsRecurring      bool   `json:"is_recurring"`
}

// HandleCreateEditionTemplate creates a new edition template.
func (h *EditionTemplateHandler) HandleCreateEditionTemplate(w http.ResponseWriter, r *http.Request) error {
	var req createEditionTemplateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if req.UserID == "" {
		return webutil.ErrBadRequest("UserID is required")
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format")
	}
	if strings.TrimSpace(req.Name) == "" {
		return webutil.ErrBadRequest("Template name is required")
	}

	editionFormat, ok := models.IsValidEditionFormat(req.Format)
	if !ok {
		return webutil.ErrBadRequest(fmt.Sprintf("Invalid format value. Must be one of: %s, %s, %s", models.EditionFormatEPUB, models.EditionFormatMOBI, models.EditionFormatPDF))
	}

	if strings.TrimSpace(req.DeliveryInterval) == "" {
		return webutil.ErrBadRequest("Delivery interval is required")
	}
	if strings.TrimSpace(req.DeliveryTime) == "" {
		return webutil.ErrBadRequest("Delivery time is required")
	}

	newTemplate := models.EditionTemplate{
		ID:               uuid.NewString(),
		UserID:           req.UserID,
		CreatedAt:        time.Now().UTC(),
		Name:             req.Name,
		Description:      req.Description,
		Format:           editionFormat, // Use validated and typed format
		DeliveryInterval: req.DeliveryInterval,
		DeliveryTime:     req.DeliveryTime,
		IsRecurring:      req.IsRecurring,
	}

	err := h.Repo.CreateEditionTemplate(r.Context(), &newTemplate)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "cannot be empty") {
			log.Printf("INFO: Validation error creating edition template for user %s: %v", req.UserID, err)
			// Return a 400 Bad Request for validation errors from the repo
			return webutil.ErrBadRequestWrap(fmt.Sprintf("Failed to create edition template: %v", err), err)
		} else {
			log.Printf("ERROR: Failed to create edition template for user %s: %v", req.UserID, err)
			// Return a generic 500 Internal Server Error for other DB errors
			return webutil.ErrInternalServerWrap("Failed to create edition template", err)
		}
	}

	log.Printf("INFO: Edition Template created: ID=%s, Name=%s, UserID=%s", newTemplate.ID, newTemplate.Name, newTemplate.UserID)
	webutil.RespondWithJSON(w, http.StatusCreated, newTemplate)
	return nil
}

func (h *EditionTemplateHandler) HandleGetEditionTemplateByID(w http.ResponseWriter, r *http.Request) error {
	templateID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "user_id_for_templates")

	if _, err := uuid.Parse(templateID); err != nil {
		return webutil.ErrBadRequest("Invalid edition template ID format")
	}
	if userID == "" {
		return webutil.ErrBadRequest("User ID path parameter ('user_id_for_templates') is required")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format in path")
	}

	template, err := h.Repo.GetEditionTemplateByID(r.Context(), templateID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Use ErrNotFound which returns 404
			return webutil.ErrNotFound("Edition template not found")
		} else {
			log.Printf("ERROR: Failed to get edition template %s for user %s: %v", templateID, userID, err)
			// Use ErrInternalServerWrap which returns 500
			return webutil.ErrInternalServerWrap("Failed to retrieve edition template", err)
		}
	}
	webutil.RespondWithJSON(w, http.StatusOK, template)
	return nil
}

func (h *EditionTemplateHandler) HandleGetEditionTemplatesByUserID(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "user_id_for_templates")

	if userID == "" {
		return webutil.ErrBadRequest("User ID path parameter ('user_id_for_templates') is required")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format in path")
	}

	templates, err := h.Repo.GetEditionTemplatesByUserID(r.Context(), userID)
	if err != nil {
		log.Printf("ERROR: Failed to get edition templates for user %s: %v", userID, err)
		return webutil.ErrInternalServerWrap("Failed to retrieve edition templates", err)
	}
	webutil.RespondWithJSON(w, http.StatusOK, templates)
	return nil
}

func (h *EditionTemplateHandler) HandleUpdateEditionTemplate(w http.ResponseWriter, r *http.Request) error {
	templateID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "user_id_for_templates")

	if _, err := uuid.Parse(templateID); err != nil {
		return webutil.ErrBadRequest("Invalid edition template ID format")
	}
	if userID == "" {
		return webutil.ErrBadRequest("User ID path parameter ('user_id_for_templates') is required")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format in path")
	}

	var req updateEditionTemplateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if strings.TrimSpace(req.Name) == "" {
		return webutil.ErrBadRequest("Name is required for update")
	}
	editionFormat, ok := models.IsValidEditionFormat(req.Format)
	if !ok {
		return webutil.ErrBadRequest(fmt.Sprintf("Invalid format value. Must be one of: %s, %s, %s", models.EditionFormatEPUB, models.EditionFormatMOBI, models.EditionFormatPDF))
	}
	if strings.TrimSpace(req.DeliveryInterval) == "" || strings.TrimSpace(req.DeliveryTime) == "" {
		return webutil.ErrBadRequest("Delivery_interval and delivery_time are required for update")
	}

	templateToUpdate := models.EditionTemplate{
		ID:               templateID,
		UserID:           userID,
		Name:             req.Name,
		Description:      req.Description,
		Format:           editionFormat, // Use validated and typed format
		DeliveryInterval: req.DeliveryInterval,
		DeliveryTime:     req.DeliveryTime,
		IsRecurring:      req.IsRecurring,
	}

	err := h.Repo.UpdateEditionTemplate(r.Context(), &templateToUpdate)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Use ErrNotFound for 404
			return webutil.ErrNotFound("Edition template not found or you do not have permission to update it.")
		} else if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "cannot be empty") {
			log.Printf("INFO: Validation error updating edition template %s for user %s: %v", templateID, userID, err)
			// Use ErrBadRequest for validation issues
			return webutil.ErrBadRequestWrap(fmt.Sprintf("Failed to update edition template: %v", err), err)
		} else {
			log.Printf("ERROR: Failed to update edition template %s for user %s: %v", templateID, userID, err)
			// Use ErrInternalServerWrap for other DB errors
			return webutil.ErrInternalServerWrap("Failed to update edition template", err)
		}
	}

	updatedTemplate, fetchErr := h.Repo.GetEditionTemplateByID(r.Context(), templateID, userID)
	if fetchErr != nil {
		// Update succeeded, but fetching failed. Log the error and return 204.
		// Also return the fetchErr so the MakeHandler wrapper can log it properly.
		log.Printf("ERROR: Failed to fetch updated edition template %s for user %s after successful update: %v", templateID, userID, fetchErr)
		w.WriteHeader(http.StatusNoContent) // Indicate success but no content to return
		return fmt.Errorf("failed to fetch updated template %s after update: %w", templateID, fetchErr) // Return error for logging
	}

	log.Printf("INFO: Edition Template updated: ID=%s, Name=%s", updatedTemplate.ID, updatedTemplate.Name)
	webutil.RespondWithJSON(w, http.StatusOK, updatedTemplate)
	return nil
}

func (h *EditionTemplateHandler) HandleDeleteEditionTemplate(w http.ResponseWriter, r *http.Request) error {
	templateID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "user_id_for_templates")

	if _, err := uuid.Parse(templateID); err != nil {
		return webutil.ErrBadRequest("Invalid edition template ID format")
	}
	if userID == "" {
		return webutil.ErrBadRequest("User ID path parameter ('user_id_for_templates') is required")
	}
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format in path")
	}

	err := h.Repo.DeleteEditionTemplate(r.Context(), templateID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Use ErrNotFound for 404
			return webutil.ErrNotFound("Edition template not found or you do not have permission to delete it.")
		} else {
			log.Printf("ERROR: Failed to delete edition template %s for user %s: %v", templateID, userID, err)
			// Use ErrInternalServerWrap for other DB errors
			return webutil.ErrInternalServerWrap("Failed to delete edition template", err)
		}
	}

	log.Printf("INFO: Edition Template deleted: ID=%s, UserID=%s", templateID, userID)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
