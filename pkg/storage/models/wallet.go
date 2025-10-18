package models

import (
	"fmt"
	"strings"
	"time"
)

// WalletChallenge represents a challenge for wallet authentication
type WalletChallenge struct {
	// DynamoDB keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl" json:"-"`

	// Business fields matching storage.WalletChallenge
	ID        string    `json:"id"`
	Username  string    `json:"username,omitempty"`
	Address   string    `json:"address"`
	ChainID   int       `json:"chain_id"`
	Nonce     string    `json:"nonce"`
	Message   string    `json:"message"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TableName returns the DynamoDB table name
func (WalletChallenge) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys and TTL before creating
func (w *WalletChallenge) BeforeCreate() error {
	if err := w.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	if w.IssuedAt.IsZero() {
		w.IssuedAt = time.Now()
	}
	return nil
}

// GetPK returns the partition key
func (w *WalletChallenge) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WalletChallenge) GetSK() string {
	return w.SK
}

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WalletChallenge) UpdateKeys() error {
	w.PK = fmt.Sprintf("WALLET_CHALLENGE#%s", w.ID)
	w.SK = "CHALLENGE"
	// Set TTL to expiration time
	w.TTL = w.ExpiresAt.Unix()
	return nil
}

// WalletCredential represents a linked wallet
type WalletCredential struct {
	// DynamoDB keys - Primary key is user's credentials
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Business fields matching storage.WalletCredential
	Username string    `json:"username"`
	Address  string    `json:"address"`
	ChainID  int       `json:"chain_id"`
	Type     string    `json:"type"` // ethereum, solana, etc.
	ENS      string    `json:"ens,omitempty"`
	LinkedAt time.Time `json:"linked_at"`
	LastUsed time.Time `json:"last_used"`
}

// TableName returns the DynamoDB table name
func (WalletCredential) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating
func (w *WalletCredential) BeforeCreate() error {
	if err := w.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	if w.LinkedAt.IsZero() {
		w.LinkedAt = time.Now()
	}
	if w.LastUsed.IsZero() {
		w.LastUsed = w.LinkedAt
	}
	return nil
}

// BeforeUpdate updates the last used timestamp
func (w *WalletCredential) BeforeUpdate() error {
	w.LastUsed = time.Now()
	return nil
}

// GetPK returns the partition key
func (w *WalletCredential) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WalletCredential) GetSK() string {
	return w.SK
}

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WalletCredential) UpdateKeys() error {
	// Normalize address to lowercase
	address := strings.ToLower(w.Address)

	// Primary key for user's wallets
	w.PK = fmt.Sprintf(KeyPatternUser, w.Username)
	w.SK = fmt.Sprintf("WALLET#%s", address)
	return nil
}

// WalletIndex represents a reverse index for wallet->user lookup
type WalletIndex struct {
	// DynamoDB keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Business fields
	Username string `json:"username"`
}

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WalletIndex) UpdateKeys(walletType, address, username string) {
	// Normalize address to lowercase
	address = strings.ToLower(address)

	w.PK = fmt.Sprintf("WALLET#%s#%s", walletType, address)
	w.SK = fmt.Sprintf(KeyPatternUser, username)
	w.Username = username
}
