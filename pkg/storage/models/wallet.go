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
	return "lesser-main"
}

// BeforeCreate sets up the keys and TTL before creating
func (w *WalletChallenge) BeforeCreate() error {
	w.UpdateKeys()
	if w.IssuedAt.IsZero() {
		w.IssuedAt = time.Now()
	}
	return nil
}

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WalletChallenge) UpdateKeys() {
	w.PK = fmt.Sprintf("WALLET_CHALLENGE#%s", w.ID)
	w.SK = "CHALLENGE"
	// Set TTL to expiration time
	w.TTL = w.ExpiresAt.Unix()
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
	return "lesser-main"
}

// BeforeCreate sets up the keys before creating
func (w *WalletCredential) BeforeCreate() error {
	w.UpdateKeys()
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

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WalletCredential) UpdateKeys() {
	// Normalize address to lowercase
	address := strings.ToLower(w.Address)
	
	// Primary key for user's wallets
	w.PK = fmt.Sprintf("USER#%s", w.Username)
	w.SK = fmt.Sprintf("WALLET#%s", address)
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
	w.SK = fmt.Sprintf("USER#%s", username)
	w.Username = username
}