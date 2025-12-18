package models

import (
	"fmt"
	"time"
)

// Series represents a multi-part content series
type Series struct {
	PK string `dynamorm:"pk,attr:PK"` // AUTHOR#{author_id}#SERIES
	SK string `dynamorm:"sk,attr:SK"` // ID#{series_id}

	ID          string `dynamorm:"attr:id" json:"id"`
	AuthorID    string `dynamorm:"attr:authorID" json:"author_id"`
	Title       string `dynamorm:"attr:title" json:"title"`
	Description string `dynamorm:"attr:description" json:"description,omitempty"`
	Slug        string `dynamorm:"attr:slug" json:"slug"`
	CoverImage  string `dynamorm:"attr:coverImage" json:"cover_image,omitempty"`

	// Status
	IsComplete   bool `dynamorm:"attr:isComplete" json:"is_complete"`
	ArticleCount int  `dynamorm:"attr:articleCount" json:"article_count"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing Series.
func (Series) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the Series model
func (s *Series) UpdateKeys() error {
	if s.AuthorID == "" {
		return fmt.Errorf("AuthorID is required")
	}
	if s.ID == "" {
		return fmt.Errorf("ID is required")
	}

	s.PK = fmt.Sprintf("AUTHOR#%s#SERIES", s.AuthorID)
	s.SK = fmt.Sprintf("ID#%s", s.ID)

	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	return nil
}

// GetPK returns the partition key
func (s *Series) GetPK() string {
	return s.PK
}

// GetSK returns the sort key
func (s *Series) GetSK() string {
	return s.SK
}
