package models

import (
	"fmt"
	"time"
)

// SearchEmbedding represents vector embeddings for semantic search
type SearchEmbedding struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Fields
	ContentID   string            `json:"content_id"`   // status/actor ID
	ContentType string            `json:"content_type"` // actor, status
	Embedding   []float32         `json:"embedding"`    // vector representation
	Score       float64           `json:"score"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (s *SearchEmbedding) UpdateKeys() {
	s.PK = fmt.Sprintf("EMBEDDING#%s", s.ContentID)
	s.SK = "VECTOR"
}
