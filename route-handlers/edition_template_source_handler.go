package routehandlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/webutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// EditionTemplateSourceHandler holds dependencies for managing the association
// between edition templates and reading sources.
type EditionTemplateSourceHandler struct {
	Repo *datastore.EditionTemplateSourceRepository
}

// NewEditionTemplateSourceHandler creates a new EditionTemplateSourceHandler.
func NewEditionTemplateSourceHandler(repo *datastore.EditionTemplateSourceRepository) *EditionTemplateSourceHandler {
	return &EditionTemplateSourceHandler{Repo: repo}
}

// HandleAddSourceToTemplate associates a reading source with an edition template.
// Example route: POST /api/edition-templates/{templateID}/sources/{sourceID}
func (h *EditionTemplateSourceHandler) HandleAddSourceToTemplate(w http.ResponseWriter, r *http.Request) error {
	templateID := chi.URLParam(r, "templateID")
	sourceID := chi.URLParam(r, "sourceID")

	if _, err := uuid.Parse(templateID); err != nil {
		return webutil.ErrBadRequest("Invalid templateID format in path")
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return webutil.ErrBadRequest("Invalid sourceID format in path")
	}

	createdAt := time.Now().UTC()
	err := h.Repo.AddSourceToTemplate(r.Context(), templateID, sourceID, createdAt)
	if err != nil {
		if strings.Contains(err.Error(), "violates foreign key constraint") {
			log.Printf("WARN: Attempt to add non-existent source %s to template %s (or vice-versa): %v", sourceID, templateID, err)
			return webutil.ErrNotFound("Edition template or reading source not found.")
		}
		log.Printf("ERROR: Failed to add source %s to template %s: %v", sourceID, templateID, err)
		return webutil.ErrInternalServerWrap(fmt.Sprintf("Failed to add source to template: %v", err), err)
	}

	log.Printf("INFO: Source %s added to template %s", sourceID, templateID)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// HandleRemoveSourceFromTemplate removes a reading source from an edition template.
// Example route: DELETE /api/edition-templates/{templateID}/sources/{sourceID}
func (h *EditionTemplateSourceHandler) HandleRemoveSourceFromTemplate(w http.ResponseWriter, r *http.Request) error {
	templateID := chi.URLParam(r, "templateID")
	sourceID := chi.URLParam(r, "sourceID")

	if _, err := uuid.Parse(templateID); err != nil {
		return webutil.ErrBadRequest("Invalid templateID format in path")
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return webutil.ErrBadRequest("Invalid sourceID format in path")
	}

	err := h.Repo.RemoveSourceFromTemplate(r.Context(), templateID, sourceID)
	if err != nil {
		if strings.Contains(err.Error(), "no source assignment found") {
			log.Printf("INFO: Attempt to remove source %s from template %s where no assignment existed.", sourceID, templateID)
			return webutil.ErrNotFound("Source assignment not found.")
		}
		log.Printf("ERROR: Failed to remove source %s from template %s: %v", sourceID, templateID, err)
		return webutil.ErrInternalServerWrap(fmt.Sprintf("Failed to remove source from template: %v", err), err)
	}

	log.Printf("INFO: Source %s removed from template %s", sourceID, templateID)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// HandleGetTemplateSources retrieves all reading sources assigned to a template.
// Example route: GET /api/edition-templates/{templateID}/sources
func (h *EditionTemplateSourceHandler) HandleGetTemplateSources(w http.ResponseWriter, r *http.Request) error {
	templateID := chi.URLParam(r, "templateID")

	if _, err := uuid.Parse(templateID); err != nil {
		return webutil.ErrBadRequest("Invalid templateID format in path")
	}

	sources, err := h.Repo.GetSourcesForTemplate(r.Context(), templateID)
	if err != nil {
		log.Printf("ERROR: Failed to get sources for template %s: %v", templateID, err)
		return webutil.ErrInternalServerWrap("Failed to retrieve template sources", err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, sources)
	return nil
}
