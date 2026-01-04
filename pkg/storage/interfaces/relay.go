// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// RelayRepository defines the interface for ActivityPub relay operations.
// This handles relay information storage, retrieval, and status management.
type RelayRepository interface {
	// ===== Core Relay Operations =====

	// StoreRelayInfo stores relay information
	StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error

	// GetRelayInfo retrieves relay information
	GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error)

	// RemoveRelayInfo removes relay information
	RemoveRelayInfo(ctx context.Context, relayURL string) error

	// ===== Relay Listing Operations =====

	// GetActiveRelays retrieves all active relays
	GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error)

	// GetAllRelays retrieves all relays with pagination
	GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error)

	// ListRelays retrieves all relays (alias for GetAllRelays without pagination)
	ListRelays(ctx context.Context) ([]*storage.RelayInfo, error)

	// ===== Relay Status Operations =====

	// UpdateRelayStatus updates the active status of a relay
	UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error

	// UpdateRelayState updates multiple relay fields beyond just active status
	UpdateRelayState(ctx context.Context, relayURL string, state storage.RelayState) error

	// ===== CRUD Aliases =====

	// CreateRelay creates a new relay
	CreateRelay(ctx context.Context, relay *storage.RelayInfo) error

	// GetRelay retrieves a relay by URL (alias for GetRelayInfo)
	GetRelay(ctx context.Context, relayURL string) (*storage.RelayInfo, error)

	// DeleteRelay removes a relay (alias for RemoveRelayInfo)
	DeleteRelay(ctx context.Context, relayURL string) error
}
