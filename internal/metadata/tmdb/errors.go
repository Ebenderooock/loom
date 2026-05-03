package tmdb

import (
	"fmt"
)

// Error types for TMDb client operations.
type ErrorCode string

const (
	ErrCodeNotFound       ErrorCode = "not_found"
	ErrCodeRateLimit      ErrorCode = "rate_limit"
	ErrCodeUnauthorized   ErrorCode = "unauthorized"
	ErrCodeClientError    ErrorCode = "client_error"
	ErrCodeServerError    ErrorCode = "server_error"
	ErrCodeNetworkError   ErrorCode = "network_error"
	ErrCodeContextError   ErrorCode = "context_error"
)

// ClientError wraps TMDb API errors with typed information.
type ClientError struct {
	Code      ErrorCode
	StatusCode int
	Message   string
	RetryAfter int // seconds (for 429)
	Err       error
}

func (e *ClientError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("tmdb: %s (HTTP %d, retry after %ds): %s", e.Code, e.StatusCode, e.RetryAfter, e.Message)
	}
	return fmt.Sprintf("tmdb: %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
}

func (e *ClientError) Unwrap() error {
	return e.Err
}

// NewNotFoundError creates a 404 error.
func NewNotFoundError(message string) *ClientError {
	return &ClientError{
		Code:       ErrCodeNotFound,
		StatusCode: 404,
		Message:    message,
	}
}

// NewUnauthorizedError creates a 401 error (bad API key).
func NewUnauthorizedError(message string) *ClientError {
	return &ClientError{
		Code:       ErrCodeUnauthorized,
		StatusCode: 401,
		Message:    message,
	}
}

// NewRateLimitError creates a 429 error with optional Retry-After.
func NewRateLimitError(message string, retryAfter int) *ClientError {
	return &ClientError{
		Code:       ErrCodeRateLimit,
		StatusCode: 429,
		Message:    message,
		RetryAfter: retryAfter,
	}
}

// NewServerError creates a 5xx error.
func NewServerError(statusCode int, message string) *ClientError {
	return &ClientError{
		Code:       ErrCodeServerError,
		StatusCode: statusCode,
		Message:    message,
	}
}

// NewClientError creates a 4xx error (not 401/404).
func NewClientError(statusCode int, message string) *ClientError {
	return &ClientError{
		Code:       ErrCodeClientError,
		StatusCode: statusCode,
		Message:    message,
	}
}

// NewNetworkError wraps a network-level error.
func NewNetworkError(err error) *ClientError {
	return &ClientError{
		Code:    ErrCodeNetworkError,
		Message: "network error",
		Err:     err,
	}
}

// NewContextError wraps a context cancellation/timeout.
func NewContextError(err error) *ClientError {
	return &ClientError{
		Code:    ErrCodeContextError,
		Message: "context error (timeout or cancelled)",
		Err:     err,
	}
}
