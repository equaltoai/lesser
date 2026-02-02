package models

import (
	"fmt"
	"time"
)

// SearchEmbedding represents vector embeddings for semantic search
type SearchEmbedding struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Fields
	ContentID   string            `theorydb:"attr:contentID" json:"content_id"`     // status/actor ID
	ContentType string            `theorydb:"attr:contentType" json:"content_type"` // actor, status
	Embedding   []float32         `theorydb:"attr:embedding" json:"embedding"`      // vector representation
	Score       float64           `theorydb:"attr:score" json:"score"`
	Metadata    map[string]string `theorydb:"attr:metadata" json:"metadata,omitempty"`
	CreatedAt   time.Time         `theorydb:"attr:createdAt" json:"created_at"`
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
