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

// UserReadingSourceHandler holds dependencies for user-reading source association handlers.
type UserReadingSourceHandler struct {
	Repo *datastore.UserReadingSourceRepository
	// Potentially UserRepository and SourceRepository if we want to validate existence
	// of user and source before attempting to create the link, though FK constraints
	// in the DB will also catch this.
}

// NewUserReadingSourceHandler creates a new UserReadingSourceHandler.
func NewUserReadingSourceHandler(repo *datastore.UserReadingSourceRepository) *UserReadingSourceHandler {
	return &UserReadingSourceHandler{Repo: repo}
}

// HandleSubscribeUserToSource handles a request for a user to subscribe to a reading source.
// It expects userID and sourceID as URL parameters.
// Example route: POST /users/{userID}/subscriptions/{sourceID}
func (h *UserReadingSourceHandler) HandleSubscribeUserToSource(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "userID")     // Or a consistent param name like "user_id_param"
	sourceID := chi.URLParam(r, "sourceID") // Or a consistent param name like "source_id_param"

	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format in path")
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return webutil.ErrBadRequest("Invalid SourceID format in path")
	}

	createdAt := time.Now().UTC()
	err := h.Repo.SubscribeUserToSource(r.Context(), userID, sourceID, createdAt)
	if err != nil {
		// The repository's ON CONFLICT DO NOTHING handles duplicates gracefully (no error).
		// An error here likely means a FK violation (user or source doesn't exist) or other DB issue.
		if strings.Contains(err.Error(), "violates foreign key constraint") {
			log.Printf("WARN: Attempt to subscribe user %s to non-existent source %s (or vice-versa): %v", userID, sourceID, err)
			return webutil.ErrNotFound("User or Source not found, or subscription failed due to invalid reference.")
		} else {
			log.Printf("ERROR: Failed to subscribe user %s to source %s: %v", userID, sourceID, err)
			return webutil.ErrInternalServerWrap(fmt.Sprintf("Failed to subscribe to source: %v", err), err)
		}
	}

	log.Printf("INFO: User %s subscribed to Source %s", userID, sourceID)
	w.WriteHeader(http.StatusNoContent) // 204 No Content is appropriate for successful subscription
	return nil
}

// HandleUnsubscribeUserFromSource handles a request for a user to unsubscribe from a reading source.
// Example route: DELETE /users/{userID}/subscriptions/{sourceID}
func (h *UserReadingSourceHandler) HandleUnsubscribeUserFromSource(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "userID")
	sourceID := chi.URLParam(r, "sourceID")

	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format in path")
	}
	if _, err := uuid.Parse(sourceID); err != nil {
		return webutil.ErrBadRequest("Invalid SourceID format in path")
	}

	err := h.Repo.UnsubscribeUserFromSource(r.Context(), userID, sourceID)
	if err != nil {
		// The repo method returns sql.ErrNoRows if the subscription didn't exist.
		if strings.Contains(err.Error(), "no subscription found") { // Or errors.Is(err, sql.ErrNoRows) if repo returns it directly
			log.Printf("INFO: Attempt to unsubscribe user %s from source %s where no subscription existed.", userID, sourceID)
			return webutil.ErrNotFound("Subscription not found to unsubscribe.")
		} else {
			log.Printf("ERROR: Failed to unsubscribe user %s from source %s: %v", userID, sourceID, err)
			return webutil.ErrInternalServerWrap(fmt.Sprintf("Failed to unsubscribe from source: %v", err), err)
		}
	}

	log.Printf("INFO: User %s unsubscribed from Source %s", userID, sourceID)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// HandleGetUserSubscribedSources retrieves all reading sources a user is subscribed to.
// Example route: GET /users/{userID}/subscriptions
func (h *UserReadingSourceHandler) HandleGetUserSubscribedSources(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "userID")

	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid UserID format in path")
	}

	sources, err := h.Repo.GetUserSubscribedSources(r.Context(), userID)
	if err != nil {
		log.Printf("ERROR: Failed to get subscribed sources for user %s: %v", userID, err)
		return webutil.ErrInternalServerWrap("Failed to retrieve subscribed sources", err)
	}
	// Repo returns empty slice if none found.

	webutil.RespondWithJSON(w, http.StatusOK, sources)
	return nil
}
