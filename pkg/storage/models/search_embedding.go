package models

import (
	"fmt"
	"time"
)

// SearchEmbedding represents vector embeddings for semantic search
type SearchEmbedding struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Fields
	ContentID   string            `dynamorm:"attr:contentID" json:"content_id"`     // status/actor ID
	ContentType string            `dynamorm:"attr:contentType" json:"content_type"` // actor, status
	Embedding   []float32         `dynamorm:"attr:embedding" json:"embedding"`      // vector representation
	Score       float64           `dynamorm:"attr:score" json:"score"`
	Metadata    map[string]string `dynamorm:"attr:metadata" json:"metadata,omitempty"`
	CreatedAt   time.Time         `dynamorm:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table backing SearchEmbedding.
func (SearchEmbedding) TableName() string {
	return MainTableName
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (s *SearchEmbedding) UpdateKeys() error {
	s.PK = fmt.Sprintf("EMBEDDING#%s", s.ContentID)
	s.SK = "VECTOR"
	return nil
}

// GetPK returns the partition key
func (s *SearchEmbedding) GetPK() string {
	return s.PK
}

// GetSK returns the sort key
func (s *SearchEmbedding) GetSK() string {
	return s.SK
}
