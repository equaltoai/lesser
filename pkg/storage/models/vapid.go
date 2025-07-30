package models

import (
	"time"
)

// VAPIDKeyRecord represents VAPID keys stored in DynamoDB
type VAPIDKeyRecord struct {
	PK        string      `dynamorm:"pk"`     // INSTANCE#CONFIG
	SK        string      `dynamorm:"sk"`     // VAPID_KEYS
	Data      interface{} `json:"data"`       // The actual VAPID keys data (storage.VAPIDKeys)
	UpdatedAt time.Time   `json:"updated_at"`
}

// UpdateKeys updates any GSI keys - VAPID doesn't use GSIs so this is no-op
func (v *VAPIDKeyRecord) UpdateKeys() {
	// VAPID keys don't use GSI keys, so no updates needed
}