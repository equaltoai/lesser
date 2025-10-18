package lift

import (
	"context"
	"fmt"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// TenantService provides tenant-aware database operations using DynamORM patterns
type TenantService struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewTenantService creates a new tenant service
func NewTenantService(db core.DB, tableName string, logger *zap.Logger) *TenantService {
	return &TenantService{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// TenantQueryBuilder provides tenant-aware query building
type TenantQueryBuilder struct {
	db        core.DB
	tenantID  string
	tableName string
	logger    *zap.Logger
}

// ForTenant creates a query builder for a specific tenant
func (s *TenantService) ForTenant(tenantID string) *TenantQueryBuilder {
	return &TenantQueryBuilder{
		db:        s.db,
		tenantID:  tenantID,
		tableName: s.tableName,
		logger:    s.logger,
	}
}

// GetPartitionKey returns the tenant partition key
func (tqb *TenantQueryBuilder) GetPartitionKey() string {
	return fmt.Sprintf("tenant#%s", tqb.tenantID)
}

// QueryByEntityType queries all entities of a specific type for the tenant
func (tqb *TenantQueryBuilder) QueryByEntityType(ctx context.Context, entityType string, dest interface{}) error {
	tqb.logger.Debug("QueryByEntityType",
		zap.String("tenant_id", tqb.tenantID),
		zap.String("entity_type", entityType),
		zap.String("table", tqb.tableName))

	// Use DynamORM's Query API with the tenant-entity GSI
	return tqb.db.WithContext(ctx).Model(dest).
		Index("tenant-entity").
		Where("tenant_id", "=", tqb.tenantID).
		Where("entity_type", "=", entityType).
		All(dest)
}

// GetByID gets a specific entity by ID within the tenant
func (tqb *TenantQueryBuilder) GetByID(ctx context.Context, entityType, entityID string, dest interface{}) error {
	pk := tqb.GetPartitionKey()
	sk := fmt.Sprintf("%s#%s", entityType, entityID)

	tqb.logger.Debug("GetByID",
		zap.String("pk", pk),
		zap.String("sk", sk),
		zap.String("table", tqb.tableName))

	// Use DynamORM get pattern
	return tqb.db.WithContext(ctx).Model(dest).
		Where("pk", "=", pk).
		Where("sk", "=", sk).
		First(dest)
}

// Create creates a new tenant-aware entity
func (tqb *TenantQueryBuilder) Create(ctx context.Context, model interface{}) error {
	tqb.logger.Debug("Create",
		zap.String("tenant_id", tqb.tenantID),
		zap.String("table", tqb.tableName))

	// Use DynamORM create pattern
	return tqb.db.WithContext(ctx).Model(model).Create()
}

// Update updates a tenant-aware entity
func (tqb *TenantQueryBuilder) Update(ctx context.Context, model interface{}, fields ...string) error {
	tqb.logger.Debug("Update",
		zap.String("tenant_id", tqb.tenantID),
		zap.String("table", tqb.tableName),
		zap.Strings("fields", fields))

	// Use DynamORM update pattern
	if len(fields) > 0 {
		return tqb.db.WithContext(ctx).Model(model).Update(fields...)
	}
	return tqb.db.WithContext(ctx).Model(model).Update()
}

// Delete deletes a tenant-aware entity
func (tqb *TenantQueryBuilder) Delete(ctx context.Context, entityType, entityID string) error {
	pk := tqb.GetPartitionKey()
	sk := fmt.Sprintf("%s#%s", entityType, entityID)

	tqb.logger.Debug("Delete",
		zap.String("pk", pk),
		zap.String("sk", sk),
		zap.String("table", tqb.tableName))

	// Use DynamORM delete pattern
	model := struct {
		PK string `dynamorm:"pk"`
		SK string `dynamorm:"sk"`
	}{PK: pk, SK: sk}
	return tqb.db.WithContext(ctx).Model(&model).Delete()
}

// TenantServiceFromContext creates a tenant service from Lift context
func TenantServiceFromContext(ctx *lift.Context, db core.DB, tableName string, logger *zap.Logger) (*TenantQueryBuilder, error) {
	// Use the same tenant ID extraction as the middleware
	tenantID, ok := ctx.Get("tenant_id").(string)
	if !ok || tenantID == "" {
		return nil, ErrTenantContextRequired
	}

	service := NewTenantService(db, tableName, logger)
	return service.ForTenant(tenantID), nil
}

// UserRepository shows how to use the tenant service in a repository pattern
type UserRepository struct {
	service *TenantService
}

// NewUserRepository creates a new user repository
func NewUserRepository(db core.DB, tableName string, logger *zap.Logger) *UserRepository {
	return &UserRepository{
		service: NewTenantService(db, tableName, logger),
	}
}

// CreateUser creates a user within a tenant context
func (r *UserRepository) CreateUser(ctx *lift.Context, user *TenantUser) error {
	tenantBuilder, err := TenantServiceFromContext(ctx, r.service.db, r.service.tableName, r.service.logger)
	if err != nil {
		return err
	}

	return tenantBuilder.Create(ctx.Context, user)
}

// GetUser gets a user by ID within the tenant context
func (r *UserRepository) GetUser(ctx *lift.Context, userID string) (*TenantUser, error) {
	tenantBuilder, err := TenantServiceFromContext(ctx, r.service.db, r.service.tableName, r.service.logger)
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
	tenantBuilder, err := TenantServiceFromContext(ctx, r.service.db, r.service.tableName, r.service.logger)
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
