package input_config

import (
	"errors"
	"fmt"
)

// EvalPredicate evaluates a single predicate against an already-extracted value.
// value is the result of selecting Predicate.Selector from a record.
func EvalPredicate(p Predicate, value any) (bool, error) {
	switch p.Op {
	case "eq":
		return equalValues(value, p.Value), nil
	case "ne":
		return !equalValues(value, p.Value), nil
	case "in":
		list, ok := toList(p.Value)
		if !ok {
			return false, fmt.Errorf("predicate op=in: value must be a list, got %T", p.Value)
		}
		for _, item := range list {
			if equalValues(value, item) {
				return true, nil
			}
		}
		return false, nil
	case "exists":
		return value != nil, nil
	case "missing":
		return value == nil, nil
	default:
		return false, fmt.Errorf("unknown predicate op: %q", p.Op)
	}
}

// EvalPredicates evaluates all predicates against the record (AND semantics).
// Selector is resolved via SelectField before calling EvalPredicate.
func EvalPredicates(predicates []Predicate, record map[string]any) (bool, error) {
	for _, p := range predicates {
		value, _ := SelectField(record, p.Selector)
		ok, err := EvalPredicate(p, value)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// ErrUnknownOp is returned when a predicate uses an unrecognized operator.
var ErrUnknownOp = errors.New("unknown predicate op")

// equalValues compares two values for equality, handling type coercion between
// numeric types (float64 from JSON/TOML vs int/string).
func equalValues(a, b any) bool {
	if a == b {
		return true
	}
	// Coerce both to string for comparison when types differ
	sa, oka := toString(a)
	sb, okb := toString(b)
	if oka && okb {
		return sa == sb
	}
	return false
}

func toString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		if t {
			return "true", true
		}
		return "false", true
	case float64:
		return fmt.Sprintf("%g", t), true
	case int:
		return fmt.Sprintf("%d", t), true
	case int64:
		return fmt.Sprintf("%d", t), true
	}
	return "", false
}

func toList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case []string:
		result := make([]any, len(t))
		for i, s := range t {
			result[i] = s
		}
		return result, true
	}
	return nil, false
}
