package diagnostics

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	err := New("CODE_001", "test message")
	if err.Code != "CODE_001" {
		t.Errorf("expected code CODE_001, got %s", err.Code)
	}
	if err.Message != "test message" {
		t.Errorf("expected message 'test message', got %s", err.Message)
	}
	if err.Cause != nil {
		t.Errorf("expected Cause to be nil, got %v", err.Cause)
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("original error")
	err := Wrap("CODE_002", "wrapped message", cause)
	if err.Code != "CODE_002" {
		t.Errorf("expected code CODE_002, got %s", err.Code)
	}
	if err.Message != "wrapped message" {
		t.Errorf("expected message 'wrapped message', got %s", err.Message)
	}
	if err.Cause != cause {
		t.Errorf("expected Cause to be %v, got %v", cause, err.Cause)
	}
}

func TestErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "without cause",
			err:      New("CODE_001", "test message"),
			expected: "CODE_001: test message",
		},
		{
			name:     "with cause",
			err:      Wrap("CODE_002", "wrapped", errors.New("cause")),
			expected: "CODE_002: wrapped (cause)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestUnwrap(t *testing.T) {
	cause := errors.New("original error")
	err := Wrap("CODE_002", "wrapped message", cause)

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("expected unwrap to return %v, got %v", cause, unwrapped)
	}

	// Test that errors.Is works with the wrapped error
	if !errors.Is(err, cause) {
		t.Error("expected errors.Is to return true")
	}
}

func TestUnwrapWithoutCause(t *testing.T) {
	err := New("CODE_001", "test message")
	unwrapped := err.Unwrap()
	if unwrapped != nil {
		t.Errorf("expected unwrap to return nil, got %v", unwrapped)
	}
}
