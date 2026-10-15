package main

import (
	"fmt"
	"math"
)

func validateUnitInterval(flag string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("--%s must be finite and between 0 and 1", flag)
	}
	return nil
}

func validateSearchNumbers(maxTokens int) error {
	if maxTokens < 0 {
		return fmt.Errorf("--max-tokens must be >= 0")
	}
	return nil
}

func validateListNumbers(recent, limit, offset int) error {
	for _, field := range []struct {
		name  string
		value int
	}{{"recent", recent}, {"limit", limit}, {"offset", offset}} {
		if field.value < 0 {
			return fmt.Errorf("--%s must be >= 0", field.name)
		}
	}
	return nil
}

func validatePatternsNumbers(support int, confidence float64, batch, minLength, maxLength int) error {
	for _, field := range []struct {
		name  string
		value int
	}{{"min-support", support}, {"batch", batch}, {"min-length", minLength}, {"max-length", maxLength}} {
		if field.value < 0 {
			return fmt.Errorf("--%s must be >= 0", field.name)
		}
	}
	if err := validateUnitInterval("min-confidence", confidence); err != nil {
		return err
	}
	// Zero lengths retain the sequence miner's historical default semantics.
	if minLength == 0 {
		minLength = 2
	}
	if maxLength == 0 {
		maxLength = 6
	}
	if minLength > maxLength {
		return fmt.Errorf("--min-length must not exceed --max-length (after applying zero defaults)")
	}
	return nil
}
