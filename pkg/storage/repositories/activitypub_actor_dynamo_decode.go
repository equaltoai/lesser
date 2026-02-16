package repositories

import (
	"encoding/json"
	"fmt"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

func decodeActivityPubActorFromDynamoValue(value any) (*activitypub.Actor, error) {
	if value == nil {
		return nil, fmt.Errorf("actor value is nil")
	}

	normalized := normalizeEmbeddedBaseObject(value)
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized actor: %w", err)
	}

	var actor activitypub.Actor
	if err := json.Unmarshal(data, &actor); err != nil {
		return nil, fmt.Errorf("unmarshal normalized actor json: %w", err)
	}
	return &actor, nil
}

func normalizeEmbeddedBaseObject(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if base, ok := typed["BaseObject"].(map[string]any); ok {
			for k, v := range base {
				if _, exists := typed[k]; !exists {
					typed[k] = v
				}
			}
			delete(typed, "BaseObject")
		}
		for k, v := range typed {
			typed[k] = normalizeEmbeddedBaseObject(v)
		}
		return typed

	case []any:
		for i := range typed {
			typed[i] = normalizeEmbeddedBaseObject(typed[i])
		}
		return typed

	default:
		return value
	}
}
