package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrorLeafMessages returns deduplicated leaf error messages from wrapped and joined errors.
func ErrorLeafMessages(err error) []string {
	if err == nil {
		return nil
	}

	visited := make(map[string]struct{})
	seenMessages := make(map[string]struct{})
	leaves := make([]string, 0, 4)

	var visit func(error)
	visit = func(current error) {
		if current == nil {
			return
		}

		key := errorVisitKey(current)
		if _, seen := visited[key]; seen {
			return
		}
		visited[key] = struct{}{}

		if joined, ok := current.(interface{ Unwrap() []error }); ok {
			children := joined.Unwrap()
			hasChild := false
			for _, child := range children {
				if child == nil {
					continue
				}
				hasChild = true
				visit(child)
			}
			if hasChild {
				return
			}
		}

		if unwrapped := errors.Unwrap(current); unwrapped != nil {
			visit(unwrapped)
			return
		}

		message := strings.TrimSpace(current.Error())
		if message == "" {
			return
		}
		if _, seen := seenMessages[message]; seen {
			return
		}
		seenMessages[message] = struct{}{}
		leaves = append(leaves, message)
	}

	visit(err)

	return leaves
}

// ErrorLeafSummary joins leaf error messages into a human-readable root-cause string.
func ErrorLeafSummary(err error) string {
	return strings.Join(ErrorLeafMessages(err), "; ")
}

// WrapErrorWithLeafCauses keeps normal wrapping semantics while appending explicit root causes.
func WrapErrorWithLeafCauses(prefix string, err error) error {
	if err == nil {
		return nil
	}

	causes := ErrorLeafSummary(err)
	if causes == "" {
		return fmt.Errorf("%s: %w", prefix, err)
	}

	return fmt.Errorf("%s: %w (root causes: %s)", prefix, err, causes)
}

func errorVisitKey(err error) string {
	if err == nil {
		return "<nil>"
	}

	value := reflect.ValueOf(err)
	switch value.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		if !value.IsNil() {
			return fmt.Sprintf("%T@0x%x", err, value.Pointer())
		}
	}

	return fmt.Sprintf("%T:%s", err, err.Error())
}
