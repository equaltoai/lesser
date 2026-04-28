// Package activitypub provides parsing and validation utilities for ActivityPub protocol messages.
package activitypub

import (
	"encoding/json"
	"fmt"

	"github.com/equaltoai/lesser/pkg/common"
)

// ParseActivity parses a JSON byte array into an Activity struct
func ParseActivity(data []byte) (*Activity, error) {
	if err := validateRawActivityPubJSON(data, "activity"); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var activity Activity
	if err := common.ParseActivityPubObject(data, &activity); err != nil {
		return nil, fmt.Errorf("failed to parse activity: %w", err)
	}

	// Validate the parsed activity
	if err := ValidateActivity(&activity); err != nil {
		return nil, fmt.Errorf("invalid activity: %w", err)
	}

	return &activity, nil
}

// ParseActor parses a JSON byte array into an Actor struct
func ParseActor(data []byte) (*Actor, error) {
	if err := validateRawActivityPubJSON(data, "actor"); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var actor Actor
	if err := common.ParseActivityPubObject(data, &actor); err != nil {
		return nil, fmt.Errorf("failed to parse actor: %w", err)
	}

	// Validate the parsed actor
	if err := ValidateActor(&actor); err != nil {
		return nil, fmt.Errorf("invalid actor: %w", err)
	}

	return &actor, nil
}

// ParseNote parses a JSON byte array into a Note struct
func ParseNote(data []byte) (*Note, error) {
	if err := validateRawActivityPubJSON(data, "note"); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var note Note
	if err := common.ParseActivityPubObject(data, &note); err != nil {
		return nil, fmt.Errorf("failed to parse note: %w", err)
	}

	// Validate the parsed note
	if err := ValidateNote(&note); err != nil {
		return nil, fmt.Errorf("invalid note: %w", err)
	}

	return &note, nil
}

func validateRawActivityPubJSON(data []byte, kind string) error {
	if len(data) > common.MaxJSONSize {
		return fmt.Errorf("%s JSON size %d bytes exceeds maximum %d", kind, len(data), common.MaxJSONSize)
	}
	if !json.Valid(data) {
		return fmt.Errorf("malformed %s JSON", kind)
	}
	return nil
}
