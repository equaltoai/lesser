package lift

import (
	"context"
	"testing"

	dynamormMocks "github.com/pay-theory/dynamorm/pkg/mocks"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTenantQueryBuilder_DBMethodChains(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)

	q.On("Index", "gsi1").Return(q)
	q.On("Where", "gsi1PK", "=", "TENANT#t1").Return(q)
	q.On("Where", "gsi1SK", "=", "user").Return(q)
	dest := &[]any{}
	q.On("All", dest).Return(nil)

	qb := NewTenantService(db, "tbl", zap.NewNop()).ForTenant("t1")
	require.Equal(t, "tenant#t1", qb.GetPartitionKey())
	require.NoError(t, qb.QueryByEntityType(context.Background(), "user", dest))
}

func TestTenantQueryBuilder_GetByID(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)

	q.On("Where", "pk", "=", "tenant#t1").Return(q)
	q.On("Where", "sk", "=", "user#1").Return(q)

	dest := &TenantUser{}
	q.On("First", dest).Return(nil)

	qb := NewTenantService(db, "tbl", zap.NewNop()).ForTenant("t1")
	require.NoError(t, qb.GetByID(context.Background(), "user", "1", dest))
}

func TestTenantQueryBuilder_CreateUpdateDelete(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)

	q.On("Create").Return(nil)
	q.On("Update", mock.Anything).Return(nil)
	q.On("Delete").Return(nil)

	qb := NewTenantService(db, "tbl", zap.NewNop()).ForTenant("t1")
	require.NoError(t, qb.Create(context.Background(), &TenantUser{UserID: "1"}))
	require.NoError(t, qb.Update(context.Background(), &TenantUser{UserID: "1"}, "name"))
	require.NoError(t, qb.Update(context.Background(), &TenantUser{UserID: "1"}))
	require.NoError(t, qb.Delete(context.Background(), "user", "1"))
}

func TestTenantServiceFromContext(t *testing.T) {
	db := new(dynamormMocks.MockDB)

	ctx := createTestContext()
	_, err := TenantServiceFromContext(ctx, db, "tbl", zap.NewNop())
	require.Error(t, err)

	ctx.Set("tenant_id", "t1")
	_, err = TenantServiceFromContext(ctx, db, "tbl", zap.NewNop())
	require.NoError(t, err)
}

func TestUserRepository_UsesTenantBuilder(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)

	q.On("Create").Return(nil)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("First", mock.Anything).Return(nil)
	q.On("Index", "gsi1").Return(q)
	q.On("All", mock.Anything).Return(nil)

	repo := NewUserRepository(db, "tbl", zap.NewNop())

	ctx := createTestContext()
	ctx.Logger = &liftPkg.NoOpLogger{}
	ctx.Set("tenant_id", "t1")

	require.NoError(t, repo.CreateUser(ctx, &TenantUser{UserID: "1"}))

	user, err := repo.GetUser(ctx, "1")
	require.NoError(t, err)
	require.NotNil(t, user)

	users, err := repo.ListUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, len(users))
}
