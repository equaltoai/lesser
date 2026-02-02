package models

import (
	"fmt"
	"strings"
	"time"
)

// WalletChallenge represents a challenge for wallet authentication
type WalletChallenge struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	// Business fields matching storage.WalletChallenge
	ID        string    `theorydb:"attr:id" json:"id"`
	Username  string    `theorydb:"attr:username" json:"username,omitempty"`
	Address   string    `theorydb:"attr:address" json:"address"`
	ChainID   int       `theorydb:"attr:chainID" json:"chain_id"`
	Nonce     string    `theorydb:"attr:nonce" json:"nonce"`
	Message   string    `theorydb:"attr:message" json:"message"`
	IssuedAt  time.Time `theorydb:"attr:issuedAt" json:"issued_at"`
	ExpiresAt time.Time `theorydb:"attr:expiresAt" json:"expires_at"`
	Used      bool      `theorydb:"attr:used" json:"used"`   // Set after first verification (wallet/verify)
	Spent     bool      `theorydb:"attr:spent" json:"spent"` // Set after second verification (wallet/link)
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
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys - Primary key is user's credentials
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Business fields matching storage.WalletCredential
	Username string    `theorydb:"attr:username" json:"username"`
	Address  string    `theorydb:"attr:address" json:"address"`
	ChainID  int       `theorydb:"attr:chainID" json:"chain_id"`
	Type     string    `theorydb:"attr:type" json:"type"` // ethereum, solana, etc.
	ENS      string    `theorydb:"attr:ens" json:"ens,omitempty"`
	LinkedAt time.Time `theorydb:"attr:linkedAt" json:"linked_at"`
	LastUsed time.Time `theorydb:"attr:lastUsed" json:"last_used"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys
	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// Business fields
	Username   string `theorydb:"attr:username" json:"username"`
	WalletType string `theorydb:"attr:walletType" json:"wallet_type"` // Need to store these for BeforeCreate
	Address    string `theorydb:"attr:address" json:"address"`
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
