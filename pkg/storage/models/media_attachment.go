package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// MediaAttachment represents the association between media files and their parent entities (users, scheduled statuses, etc.)
// This model handles the relationships where media is attached to different types of entities
type MediaAttachment struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary composite key - flexible to support different entity types
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format varies by entity type
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "MEDIA#{mediaID}"

	// Entity identifiers
	EntityType string `dynamorm:"attr:entityType" json:"entity_type"` // "user", "scheduled_status", etc.
	EntityID   string `dynamorm:"attr:entityID" json:"entity_id"`     // Username, status ID, etc.
	MediaID    string `dynamorm:"attr:mediaID" json:"media_id"`       // The attached media ID

	// Media metadata (denormalized for quick access)
	MediaType   string `dynamorm:"attr:mediaType" json:"media_type"`   // "image", "video", "audio"
	ContentType string `dynamorm:"attr:contentType" json:"content_type"` // Full MIME type
	FileSize    int64  `dynamorm:"attr:fileSize" json:"file_size"`      // Size in bytes

	// Display metadata
	Description string `dynamorm:"attr:description" json:"description,omitempty"` // Alt text or description
	FocalPoint  string `dynamorm:"attr:focalPoint" json:"focal_point,omitempty"`  // Focal point for cropping (x,y format)
	Order       int    `dynamorm:"attr:order" json:"order"`                       // Display order when multiple attachments

	// Timestamps
	AttachedAt time.Time `dynamorm:"attr:attachedAt" json:"attached_at"`
	UpdatedAt  time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (MediaAttachment) TableName() string {
	return MainTableName
}

// UpdateKeys updates the primary key based on entity type and ID
func (m *MediaAttachment) UpdateKeys(entityType, entityID string) {
	m.EntityType = entityType
	m.EntityID = entityID

	switch strings.ToLower(entityType) {
	case EntityTypeUser:
		// Match legacy pattern: PK=USER#username, SK=MEDIA#mediaID
		m.PK = fmt.Sprintf(KeyPatternUser, entityID)
		m.SK = fmt.Sprintf("MEDIA#%s", m.MediaID)
	case EntityTypeScheduledStatus:
		// Match legacy pattern: PK=SCHEDULED_STATUS#statusID, SK=MEDIA#mediaID
		m.PK = fmt.Sprintf("SCHEDULED_STATUS#%s", entityID)
		m.SK = fmt.Sprintf("MEDIA#%s", m.MediaID)
	default:
		// Generic pattern for future entity types
		m.PK = fmt.Sprintf("%s#%s", strings.ToUpper(entityType), entityID)
		m.SK = fmt.Sprintf("MEDIA#%s", m.MediaID)
	}
}

// BeforeCreate sets up the model before creation
func (m *MediaAttachment) BeforeCreate() error {
	now := time.Now()
	m.AttachedAt = now
	m.UpdatedAt = now

	// Ensure keys are set up correctly
	if m.EntityType != "" && m.EntityID != "" {
		m.UpdateKeys(m.EntityType, m.EntityID)
	}

	return m.Validate()
}

// BeforeUpdate sets up the model before update
func (m *MediaAttachment) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Ensure keys remain consistent
	if m.EntityType != "" && m.EntityID != "" {
		m.UpdateKeys(m.EntityType, m.EntityID)
	}

	return m.Validate()
}

// Validate performs validation on the MediaAttachment
func (m *MediaAttachment) Validate() error {
	if err := common.ValidateRequiredParam("MediaID", strings.TrimSpace(m.MediaID)); err != nil {
		return ErrMediaAttachmentIDRequired
	}
	if err := common.ValidateRequiredParam("EntityType", strings.TrimSpace(m.EntityType)); err != nil {
		return ErrMediaAttachmentEntityTypeRequired
	}
	if err := common.ValidateRequiredParam("EntityID", strings.TrimSpace(m.EntityID)); err != nil {
		return ErrMediaAttachmentEntityIDRequired
	}
	if m.Order < 0 {
		return ErrMediaAttachmentOrderNegative
	}

	// Validate entity type
	validTypes := []string{EntityTypeUser, EntityTypeScheduledStatus, EntityTypeStatus, EntityTypeAccount}
	isValid := false
	for _, vt := range validTypes {
		if strings.ToLower(m.EntityType) == vt {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("%w: %s", ErrMediaAttachmentInvalidEntityType, m.EntityType)
	}

	// Validate focal point format if provided
	if m.FocalPoint != "" {
		parts := strings.Split(m.FocalPoint, ",")
		if len(parts) != 2 {
			return ErrMediaAttachmentInvalidFocalPoint
		}
	}

	return nil
}

// IsForUser returns true if this attachment is for a user
func (m *MediaAttachment) IsForUser() bool {
	return strings.ToLower(m.EntityType) == EntityTypeUser
}

// IsForScheduledStatus returns true if this attachment is for a scheduled status
func (m *MediaAttachment) IsForScheduledStatus() bool {
	return strings.ToLower(m.EntityType) == "scheduled_status"
}

// SetFocalPoint sets the focal point for image cropping
func (m *MediaAttachment) SetFocalPoint(x, y float64) {
	m.FocalPoint = fmt.Sprintf("%.2f,%.2f", x, y)
}

// GetFocalPoint returns the focal point coordinates
func (m *MediaAttachment) GetFocalPoint() (x, y float64, ok bool) {
	if err := common.ValidateRequiredParam("FocalPoint", m.FocalPoint); err != nil {
		return 0, 0, false
	}

	parts := strings.Split(m.FocalPoint, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}

	var err error
	if x, err = parseFloat(parts[0]); err != nil {
		return 0, 0, false
	}
	if y, err = parseFloat(parts[1]); err != nil {
		return 0, 0, false
	}

	return x, y, true
}

// parseFloat is a helper to parse float from string
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
