package models

import (
	"fmt"
	"strings"
	"time"
)

// WalletChallenge represents a challenge for wallet authentication
type WalletChallenge struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB keys
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"-"`

	// Business fields matching storage.WalletChallenge
	ID        string    `dynamorm:"attr:id" json:"id"`
	Username  string    `dynamorm:"attr:username" json:"username,omitempty"`
	Address   string    `dynamorm:"attr:address" json:"address"`
	ChainID   int       `dynamorm:"attr:chainID" json:"chain_id"`
	Nonce     string    `dynamorm:"attr:nonce" json:"nonce"`
	Message   string    `dynamorm:"attr:message" json:"message"`
	IssuedAt  time.Time `dynamorm:"attr:issuedAt" json:"issued_at"`
	ExpiresAt time.Time `dynamorm:"attr:expiresAt" json:"expires_at"`
	Used      bool      `dynamorm:"attr:used" json:"used"`   // Set after first verification (wallet/verify)
	Spent     bool      `dynamorm:"attr:spent" json:"spent"` // Set after second verification (wallet/link)
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB keys - Primary key is user's credentials
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Business fields matching storage.WalletCredential
	Username string    `dynamorm:"attr:username" json:"username"`
	Address  string    `dynamorm:"attr:address" json:"address"`
	ChainID  int       `dynamorm:"attr:chainID" json:"chain_id"`
	Type     string    `dynamorm:"attr:type" json:"type"` // ethereum, solana, etc.
	ENS      string    `dynamorm:"attr:ens" json:"ens,omitempty"`
	LinkedAt time.Time `dynamorm:"attr:linkedAt" json:"linked_at"`
	LastUsed time.Time `dynamorm:"attr:lastUsed" json:"last_used"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB keys
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// Business fields
	Username   string `dynamorm:"attr:username" json:"username"`
	WalletType string `dynamorm:"attr:walletType" json:"wallet_type"` // Need to store these for BeforeCreate
	Address    string `dynamorm:"attr:address" json:"address"`
}

// GetPK returns the partition key
func (w *WalletIndex) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WalletIndex) GetSK() string {
	return w.SK
}

// BeforeCreate sets up the keys before creation
func (w *WalletIndex) BeforeCreate() error {
	if w.WalletType == "" {
		w.WalletType = "ethereum" // Default
	}
	w.UpdateKeys(w.WalletType, w.Address, w.Username)
	return nil
}

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WalletIndex) UpdateKeys(walletType, address, username string) {
	// Normalize address to lowercase
	address = strings.ToLower(address)

	w.PK = fmt.Sprintf("WALLET#%s#%s", walletType, address)
	w.SK = fmt.Sprintf(KeyPatternUser, username)
	w.Username = username
	w.WalletType = walletType
	w.Address = address
}

// TableName returns the DynamoDB table backing WalletIndex.
func (WalletIndex) TableName() string {
	return MainTableName
}
