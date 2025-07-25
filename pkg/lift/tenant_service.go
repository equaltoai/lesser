package lift

import (
	"context"
	"fmt"

	"github.com/pay-theory/lift/pkg/lift"
)

// TenantService provides tenant-aware database operations using DynamORM patterns
type TenantService struct {
	tableName string
}

// NewTenantService creates a new tenant service
func NewTenantService(tableName string) *TenantService {
	return &TenantService{
		tableName: tableName,
	}
}

// TenantQueryBuilder provides tenant-aware query building
type TenantQueryBuilder struct {
	tenantID  string
	tableName string
}

// NewTenantQueryBuilder creates a query builder for a specific tenant
func (s *TenantService) ForTenant(tenantID string) *TenantQueryBuilder {
	return &TenantQueryBuilder{
		tenantID:  tenantID,
		tableName: s.tableName,
	}
}

// GetPartitionKey returns the tenant partition key
func (tqb *TenantQueryBuilder) GetPartitionKey() string {
	return fmt.Sprintf("tenant#%s", tqb.tenantID)
}

// QueryByEntityType queries all entities of a specific type for the tenant
func (tqb *TenantQueryBuilder) QueryByEntityType(ctx context.Context, entityType string, dest interface{}) error {
	// In a real implementation, this would use DynamORM's Query API
	// with the tenant-entity GSI
	fmt.Printf("DynamORM Query: tenant=%s, entity_type=%s, table=%s\n", 
		tqb.tenantID, entityType, tqb.tableName)
	
	// Example DynamORM query pattern:
	// return db.Model(dest).
	//   Index("tenant-entity").
	//   Where("tenant_id", "=", tqb.tenantID).
	//   Where("entity_type", "=", entityType).
	//   All(dest)
	
	return nil
}

// GetByID gets a specific entity by ID within the tenant
func (tqb *TenantQueryBuilder) GetByID(ctx context.Context, entityType, entityID string, dest interface{}) error {
	pk := tqb.GetPartitionKey()
	sk := fmt.Sprintf("%s#%s", entityType, entityID)
	
	fmt.Printf("DynamORM GetItem: PK=%s, SK=%s, table=%s\n", pk, sk, tqb.tableName)
	
	// Example DynamORM get pattern:
	// return db.Model(dest).
	//   Where("pk", "=", pk).
	//   Where("sk", "=", sk).
	//   First(dest)
	
	return nil
}

// Create creates a new tenant-aware entity
func (tqb *TenantQueryBuilder) Create(ctx context.Context, model interface{}) error {
	fmt.Printf("DynamORM Create: tenant=%s, table=%s\n", tqb.tenantID, tqb.tableName)
	
	// Example DynamORM create pattern:
	// return db.Model(model).Create()
	
	return nil
}

// Update updates a tenant-aware entity
func (tqb *TenantQueryBuilder) Update(ctx context.Context, model interface{}, fields ...string) error {
	fmt.Printf("DynamORM Update: tenant=%s, table=%s\n", tqb.tenantID, tqb.tableName)
	
	// Example DynamORM update pattern:
	// return db.Model(model).Update(fields...)
	
	return nil
}

// Delete deletes a tenant-aware entity
func (tqb *TenantQueryBuilder) Delete(ctx context.Context, entityType, entityID string) error {
	pk := tqb.GetPartitionKey()
	sk := fmt.Sprintf("%s#%s", entityType, entityID)
	
	fmt.Printf("DynamORM Delete: PK=%s, SK=%s, table=%s\n", pk, sk, tqb.tableName)
	
	// Example DynamORM delete pattern:
	// model := struct {
	//   PK string `dynamorm:"pk"`
	//   SK string `dynamorm:"sk"`
	// }{PK: pk, SK: sk}
	// return db.Model(&model).Delete()
	
	return nil
}

// TenantServiceFromContext creates a tenant service from Lift context
func TenantServiceFromContext(ctx *lift.Context, tableName string) (*TenantQueryBuilder, error) {
	// Use the same tenant ID extraction as the middleware
	tenantID, ok := ctx.Get("tenant_id").(string)
	if !ok || tenantID == "" {
		return nil, fmt.Errorf("tenant context required")
	}
	
	service := NewTenantService(tableName)
	return service.ForTenant(tenantID), nil
}

// RepositoryExample shows how to use the tenant service in a repository pattern
type UserRepository struct {
	service *TenantService
}

// NewUserRepository creates a new user repository
func NewUserRepository(tableName string) *UserRepository {
	return &UserRepository{
		service: NewTenantService(tableName),
	}
}

// CreateUser creates a user within a tenant context
func (r *UserRepository) CreateUser(ctx *lift.Context, user *TenantUser) error {
	tenantBuilder, err := TenantServiceFromContext(ctx, r.service.tableName)
	if err != nil {
		return err
	}
	
	return tenantBuilder.Create(ctx.Context, user)
}

// GetUser gets a user by ID within the tenant context
func (r *UserRepository) GetUser(ctx *lift.Context, userID string) (*TenantUser, error) {
	tenantBuilder, err := TenantServiceFromContext(ctx, r.service.tableName)
	if err != nil {
		return nil, err
	}
	
	user := &TenantUser{}
	err = tenantBuilder.GetByID(ctx.Context, "user", userID, user)
	if err != nil {
		return nil, err
	}
	
	return user, nil
}

// ListUsers lists all users for the current tenant
func (r *UserRepository) ListUsers(ctx *lift.Context) ([]*TenantUser, error) {
	tenantBuilder, err := TenantServiceFromContext(ctx, r.service.tableName)
	if err != nil {
		return nil, err
	}
	
	var users []*TenantUser
	err = tenantBuilder.QueryByEntityType(ctx.Context, "user", &users)
	if err != nil {
		return nil, err
	}
	
	return users, nil
}