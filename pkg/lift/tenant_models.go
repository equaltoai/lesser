package lift

import (
	"fmt"
	"time"
)

// TenantAwareModel provides base structure for tenant-isolated DynamoDB models
// Following the official DynamORM multi-tenant pattern
type TenantAwareModel struct {
	PK         string    `dynamorm:"pk" json:"pk"`                              // tenant#{tenant_id}
	SK         string    `dynamorm:"sk" json:"sk"`                              // entity#{id}
	TenantID   string    `dynamorm:"index:tenant-entity,pk" json:"tenant_id"`   // For GSI
	EntityType string    `dynamorm:"index:tenant-entity,sk" json:"entity_type"` // For GSI
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NewTenantAwareModel creates a new tenant-aware model instance
func NewTenantAwareModel(tenantID, entityType, entityID string) TenantAwareModel {
	return TenantAwareModel{
		PK:         fmt.Sprintf("tenant#%s", tenantID),
		SK:         fmt.Sprintf("%s#%s", entityType, entityID),
		TenantID:   tenantID,
		EntityType: entityType,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// GetPartitionKey returns the partition key for tenant isolation
func (m *TenantAwareModel) GetPartitionKey() string {
	return m.PK
}

// GetSortKey returns the sort key
func (m *TenantAwareModel) GetSortKey() string {
	return m.SK
}

// UpdateTimestamp updates the UpdatedAt field
func (m *TenantAwareModel) UpdateTimestamp() {
	m.UpdatedAt = time.Now()
}

// ValidateTenant ensures the model belongs to the specified tenant
func (m *TenantAwareModel) ValidateTenant(tenantID string) bool {
	return m.TenantID == tenantID
}

// Example tenant-aware models following the official pattern

// TenantUser represents a user within a tenant
type TenantUser struct {
	TenantAwareModel

	// User-specific fields
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

// NewTenantUser creates a new tenant user
func NewTenantUser(tenantID, userID string) *TenantUser {
	return &TenantUser{
		TenantAwareModel: NewTenantAwareModel(tenantID, "user", userID),
		UserID:           userID,
		Status:           "active",
	}
}

// TenantProject represents a project within a tenant
type TenantProject struct {
	TenantAwareModel

	// Project-specific fields
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	OwnerID     string `json:"owner_id"`
}

// NewTenantProject creates a new tenant project
func NewTenantProject(tenantID, projectID, ownerID string) *TenantProject {
	return &TenantProject{
		TenantAwareModel: NewTenantAwareModel(tenantID, "project", projectID),
		ProjectID:        projectID,
		OwnerID:          ownerID,
		Status:           "active",
	}
}

// TenantConfig represents tenant configuration
type TenantConfig struct {
	TenantAwareModel

	// Config-specific fields
	Name      string            `json:"name"`
	Settings  map[string]string `json:"settings"`
	Plan      string            `json:"plan"`
	RateLimit int               `json:"rate_limit"`
	IsActive  bool              `json:"is_active"`
}

// NewTenantConfig creates a new tenant configuration
func NewTenantConfig(tenantID string) *TenantConfig {
	return &TenantConfig{
		TenantAwareModel: NewTenantAwareModel(tenantID, "config", tenantID),
		Plan:             "free",
		RateLimit:        100,
		IsActive:         true,
		Settings:         make(map[string]string),
	}
}
