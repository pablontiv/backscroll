package diagnostics

import "fmt"

// Error represents a structured error with a code and message.
type Error struct {
	Code    string
	Message string
	Cause   error
}

// New creates a new Error with the given code and message.
func New(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   nil,
	}
}

// Wrap creates a new Error with the given code and message, wrapping a cause error.
func Wrap(code, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error, implementing errors.Unwrap.
func (e *Error) Unwrap() error {
	return e.Cause
}
