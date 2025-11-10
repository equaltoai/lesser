package models

import (
	"time"
)

// VAPIDKeyRecord represents VAPID keys stored in DynamoDB
type VAPIDKeyRecord struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	PK        string      `dynamorm:"pk,attr:PK"`            // INSTANCE#CONFIG
	SK        string      `dynamorm:"sk,attr:SK"`            // VAPID_KEYS
	Data      interface{} `dynamorm:"attr:data" json:"data"` // The actual VAPID keys data (storage.VAPIDKeys)
	UpdatedAt time.Time   `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys updates any GSI keys - VAPID doesn't use GSIs so this is no-op
func (v *VAPIDKeyRecord) UpdateKeys() error {
	// Set primary keys (static for VAPID keys)
	v.PK = "INSTANCE#CONFIG"
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
