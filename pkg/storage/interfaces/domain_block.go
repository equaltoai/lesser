// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
)

// DomainBlockRepository defines the interface for domain block operations.
// This handles both user-level and instance-level domain blocking.
type DomainBlockRepository interface {
	// User domain block operations

	// AddDomainBlock adds a domain to the user's block list
	AddDomainBlock(ctx context.Context, username, domain string) error

	// RemoveDomainBlock removes a domain from the user's block list
	RemoveDomainBlock(ctx context.Context, username, domain string) error

	// GetUserDomainBlocks retrieves all domains blocked by a user
	GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)

	// IsBlockedDomain checks if a domain is blocked by a user
	IsBlockedDomain(ctx context.Context, username, domain string) (bool, error)

	// Instance domain block operations

	// CreateInstanceDomainBlock creates an instance-level domain block
	CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error

	// GetInstanceDomainBlock retrieves a domain block by domain
	GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error)

	// GetInstanceDomainBlockByID retrieves a domain block by ID
	GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error)

	// ListInstanceDomainBlocks lists all instance domain blocks with pagination
	ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error)

	// UpdateInstanceDomainBlock updates an existing domain block
	UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error

	// DeleteInstanceDomainBlock deletes a domain block
	DeleteInstanceDomainBlock(ctx context.Context, domain string) error

	// IsInstanceDomainBlocked checks if a domain is blocked at the instance level
	IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error)

	// Alias methods for backward compatibility

	// GetDomainBlocks retrieves instance-level domain blocks with pagination
	GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error)

	// GetDomainBlock retrieves a specific domain block by ID
	GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error)

	// CreateDomainBlock creates a new instance-level domain block
	CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error

	// UpdateDomainBlock updates an existing domain block
	UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error

	// DeleteDomainBlock removes a domain block
	DeleteDomainBlock(ctx context.Context, id string) error

	// IsDomainBlocked checks if a domain is blocked at the instance level
	IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error)

	// Email domain block operations

	// CreateEmailDomainBlock creates an email domain block
	CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error

	// GetEmailDomainBlocks retrieves email domain blocks with pagination
	GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error)

	// DeleteEmailDomainBlock deletes an email domain block
	DeleteEmailDomainBlock(ctx context.Context, id string) error

	// Domain allow operations (for allowlist mode)

	// GetDomainAllows retrieves domain allows (for allowlist mode)
	GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error)

	// CreateDomainAllow adds a domain to the allowlist
	CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error

	// DeleteDomainAllow removes a domain from the allowlist
	DeleteDomainAllow(ctx context.Context, id string) error
}
