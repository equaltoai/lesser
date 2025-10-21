package payment

import (
	"time"
)

// Payment represents a payment transaction with idempotency support
type Payment struct {
	ID             string            `dynamorm:"pk" json:"id"`
	IdempotencyKey string            `dynamorm:"index:gsi-idempotency" json:"idempotency_key"`
	MerchantID     string            `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	Amount         int64             `json:"amount"` // Always in cents
	Currency       string            `json:"currency"`
	Status         string            `dynamorm:"index:gsi-merchant,sk,prefix:status" json:"status"`
	PaymentMethod  string            `json:"payment_method"`
	CustomerID     string            `dynamorm:"index:gsi-customer" json:"customer_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	Metadata       map[string]string `dynamorm:"json" json:"metadata,omitempty"`
	CreatedAt      time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt      time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Version        int               `dynamorm:"version" json:"version"`
}

// PaymentStatus constants
const (
	PaymentStatusPending    = "pending"
	PaymentStatusProcessing = "processing"
	PaymentStatusSucceeded  = "succeeded"
	PaymentStatusFailed     = "failed"
	PaymentStatusCanceled   = "canceled"
)

// Transaction represents a transaction on a payment (capture, refund, void)
type Transaction struct {
	ID           string       `dynamorm:"pk" json:"id"`
	PaymentID    string       `dynamorm:"index:gsi-payment" json:"payment_id"`
	Type         string       `json:"type"` // capture, refund, void
	Amount       int64        `json:"amount"`
	Status       string       `json:"status"`
	ProcessedAt  time.Time    `json:"processed_at"`
	ProcessorID  string       `json:"processor_id,omitempty"`
	ResponseCode string       `json:"response_code,omitempty"`
	ResponseText string       `json:"response_text,omitempty"`
	AuditTrail   []AuditEntry `dynamorm:"json" json:"audit_trail"`
	CreatedAt    time.Time    `dynamorm:"created_at" json:"created_at"`
	UpdatedAt    time.Time    `dynamorm:"updated_at" json:"updated_at"`
	Version      int          `dynamorm:"version" json:"version"`
}

// TransactionType constants
const (
	TransactionTypeCapture = "capture"
	TransactionTypeRefund  = "refund"
	TransactionTypeVoid    = "void"
)

// Customer represents a customer with PCI-compliant payment methods
type Customer struct {
	ID             string            `dynamorm:"pk" json:"id"`
	MerchantID     string            `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	Email          string            `dynamorm:"index:gsi-email,pk,encrypted" json:"email"`
	Name           string            `dynamorm:"encrypted" json:"name"`
	Phone          string            `dynamorm:"encrypted" json:"phone,omitempty"`
	PaymentMethods []PaymentMethod   `dynamorm:"json,encrypted:pci" json:"payment_methods"`
	DefaultMethod  string            `json:"default_method,omitempty"`
	Metadata       map[string]string `dynamorm:"json" json:"metadata,omitempty"`
	CreatedAt      time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt      time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Version        int               `dynamorm:"version" json:"version"`
}

// PaymentMethod represents a customer's payment method
type PaymentMethod struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // card, bank_account
	Last4       string    `json:"last4"`
	Brand       string    `json:"brand,omitempty"` // For cards
	ExpiryMonth int       `json:"expiry_month,omitempty"`
	ExpiryYear  int       `json:"expiry_year,omitempty"`
	BankName    string    `json:"bank_name,omitempty"` // For bank accounts
	AccountType string    `json:"account_type,omitempty"`
	Token       string    `json:"-"` // Never expose in JSON
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

// Merchant represents a merchant account
type Merchant struct {
	ID              string         `dynamorm:"pk" json:"id"`
	Name            string         `json:"name"`
	Email           string         `dynamorm:"index:gsi-email,pk" json:"email"`
	Status          string         `json:"status"`
	ProcessorConfig map[string]any `dynamorm:"json,encrypted" json:"-"`
	WebhookURL      string         `json:"webhook_url,omitempty"`
	WebhookSecret   string         `dynamorm:"encrypted" json:"-"`
	Features        []string       `dynamorm:"set" json:"features"`
	RateLimits      RateLimits     `dynamorm:"json" json:"rate_limits"`
	CreatedAt       time.Time      `dynamorm:"created_at" json:"created_at"`
	UpdatedAt       time.Time      `dynamorm:"updated_at" json:"updated_at"`
	Version         int            `dynamorm:"version" json:"version"`
}

// RateLimits defines rate limiting configuration
type RateLimits struct {
	PaymentsPerMinute int   `json:"payments_per_minute"`
	PaymentsPerDay    int   `json:"payments_per_day"`
	MaxPaymentAmount  int64 `json:"max_payment_amount"`
}

// AuditEntry represents an entry in the audit trail
type AuditEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Action    string         `json:"action"`
	UserID    string         `json:"user_id,omitempty"`
	IPAddress string         `json:"ip_address,omitempty"`
	Changes   map[string]any `json:"changes,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

// IdempotencyRecord tracks idempotent requests
type IdempotencyRecord struct {
	Key         string    `dynamorm:"pk" json:"key"`
	MerchantID  string    `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	RequestHash string    `json:"request_hash"`
	Response    string    `dynamorm:"json" json:"response"`
	StatusCode  int       `json:"status_code"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	ExpiresAt   time.Time `dynamorm:"ttl" json:"expires_at"`
}

// Settlement represents a batch settlement
type Settlement struct {
	ID               string             `dynamorm:"pk" json:"id"`
	MerchantID       string             `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	Date             string             `dynamorm:"index:gsi-merchant,sk" json:"date"` // YYYY-MM-DD
	TotalAmount      int64              `json:"total_amount"`
	TransactionCount int                `json:"transaction_count"`
	Status           string             `json:"status"`
	BatchID          string             `json:"batch_id"`
	ProcessedAt      time.Time          `json:"processed_at,omitempty"`
	Transactions     []SettlementDetail `dynamorm:"json" json:"transactions"`
	CreatedAt        time.Time          `dynamorm:"created_at" json:"created_at"`
	UpdatedAt        time.Time          `dynamorm:"updated_at" json:"updated_at"`
}

// SettlementDetail represents a transaction in a settlement
type SettlementDetail struct {
	PaymentID     string `json:"payment_id"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount"`
	Fee           int64  `json:"fee"`
	NetAmount     int64  `json:"net_amount"`
}

// Webhook represents a webhook delivery attempt
type Webhook struct {
	ID           string         `dynamorm:"pk" json:"id"`
	MerchantID   string         `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	EventType    string         `dynamorm:"index:gsi-merchant,sk,prefix:event" json:"event_type"`
	PaymentID    string         `json:"payment_id,omitempty"`
	URL          string         `json:"url"`
	Payload      map[string]any `dynamorm:"json" json:"payload"`
	Attempts     int            `json:"attempts"`
	LastAttempt  time.Time      `json:"last_attempt,omitempty"`
	NextRetry    time.Time      `dynamorm:"index:gsi-retry" json:"next_retry,omitempty"`
	Status       string         `json:"status"`
	ResponseCode int            `json:"response_code,omitempty"`
	ResponseBody string         `json:"response_body,omitempty"`
	CreatedAt    time.Time      `dynamorm:"created_at" json:"created_at"`
	ExpiresAt    time.Time      `dynamorm:"ttl" json:"expires_at"`
}

// WebhookStatus constants
const (
	WebhookStatusPending   = "pending"
	WebhookStatusDelivered = "delivered"
	WebhookStatusFailed    = "failed"
	WebhookStatusExpired   = "expired"
)
