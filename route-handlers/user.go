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

type UserHandler struct {
	Repo *datastore.UserRepository
}

func NewUserHandler(repo *datastore.UserRepository) *UserHandler {
	return &UserHandler{Repo: repo}
}

func (h *UserHandler) HandleGetUsers(w http.ResponseWriter, r *http.Request) error {
	users, err := h.Repo.GetUsers(r.Context())
	if err != nil {
		return fmt.Errorf("failed to retrieve users: %w", err)
	}
	if users == nil {
		users = []models.User{}
	}
	webutil.RespondWithJSON(w, http.StatusOK, users)
	return nil
}

func (h *UserHandler) HandleCreateUser(w http.ResponseWriter, r *http.Request) error {
	var requestData struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&requestData); err != nil {
		return webutil.ErrBadRequest("Invalid request payload: " + err.Error())
	}
	defer r.Body.Close()

	if requestData.Email == "" {
		return webutil.ErrBadRequest("Email is required")
	}
	// TODO: Add more robust email validation here if needed.

	newUser := models.User{
		ID:        uuid.NewString(),
		CreatedAt: time.Now().UTC(),
		Email:     requestData.Email,
	}

	emailToken, err := webutil.GenerateRandomToken(16)
	if err != nil {
		// This is an internal server error.
		return fmt.Errorf("failed to generate email token: %w", err)
	}

	err = h.Repo.CreateUser(r.Context(), &newUser, emailToken)
	if err != nil {
		// TODO: Could be refined to return a 409 Conflict if we detect unique constraint violation.
		return fmt.Errorf("failed to create user %s: %w", newUser.Email, err)
	}

	webutil.RespondWithJSON(w, http.StatusCreated, newUser)
	return nil
}

func (h *UserHandler) HandleGetUser(w http.ResponseWriter, r *http.Request) error {
	userID := chi.URLParam(r, "id") // "id" is the common constant name in routes.go
	if _, err := uuid.Parse(userID); err != nil {
		return webutil.ErrBadRequest("Invalid user ID format")
	}

	user, err := h.Repo.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "user not found") {
			return webutil.ErrNotFound("User not found")
		}
		return fmt.Errorf("failed to retrieve user %s: %w", userID, err)
	}

	webutil.RespondWithJSON(w, http.StatusOK, user)
	return nil
}