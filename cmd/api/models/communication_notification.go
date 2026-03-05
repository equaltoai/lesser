package models

import "time"

// CommunicationFrom represents the "from" identity on a communication notification.
type CommunicationFrom struct {
	Address     string `json:"address"`
	DisplayName string `json:"display_name,omitempty"`
	SoulAgentID string `json:"soul_agent_id,omitempty"`
}

// CommunicationTo represents the "to" identity on a communication notification.
type CommunicationTo struct {
	Address string `json:"address"`
}

// CommunicationAttachment carries metadata-only attachment information for communication notifications.
type CommunicationAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
}

// CommunicationNotification carries channel-specific comm-worker delivery data for communication:* notifications.
type CommunicationNotification struct {
	Channel     string                    `json:"channel"`
	From        CommunicationFrom         `json:"from"`
	To          *CommunicationTo          `json:"to,omitempty"`
	Attachments []CommunicationAttachment `json:"attachments,omitempty"`
	Subject     string                    `json:"subject,omitempty"`
	Body        string                    `json:"body,omitempty"`
	ReceivedAt  time.Time                 `json:"received_at"`
	MessageID   string                    `json:"message_id"`
	InReplyTo   string                    `json:"in_reply_to,omitempty"`
	ThreadID    string                    `json:"thread_id"`
}
