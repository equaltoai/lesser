package models

import (
	"fmt"
	"time"
)

// Publication represents a blog/newsletter publication with multiple contributors
type Publication struct {
	PK string `dynamorm:"pk,attr:PK"` // PUBLICATION#{id}
	SK string `dynamorm:"sk,attr:SK"` // METADATA

	ID          string `dynamorm:"attr:id" json:"id"`
	Name        string `dynamorm:"attr:name" json:"name"`
	Tagline     string `dynamorm:"attr:tagline" json:"tagline,omitempty"`
	Description string `dynamorm:"attr:description" json:"description,omitempty"`
	Slug        string `dynamorm:"attr:slug" json:"slug"`

	// Branding
	LogoURL   string `dynamorm:"attr:logoURL" json:"logo_url,omitempty"`
	BannerURL string `dynamorm:"attr:bannerURL" json:"banner_url,omitempty"`
	Theme     string `dynamorm:"attr:theme" json:"theme,omitempty"` // JSON theme config

	// Configuration
	CustomDomain string `dynamorm:"attr:customDomain" json:"custom_domain,omitempty"`

	// ActivityPub
	ActorID string `dynamorm:"attr:actorID" json:"actor_id"` // The AP Actor for this publication

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing Publication.
func (Publication) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the Publication model
func (p *Publication) UpdateKeys() error {
	if p.ID == "" {
		return fmt.Errorf("ID is required")
	}

	p.PK = fmt.Sprintf("PUBLICATION#%s", p.ID)
	p.SK = "METADATA"

	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	return nil
}

// GetPK returns the partition key
func (p *Publication) GetPK() string {
	return p.PK
}

// GetSK returns the sort key
func (p *Publication) GetSK() string {
	return p.SK
}

// PublicationMember represents a contributor to a publication
type PublicationMember struct {
	PK string `dynamorm:"pk,attr:PK"` // PUBLICATION#{pub_id}#MEMBER
	SK string `dynamorm:"sk,attr:SK"` // USER#{user_id}

	PublicationID string `dynamorm:"attr:publicationID" json:"publication_id"`
	UserID        string `dynamorm:"attr:userID" json:"user_id"`
	Role          string `dynamorm:"attr:role" json:"role"` // owner, editor, writer, contributor

	// Display
	DisplayName string `dynamorm:"attr:displayName" json:"display_name,omitempty"` // Override user's name
	Bio         string `dynamorm:"attr:bio" json:"bio,omitempty"`                  // Publication-specific bio

	// Timestamps
	JoinedAt  time.Time `dynamorm:"attr:joinedAt" json:"joined_at"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing PublicationMember.
func (PublicationMember) TableName() string {
	return MainTableName
}

// UpdateKeys updates the keys for the PublicationMember model
func (pm *PublicationMember) UpdateKeys() error {
	if pm.PublicationID == "" {
		return fmt.Errorf("PublicationID is required")
	}
	if pm.UserID == "" {
		return fmt.Errorf("UserID is required")
	}

	pm.PK = fmt.Sprintf("PUBLICATION#%s#MEMBER", pm.PublicationID)
	pm.SK = fmt.Sprintf("USER#%s", pm.UserID)

	now := time.Now()
	if pm.CreatedAt.IsZero() {
		pm.CreatedAt = now
	}
	pm.UpdatedAt = now
	if pm.JoinedAt.IsZero() {
		pm.JoinedAt = now
	}

	return nil
}

// GetPK returns the partition key
func (pm *PublicationMember) GetPK() string {
	return pm.PK
}

// GetSK returns the sort key
func (pm *PublicationMember) GetSK() string {
	return pm.SK
}
