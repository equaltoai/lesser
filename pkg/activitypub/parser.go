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
		return nil, fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	}

	var activity Activity
	if err := json.Unmarshal(data, &activity); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseActivity, err)
	}

	// Validate the parsed activity
	if err := ValidateActivity(&activity); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidActivity, err)
	}

	return &activity, nil
}

// ParseActor parses a JSON byte array into an Actor struct
func ParseActor(data []byte) (*Actor, error) {
	// Validate JSON before parsing
	if err := common.ValidateJSONField(string(data), "activitypub_actor"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	}

	var actor Actor
	if err := json.Unmarshal(data, &actor); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseActor, err)
	}

	// Validate the parsed actor
	if err := ValidateActor(&actor); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidActor, err)
	}

	return &actor, nil
}

// ParseNote parses a JSON byte array into a Note struct
func ParseNote(data []byte) (*Note, error) {
	// Validate JSON before parsing
	if err := common.ValidateJSONField(string(data), "activitypub_note"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidJSON, err)
	}

	var note Note
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseNote, err)
	}

	// Validate the parsed note
	if err := ValidateNote(&note); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidNote, err)
	}

	return &note, nil
}
