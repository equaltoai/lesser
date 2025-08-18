// Package activitypub provides parsing and validation utilities for ActivityPub protocol messages.
package activitypub

import (
	"encoding/json"
	"fmt"

	"github.com/equaltoai/lesser/pkg/common"
)

// ParseActivity parses a JSON byte array into an Activity struct
func ParseActivity(data []byte) (*Activity, error) {
	// Validate JSON before parsing
	if err := common.ValidateJSONField(string(data), "activitypub_activity"); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var activity Activity
	if err := json.Unmarshal(data, &activity); err != nil {
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
	// Validate JSON before parsing
	if err := common.ValidateJSONField(string(data), "activitypub_actor"); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var actor Actor
	if err := json.Unmarshal(data, &actor); err != nil {
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
	// Validate JSON before parsing
	if err := common.ValidateJSONField(string(data), "activitypub_note"); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var note Note
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("failed to parse note: %w", err)
	}

	// Validate the parsed note
	if err := ValidateNote(&note); err != nil {
		return nil, fmt.Errorf("invalid note: %w", err)
	}

	return &note, nil
}
