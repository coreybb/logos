package routehandlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/models"
	"github.com/coreybb/logos/webutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SourceHandler struct {
	Repo *datastore.SourceRepository
}

func NewSourceHandler(repo *datastore.SourceRepository) *SourceHandler {
	return &SourceHandler{Repo: repo}
}

type createReadingSourceRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // e.g., email, rss, api
	Identifier string `json:"identifier"`
}

func (h *SourceHandler) HandleCreateSource(w http.ResponseWriter, r *http.Request) error {
	var req createReadingSourceRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if req.Name == "" || req.Type == "" || req.Identifier == "" {
		return webutil.ErrBadRequest("Missing required fields (name, type, identifier)")
	}
	// Validate Type enum value
	validTypes := map[string]bool{"api": true, "email": true, "rss": true}
	if !validTypes[req.Type] {
		return webutil.ErrBadRequest("Invalid source type")
	}

	newSource := models.ReadingSource{
		ID:        uuid.NewString(),
		CreatedAt:  time.Now().UTC(),
		Name:       req.Name,
		Type:       req.Type,
		Identifier: req.Identifier,
	}

	err := h.Repo.CreateReadingSource(r.Context(), &newSource)
	if err != nil {
		// Log the detailed error internally
		log.Printf("ERROR: Failed to create reading source '%s': %v", req.Name, err)
		// Return a generic internal server error to the client
		return webutil.ErrInternalServerWrap("Failed to create reading source", err)
	}

	log.Printf("INFO: Reading Source created: ID=%s, Name=%s", newSource.ID, newSource.Name)
	webutil.RespondWithJSON(w, http.StatusCreated, newSource)
	return nil
}

func (h *SourceHandler) HandleGetSources(w http.ResponseWriter, r *http.Request) error {
	sources, err := h.Repo.GetReadingSources(r.Context())
	if err != nil {
		log.Printf("ERROR: Failed to get reading sources: %v", err)
		return webutil.ErrInternalServerWrap("Failed to retrieve reading sources", err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, sources)
	return nil
}

func (h *SourceHandler) HandleGetSourceByID(w http.ResponseWriter, r *http.Request) error {
	sourceID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(sourceID); err != nil {
		return webutil.ErrBadRequest("Invalid source ID format")
	}

	source, err := h.Repo.GetReadingSourceByID(r.Context(), sourceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return webutil.ErrNotFound("Reading source not found")
		}
		// Log detailed error and return a generic one
		log.Printf("ERROR: Failed to get reading source %s: %v", sourceID, err)
		return webutil.ErrInternalServerWrap("Failed to retrieve reading source", err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, source)
	return nil
}
