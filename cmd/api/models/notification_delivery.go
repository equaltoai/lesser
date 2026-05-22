package models

// NotificationDeliveryRequest is the strict JSON contract for lesser-host to deliver
// communication notifications into a lesser instance (v3 spec §6.3).
//
// It is intentionally narrow (only the fields in the spec) so the instance side can
// validate and persist notifications without guessing at provider-specific payloads.
type NotificationDeliveryRequest struct {
	Type         string                           `json:"type"`
	Channel      string                           `json:"channel"`
	From         NotificationDeliveryFrom         `json:"from"`
	To           *NotificationDeliveryTo          `json:"to,omitempty"`
	Subject      string                           `json:"subject"`
	Body         string                           `json:"body"`
	BodyMimeType string                           `json:"bodyMimeType,omitempty"`
	ReceivedAt   string                           `json:"receivedAt"`
	MessageID    string                           `json:"messageId"`
	InReplyTo    *string                          `json:"inReplyTo,omitempty"`
	Attachments  []NotificationDeliveryAttachment `json:"attachments,omitempty"`
}

// NotificationDeliveryFrom describes the sender for an inbound communication notification.
type NotificationDeliveryFrom struct {
	Address     string  `json:"address"`
	Number      string  `json:"number,omitempty"`
	SoulAgentID *string `json:"soulAgentId"`
	DisplayName string  `json:"displayName"`
}

// NotificationDeliveryTo describes the recipient for an inbound communication notification.
type NotificationDeliveryTo struct {
	Address string `json:"address"`
	Number  string `json:"number,omitempty"`
	// SoulAgentID is the authoritative recipient soul identity. lesser-host should
	// send it for instance-scoped lessersoul.ai addresses so Lesser does not derive
	// the local body by parsing the public email local-part.
	SoulAgentID *string `json:"soulAgentId,omitempty"`
}

// NotificationDeliveryAttachment carries metadata-only attachment information for inbound communication notifications.
type NotificationDeliveryAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
}
