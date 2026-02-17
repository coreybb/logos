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

// Holds dependencies for reading route handlers.
type ReadingHandler struct {
	Repo *datastore.ReadingRepository
}

// Creates a new ReadingHandler.
func NewReadingHandler(repo *datastore.ReadingRepository) *ReadingHandler {
	return &ReadingHandler{Repo: repo}
}

func (h *ReadingHandler) HandleGetReadings(w http.ResponseWriter, r *http.Request) error {
	readings, err := h.Repo.GetReadings(r.Context())
	if err != nil {
		return fmt.Errorf("failed to retrieve readings: %w", err)
	}
	if readings == nil {
		readings = []models.Reading{}
	}
	webutil.RespondWithJSON(w, http.StatusOK, readings)
	return nil
}

func (h *ReadingHandler) HandleGetReading(w http.ResponseWriter, r *http.Request) error {
	readingID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(readingID); err != nil {
		return webutil.ErrBadRequest("Invalid reading ID format")
	}

	reading, err := h.Repo.GetReadingByID(r.Context(), readingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "reading not found") {
			return webutil.ErrNotFound("Reading not found")
		}
		return fmt.Errorf("failed to retrieve reading %s: %w", readingID, err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, reading)
	return nil
}

func (h *ReadingHandler) HandleGetUserReadings(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid user ID format")
	}

	readings, err := h.Repo.GetReadingsByUserID(r.Context(), userID)
	if err != nil {
		return fmt.Errorf("failed to retrieve readings for user %s: %w", userID, err)
	}
	if readings == nil {
		readings = []models.Reading{}
	}
	webutil.RespondWithJSON(w, http.StatusOK, readings)
	return nil
}

func (h *ReadingHandler) HandleCreateReading(w http.ResponseWriter, r *http.Request) error {
	var req models.Reading
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	// Validate required fields from the Reading model that are expected from the client
	// Note: ID and CreatedAt are set by the server.
	// Format will be set by more sophisticated ingestion logic later. For direct API, client might send it.
	if req.SourceID == "" || req.ContentHash == "" || req.Excerpt == "" || req.StoragePath == "" || req.Title == "" || req.Format == "" {
		return webutil.ErrBadRequest("Missing required fields (source_id, content_hash, excerpt, storage_path, title, format)")
	}

	// Validate ReadingFormat
	// This assumes ReadingFormat constants are strings like "html", "pdf"
	isValidFormat := false
	switch req.Format {
	case models.ReadingFormatHTML, models.ReadingFormatPDF, models.ReadingFormatEPUB,
		models.ReadingFormatMOBI, models.ReadingFormatTXT, models.ReadingFormatMD,
		models.ReadingFormatDOCX, models.ReadingFormatRTF:
		isValidFormat = true
	}
	if !isValidFormat {
		// Provide a more helpful error message listing valid formats
		return webutil.ErrBadRequest(fmt.Sprintf("Invalid format value. Must be one of: %s, %s, %s, %s, %s, %s, %s, %s.",
			models.ReadingFormatHTML, models.ReadingFormatPDF, models.ReadingFormatEPUB,
			models.ReadingFormatMOBI, models.ReadingFormatTXT, models.ReadingFormatMD,
			models.ReadingFormatDOCX, models.ReadingFormatRTF))
	}

	req.ID = uuid.NewString()
	req.CreatedAt = time.Now().UTC()

	err := h.Repo.CreateReading(r.Context(), &req)
	if err != nil {
		// TODO: Check for specific DB errors like unique constraint on ContentHash if applicable
		return fmt.Errorf("failed to create reading '%s': %w", req.Title, err)
	}

	// log.Printf("INFO: Reading created (via direct API): ID=%s, Title=%s", req.ID, req.Title)
	webutil.RespondWithJSON(w, http.StatusCreated, req)
	return nil
}
