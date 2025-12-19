package dynamorm

import (
	"fmt"
	"time"
)

// StandardModel provides the base structure for all DynamORM models
// with standardized primary key fields and timestamps
type StandardModel struct {
	// Primary keys using standard naming
	PK string `dynamorm:"pk" json:"pk"` // entity_type#{id}
	SK string `dynamorm:"sk" json:"sk"` // entity_type#{id} or hierarchical structure

	// Standard timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate is a hook that sets CreatedAt and UpdatedAt before creating a record
func (m *StandardModel) BeforeCreate() error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	return nil
}

// BeforeUpdate is a hook that updates the UpdatedAt timestamp before updating a record
func (m *StandardModel) BeforeUpdate() error {
	m.UpdatedAt = time.Now()
	return nil
}

// TenantModel extends StandardModel with tenant isolation
type TenantModel struct {
	StandardModel

	// Tenant ID for multi-tenant applications
	TenantID string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"tenant_id"`
}

// KeyComponents defines the components of a composite key
type KeyComponents struct {
	EntityType string
	ID         string
	Tenant     string
	Sort       string
}

// GenerateKeys generates standard PK and SK values based on the provided components
func GenerateKeys(components KeyComponents) (string, string) {
	// Generate partition key
	var pk string
	if components.Tenant != "" {
		pk = fmt.Sprintf("tenant#%s#%s#%s", components.Tenant, components.EntityType, components.ID)
	} else {
		pk = fmt.Sprintf("%s#%s", components.EntityType, components.ID)
	}

	// Generate sort key
	var sk string
	if components.Sort != "" {
		sk = components.Sort
	} else {
		sk = fmt.Sprintf("%s#%s", components.EntityType, components.ID)
	}

	return pk, sk
}

// GenerateSimpleKeys generates simple PK and SK values for a single entity
func GenerateSimpleKeys(entityType, id string) (string, string) {
	pk := fmt.Sprintf("%s#%s", entityType, id)
	sk := pk // Same as PK for simple entities
	return pk, sk
}

// GenerateTenantKeys generates PK and SK values for tenant-isolated entities
func GenerateTenantKeys(tenantID, entityType, id string) (string, string) {
	pk := fmt.Sprintf("tenant#%s", tenantID)
	sk := fmt.Sprintf("%s#%s", entityType, id)
	return pk, sk
}

// GenerateHierarchicalKeys generates PK and SK values for hierarchical data
// For example, a user's posts would have PK=user#{user_id} and SK=post#{post_id}
func GenerateHierarchicalKeys(parentType, parentID, childType, childID string) (string, string) {
	pk := fmt.Sprintf("%s#%s", parentType, parentID)
	sk := fmt.Sprintf("%s#%s", childType, childID)
	return pk, sk
}

// ExtractIDFromKey extracts the ID portion from a composite key
// For example, "user#123" would return "123"
func ExtractIDFromKey(key, prefix string) string {
	prefixWithHash := prefix + "#"
	if len(key) > len(prefixWithHash) && key[:len(prefixWithHash)] == prefixWithHash {
		return key[len(prefixWithHash):]
	}
	return key
}

// TTLModel extends StandardModel with TTL support
type TTLModel struct {
	StandardModel

	// TTL for automatic expiration
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// SetTTL sets the TTL value based on the provided duration from now
func (m *TTLModel) SetTTL(duration time.Duration) {
	m.TTL = time.Now().Add(duration).Unix()
}
