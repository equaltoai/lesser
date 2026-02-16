package models

import (
	"time"
)

// VAPIDKeyRecord represents VAPID keys stored in DynamoDB
type VAPIDKeyRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK        string         `theorydb:"pk,attr:PK"`            // INSTANCE#CONFIG
	SK        string         `theorydb:"sk,attr:SK"`            // VAPID_KEYS
	Data      map[string]any `theorydb:"attr:data" json:"data"` // The actual VAPID keys data (storage.VAPIDKeys-compatible)
	UpdatedAt time.Time      `theorydb:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys updates any GSI keys - VAPID doesn't use GSIs so this is no-op
func (v *VAPIDKeyRecord) UpdateKeys() error {
	// Set primary keys (static for VAPID keys)
	v.PK = instanceConfigPK
	v.SK = "VAPID_KEYS"

	// VAPID keys don't use GSI keys, so no updates needed
	return nil
}

// GetPK returns the partition key
func (v *VAPIDKeyRecord) GetPK() string {
	return v.PK
}

// GetSK returns the sort key
func (v *VAPIDKeyRecord) GetSK() string {
	return v.SK
}

// TableName returns the DynamoDB table backing VAPIDKeyRecord.
func (VAPIDKeyRecord) TableName() string {
	return MainTableName
}
