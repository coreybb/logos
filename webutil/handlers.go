package webutil

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
)

// AppHandler represents a handler function that returns an error.
type AppHandler func(w http.ResponseWriter, r *http.Request) error

// MakeHandler adapts an AppHandler to the standard http.HandlerFunc signature.
// It executes the AppHandler and handles any returned error by logging appropriately
// and sending a standardized JSON error response.
func MakeHandler(handler AppHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := handler(w, r)
		if err != nil {
			var httpErr *HTTPError // Use pointer type for errors.As with our HTTPError constructors
			var publicMessage string
			var statusCode int

			switch {
			case errors.As(err, &httpErr):
				// This is an HTTPError we explicitly created (e.g., ErrBadRequest, ErrNotFound)
				statusCode = httpErr.Code
				publicMessage = httpErr.Message
				logLevel := slog.LevelWarn // Treat client errors as warnings server-side
				if statusCode >= 500 {
					logLevel = slog.LevelError // Treat 5xx HTTPError as server error
				}
				// Log the underlying cause if present and different from the public message
				cause := errors.Unwrap(httpErr)
				if cause != nil && cause.Error() != publicMessage {
					slog.Log(r.Context(), logLevel, "Client error response",
						"code", httpErr.Code,
						"msg", httpErr.Message,
						"cause", cause,
						"path", r.URL.Path,
						"method", r.Method,
					)
				} else {
					slog.Log(r.Context(), logLevel, "Client error response",
						"code", httpErr.Code,
						"msg", httpErr.Message,
						"path", r.URL.Path,
						"method", r.Method,
					)
				}

			case errors.Is(err, sql.ErrNoRows):
				// Specific handling for sql.ErrNoRows from datastore layer -> 404 Not Found
				statusCode = http.StatusNotFound
				publicMessage = "Resource not found"
				slog.Info("Resource not found (sql.ErrNoRows)", "path", r.URL.Path, "method", r.Method, "error", err)

			default:
				// Any other error is treated as an internal server error
				statusCode = http.StatusInternalServerError
				publicMessage = "Internal Server Error"
				slog.Error("Unhandled internal error", "path", r.URL.Path, "method", r.Method, "error", err)
			}

			// Check if response headers have already been written by the handler
			// (which shouldn't happen if errors are returned correctly).
			if HasResponseWriterSentHeader(w) {
				slog.Warn("Handler returned error after writing response header",
					"path", r.URL.Path,
					"method", r.Method,
					"error", err,
				)
				// Cannot send another response, just log.
				return
			}

			// Send the standardized JSON error response
			RespondWithJSON(w, statusCode, map[string]string{"error": publicMessage})
		}
		// If err is nil, the handler is assumed to have written its own successful response.
	}
}
