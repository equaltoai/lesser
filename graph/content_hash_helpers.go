package graph

import "github.com/equaltoai/lesser/pkg/tips"

func contentHashForObjectID(objectID string) string {
	return tips.ContentHashFromObjectID(objectID)
}
