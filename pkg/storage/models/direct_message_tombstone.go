package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

const (
	directMessageTombstonePKPrefix = "DM_MESSAGE_TOMBSTONE"
	directMessageTombstoneSKPrefix = "STATUS"
)

// DirectMessageTombstone represents a per-viewer deletion marker for a direct message (Status).
// It enables "delete for me" semantics without deleting the underlying status globally.
//
// Primary keying is (viewerUsername, statusID).
type DirectMessageTombstone struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"PK"` // DM_MESSAGE_TOMBSTONE#viewerUsername
	SK string `theorydb:"sk,attr:SK" json:"SK"` // STATUS#statusID

	ViewerUsername string    `theorydb:"attr:viewerUsername" json:"viewer_username"`
	StatusID       string    `theorydb:"attr:statusID" json:"status_id"`
	CreatedAt      time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// TableName returns the DynamoDB table name.
func (DirectMessageTombstone) TableName() string {
	return MainTableName
}

// UpdateKeys populates keys and timestamps before persistence.
func (t *DirectMessageTombstone) UpdateKeys() error {
	if err := common.ValidateRequiredParam("viewerUsername", t.ViewerUsername); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("statusID", t.StatusID); err != nil {
		return err
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}

	t.PK = fmt.Sprintf("%s#%s", directMessageTombstonePKPrefix, t.ViewerUsername)
	t.SK = fmt.Sprintf("%s#%s", directMessageTombstoneSKPrefix, t.StatusID)
	return nil
}

// GetPK returns the record's partition key.
func (t *DirectMessageTombstone) GetPK() string { return t.PK }

// GetSK returns the record's sort key.
func (t *DirectMessageTombstone) GetSK() string { return t.SK }
