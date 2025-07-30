package models

import (
	"fmt"
	"time"
)

// Device represents a user's device/session stored in DynamoDB
type Device struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // USER#username
	SK string `dynamorm:"sk" json:"-"` // DEVICE#deviceID

	// GSI for querying devices by last seen
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"-"` // USER#username#DEVICES
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"-"` // {lastSeenAt}#{deviceID}

	// GSI for trust level monitoring
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"-"` // TRUST_LEVEL#{trustLevel}
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"-"` // {lastSeenAt}#{deviceID}

	// Device data
	DeviceID      string    `json:"device_id"`
	Username      string    `json:"username"`
	DeviceName    string    `json:"device_name"`
	DeviceType    string    `json:"device_type"` // web, mobile, desktop
	LastIPAddress string    `json:"last_ip_address"`
	LastUserAgent string    `json:"last_user_agent"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	TrustLevel    string    `json:"trust_level"` // trusted, untrusted, suspicious
	Platform      string    `json:"platform,omitempty"` // iOS, Android, Windows, etc.
	AppVersion    string    `json:"app_version,omitempty"`
	Location      string    `json:"location,omitempty"` // Approximate location
	Active        bool      `json:"active"`

	// TTL for auto-cleanup of inactive devices (90 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the device data
func (d *Device) UpdateKeys() {
	// Primary key
	d.PK = fmt.Sprintf("USER#%s", d.Username)
	d.SK = fmt.Sprintf("DEVICE#%s", d.DeviceID)

	// GSI1 for querying user's devices by last activity
	d.GSI1PK = fmt.Sprintf("USER#%s#DEVICES", d.Username)
	d.GSI1SK = fmt.Sprintf("%d#%s", d.LastSeenAt.Unix(), d.DeviceID)

	// GSI2 for monitoring devices by trust level
	d.GSI2PK = fmt.Sprintf("TRUST_LEVEL#%s", d.TrustLevel)
	d.GSI2SK = fmt.Sprintf("%d#%s", d.LastSeenAt.Unix(), d.DeviceID)

	// Set TTL for inactive devices (90 days after last seen)
	d.TTL = d.LastSeenAt.Add(90 * 24 * time.Hour).Unix()
}

// UpdateLastSeen updates the last seen timestamp and related fields
func (d *Device) UpdateLastSeen(ipAddress, userAgent string) {
	d.LastSeenAt = time.Now()
	d.LastIPAddress = ipAddress
	d.LastUserAgent = userAgent
	d.UpdateKeys()
}

// SetTrustLevel updates the device trust level
func (d *Device) SetTrustLevel(level string) {
	validLevels := map[string]bool{
		"trusted":    true,
		"untrusted":  true,
		"suspicious": true,
	}
	if validLevels[level] {
		d.TrustLevel = level
		d.UpdateKeys()
	}
}

// IsActive checks if the device is considered active (seen in last 30 days)
func (d *Device) IsActive() bool {
	return d.Active && time.Since(d.LastSeenAt) < 30*24*time.Hour
}