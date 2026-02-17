package webutil

import (
	"errors"
	"fmt"
	"net/http"
)

const (
	msgBadRequest          = "Bad Request"
	msgNotFound            = "Resource not found"
	msgInternalServer      = "Internal Server Error"
	msgUnauthorized        = "Unauthorized"
	msgForbidden           = "Forbidden"
	msgConflict            = "Conflict"
	msgUnprocessableEntity = "Unprocessable Entity"
)

// Represents an error with an associated HTTP status code
// and a user-facing message.
type HTTPError struct {
	cause   error  // The underlying error, can be nil
	Code    int    // HTTP status code
	Message string // User-facing error message
}

// Implements the error interface.
// It returns the Message, which is intended for the HTTP response.
func (he HTTPError) Error() string {
	return he.Message
}

// Provides compatibility for errors.Is and errors.As.
func (he HTTPError) Unwrap() error {
	return he.cause
}

// Returns the defaultVal if the initial message is empty.
func defaultMessageIfEmpty(initialMsg, defaultVal string) string {
	if initialMsg == "" {
		return defaultVal
	}
	return initialMsg
}

// Creates a new HTTPError with a code and message.
// The message provided will be used directly. If a default message is desired
// for an empty input message, use the specific ErrXxx constructors.
func NewHTTPError(code int, message string) *HTTPError {
	return &HTTPError{
		cause:   errors.New(message), // Base error is the message itself
		Code:    code,
		Message: message,
	}
}

// Creates a new HTTPError that wraps an existing error (cause).
// The message is a user-facing message for this specific HTTP error context.
func NewHTTPErrorWrap(code int, message string, cause error) *HTTPError {
	return &HTTPError{
		cause:   cause,
		Code:    code,
		Message: message,
	}
}

func ErrBadRequest(message string) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, defaultMessageIfEmpty(message, msgBadRequest))
}

func ErrBadRequestWrap(message string, cause error) *HTTPError {
	return NewHTTPErrorWrap(http.StatusBadRequest, defaultMessageIfEmpty(message, msgBadRequest), cause)
}

func ErrNotFound(message string) *HTTPError {
	return NewHTTPError(http.StatusNotFound, defaultMessageIfEmpty(message, msgNotFound))
}

func ErrNotFoundWrap(message string, cause error) *HTTPError {
	return NewHTTPErrorWrap(http.StatusNotFound, defaultMessageIfEmpty(message, msgNotFound), cause)
}

func ErrInternalServer(message string) *HTTPError {
	return NewHTTPError(http.StatusInternalServerError, defaultMessageIfEmpty(message, msgInternalServer))
}

func ErrInternalServerWrap(message string, cause error) *HTTPError {
	return NewHTTPErrorWrap(http.StatusInternalServerError, msgInternalServer, fmt.Errorf("%s: %w", message, cause))
}

func ErrUnauthorized(message string) *HTTPError {
	return NewHTTPError(http.StatusUnauthorized, defaultMessageIfEmpty(message, msgUnauthorized))
}

func ErrForbidden(message string) *HTTPError {
	return NewHTTPError(http.StatusForbidden, defaultMessageIfEmpty(message, msgForbidden))
}

func ErrConflict(message string) *HTTPError {
	return NewHTTPError(http.StatusConflict, defaultMessageIfEmpty(message, msgConflict))
}

func ErrUnprocessableEntity(message string) *HTTPError {
	return NewHTTPError(http.StatusUnprocessableEntity, defaultMessageIfEmpty(message, msgUnprocessableEntity))
}
