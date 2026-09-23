package errors

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// CliError is a user-facing CLI error with an optional suggestion.
type CliError struct {
	UserMessage string
	Suggestion  string
	Cause       error
}

func (e *CliError) Error() string {
	return e.UserMessage
}

func (e *CliError) Unwrap() error {
	return e.Cause
}

// NewCliError creates a CliError.
func NewCliError(userMessage string, opts ...Option) *CliError {
	e := &CliError{UserMessage: userMessage}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Option configures a CliError.
type Option func(*CliError)

// WithSuggestion sets a fix suggestion.
func WithSuggestion(s string) Option {
	return func(e *CliError) { e.Suggestion = s }
}

// WithCause sets the underlying cause.
func WithCause(err error) Option {
	return func(e *CliError) { e.Cause = err }
}

// NotFoundError is returned when an entity lookup fails.
type NotFoundError struct {
	CliError
	EntityType string
	Identifier string
}

// NewNotFoundError formats "<type> not found: <id>".
func NewNotFoundError(entityType, identifier string, opts ...Option) *NotFoundError {
	e := &NotFoundError{
		CliError: CliError{
			UserMessage: fmt.Sprintf("%s not found: %s", entityType, identifier),
		},
		EntityType: entityType,
		Identifier: identifier,
	}
	for _, opt := range opts {
		opt(&e.CliError)
	}
	return e
}

// ValidationError is returned for invalid user input.
type ValidationError struct {
	CliError
}

// NewValidationError creates a ValidationError.
func NewValidationError(message string, opts ...Option) *ValidationError {
	e := &ValidationError{CliError: CliError{UserMessage: message}}
	for _, opt := range opts {
		opt(&e.CliError)
	}
	return e
}

// AuthError is returned for authentication/authorization failures.
type AuthError struct {
	CliError
}

// NewAuthError creates an AuthError with a default login suggestion.
func NewAuthError(message string, opts ...Option) *AuthError {
	e := &AuthError{CliError: CliError{
		UserMessage: message,
		Suggestion:  "Run `linear auth login` to authenticate.",
	}}
	for _, opt := range opts {
		opt(&e.CliError)
	}
	if e.Suggestion == "" {
		e.Suggestion = "Run `linear auth login` to authenticate."
	}
	return e
}

// IsDebugMode reports whether LINEAR_DEBUG is "1" or "true".
func IsDebugMode() bool {
	v := os.Getenv("LINEAR_DEBUG")
	return v == "1" || v == "true"
}

// HandleError writes a user-facing error to stderr (✗ prefix). It does not exit.
func HandleError(err error, context string) {
	HandleErrorTo(os.Stderr, err, context)
}

// HandleErrorTo writes a user-facing error to w.
func HandleErrorTo(w io.Writer, err error, context string) {
	prefix := ""
	if context != "" {
		prefix = context + ": "
	}

	var cliErr *CliError
	var notFound *NotFoundError
	var validation *ValidationError
	var auth *AuthError

	switch {
	case errors.As(err, &notFound):
		cliErr = &notFound.CliError
	case errors.As(err, &validation):
		cliErr = &validation.CliError
	case errors.As(err, &auth):
		cliErr = &auth.CliError
	case errors.As(err, &cliErr):
		// cliErr set
	default:
		msg := fmt.Sprint(err)
		if e, ok := err.(interface{ Error() string }); ok {
			msg = e.Error()
		}
		fmt.Fprintf(w, "✗ %s%s\n", prefix, msg)
		if IsDebugMode() {
			fmt.Fprintf(w, "\nDebug info:\n%+v\n", err)
		}
		return
	}

	fmt.Fprintf(w, "✗ %s%s\n", prefix, cliErr.UserMessage)
	if cliErr.Suggestion != "" {
		fmt.Fprintf(w, "  %s\n", cliErr.Suggestion)
	}
	if IsDebugMode() && cliErr.Cause != nil {
		fmt.Fprintf(w, "\nStack trace (LINEAR_DEBUG=1):\n%+v\n", cliErr.Cause)
	}
}
