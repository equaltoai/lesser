package repositories

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testServicesModel struct {
	ID        string
	Name      string
	PK        string
	SK        string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (m *testServicesModel) UpdateKeys() error {
	m.PK = "PK#" + m.ID
	m.SK = "SK#" + m.ID
	return nil
}

func (m *testServicesModel) GetPK() string { return m.PK }
func (m *testServicesModel) GetSK() string { return m.SK }

type testServicesBadKeys struct {
	PK string
	SK string
}

func (m *testServicesBadKeys) UpdateKeys() error { return fmt.Errorf("boom") }
func (m *testServicesBadKeys) GetPK() string     { return m.PK }
func (m *testServicesBadKeys) GetSK() string     { return m.SK }

type testServicesEmptyKeys struct {
	PK string
	SK string
}

func (m *testServicesEmptyKeys) UpdateKeys() error { return nil }
func (m *testServicesEmptyKeys) GetPK() string     { return m.PK }
func (m *testServicesEmptyKeys) GetSK() string     { return m.SK }

type testClaims struct{ username string }

func (c testClaims) HasScope(string) bool { return true }
func (c testClaims) GetUsername() string  { return c.username }

type testTaggedRequiredModel struct {
	Foo string `validate:"required"`
	PK  string
	SK  string
}

func (m *testTaggedRequiredModel) UpdateKeys() error { return nil }
func (m *testTaggedRequiredModel) GetPK() string     { return m.PK }
func (m *testTaggedRequiredModel) GetSK() string     { return m.SK }

type testEventHandler struct {
	seen []Event
	err  error
}

func (h *testEventHandler) Handle(_ context.Context, event Event) error {
	h.seen = append(h.seen, event)
	return h.err
}

func TestServices_DefaultValidationService(t *testing.T) {
	t.Run("validate_model_nil_and_key_generation_errors", func(t *testing.T) {
		svc := NewDefaultValidationService()

		err := svc.ValidateModel(context.Background(), nil)
		assert.Error(t, err)

		err = svc.ValidateModel(context.Background(), &testServicesBadKeys{})
		assert.Error(t, err)
	})

	t.Run("validate_model_requires_pk_sk", func(t *testing.T) {
		svc := NewDefaultValidationService()

		err := svc.ValidateModel(context.Background(), &testServicesEmptyKeys{PK: "", SK: "sk"})
		assert.Error(t, err)

		err = svc.ValidateModel(context.Background(), &testServicesEmptyKeys{PK: "pk", SK: ""})
		assert.Error(t, err)
	})

	t.Run("validate_business_rules_nil_pointer_and_create_update", func(t *testing.T) {
		svc := NewDefaultValidationService()

		var nilPtr *testServicesModel
		err := svc.ValidateBusinessRules(context.Background(), nilPtr, common.OperationCreate)
		assert.Error(t, err)

		createdMissing := &testServicesModel{ID: "c1"}
		require.NoError(t, createdMissing.UpdateKeys())
		err = svc.ValidateBusinessRules(context.Background(), createdMissing, common.OperationCreate)
		assert.Error(t, err)

		createdOK := &testServicesModel{
			ID:        "c2",
			CreatedAt: time.Now(),
		}
		require.NoError(t, createdOK.UpdateKeys())
		err = svc.ValidateBusinessRules(context.Background(), createdOK, common.OperationCreate)
		assert.NoError(t, err)

		updatedMissing := &testServicesModel{ID: "u1"}
		require.NoError(t, updatedMissing.UpdateKeys())
		err = svc.ValidateBusinessRules(context.Background(), updatedMissing, common.OperationUpdate)
		assert.Error(t, err)

		updatedOK := &testServicesModel{
			ID:        "u2",
			UpdatedAt: time.Now(),
		}
		require.NoError(t, updatedOK.UpdateKeys())
		err = svc.ValidateBusinessRules(context.Background(), updatedOK, common.OperationUpdate)
		assert.NoError(t, err)
	})

	t.Run("validate_required_fields_tags_and_common_names", func(t *testing.T) {
		svc := NewDefaultValidationService()

		err := svc.ValidateRequiredFields(context.Background(), &testTaggedRequiredModel{})
		assert.Error(t, err)

		// Common required field name: ID.
		err = svc.ValidateRequiredFields(context.Background(), &testServicesModel{})
		assert.Error(t, err)
	})
}

func TestServices_DefaultPermissionService(t *testing.T) {
	p := NewDefaultPermissionService()

	t.Run("has_permission", func(t *testing.T) {
		assert.False(t, p.HasPermission(context.Background(), "", "read"))
		assert.True(t, p.HasPermission(context.Background(), "admin-user", "anything"))
		assert.True(t, p.HasPermission(context.Background(), "mod-1", "moderator"))
		assert.True(t, p.HasPermission(context.Background(), "user-1", "create"))
		assert.False(t, p.HasPermission(context.Background(), "user-1", "admin"))
	})

	t.Run("check_permissions_update_and_delete", func(t *testing.T) {
		err := p.CheckPermissions(context.Background(), "", "update", &testPermissionResource{PK: "USER#x"})
		assert.Error(t, err)

		// Owner can update/delete.
		err = p.CheckPermissions(context.Background(), "alice", "update", &testPermissionResource{PK: "POST#ALICE#1"})
		assert.NoError(t, err)
		err = p.CheckPermissions(context.Background(), "alice", "delete", &testPermissionResource{PK: "POST#ALICE#1"})
		assert.NoError(t, err)

		// Non-owner can't update/delete unless admin.
		err = p.CheckPermissions(context.Background(), "bob", "update", &testPermissionResource{PK: "POST#ALICE#1"})
		assert.Error(t, err)
		err = p.CheckPermissions(context.Background(), "admin", "update", &testPermissionResource{PK: "POST#ALICE#1"})
		assert.NoError(t, err)

		// Unknown action -> generic permission check.
		err = p.CheckPermissions(context.Background(), "bob", "ban", &testPermissionResource{PK: "POST#ALICE#1"})
		assert.Error(t, err)
		err = p.CheckPermissions(context.Background(), "admin", "ban", &testPermissionResource{PK: "POST#ALICE#1"})
		assert.NoError(t, err)
	})
}

func TestServices_InMemoryCachingService(t *testing.T) {
	cache := NewInMemoryCachingService()

	t.Run("get_missing_and_invalid_dest", func(t *testing.T) {
		var out string
		err := cache.Get(context.Background(), "missing", &out)
		assert.Error(t, err)

		_ = cache.Set(context.Background(), "k1", "v1", time.Minute)
		err = cache.Get(context.Background(), "k1", out) // not a pointer
		assert.Error(t, err)
	})

	t.Run("set_get_delete_and_expiration", func(t *testing.T) {
		err := cache.Set(context.Background(), "k2", "v2", time.Hour)
		require.NoError(t, err)

		var out string
		require.NoError(t, cache.Get(context.Background(), "k2", &out))
		assert.Equal(t, "v2", out)

		require.NoError(t, cache.Delete(context.Background(), "k2"))
		err = cache.Get(context.Background(), "k2", &out)
		assert.Error(t, err)

		// Expired entries should be removed.
		require.NoError(t, cache.Set(context.Background(), "k3", "v3", -1*time.Second))
		err = cache.Get(context.Background(), "k3", &out)
		assert.Error(t, err)
	})

	t.Run("invalidate_pattern", func(t *testing.T) {
		_ = cache.Set(context.Background(), "user:1", "a", time.Hour)
		_ = cache.Set(context.Background(), "user:2", "b", time.Hour)
		_ = cache.Set(context.Background(), "post:1", "c", time.Hour)

		require.NoError(t, cache.InvalidatePattern(context.Background(), "user:"))

		var out string
		assert.Error(t, cache.Get(context.Background(), "user:1", &out))
		assert.Error(t, cache.Get(context.Background(), "user:2", &out))
		require.NoError(t, cache.Get(context.Background(), "post:1", &out))
	})
}

func TestServices_DefaultEventService(t *testing.T) {
	t.Run("emit_success_and_handler_error", func(t *testing.T) {
		svc := NewDefaultEventService()
		h1 := &testEventHandler{}
		h2 := &testEventHandler{err: fmt.Errorf("fail")}

		svc.AddHandler(h1)
		svc.AddHandler(h2)

		err := svc.Emit(context.Background(), NewEvent("t", "e", "id", "a", map[string]string{}))
		assert.Error(t, err)
		assert.Len(t, h1.seen, 1)
	})

	t.Run("log_event_handler_handle", func(t *testing.T) {
		h := NewLogEventHandler()
		assert.NoError(t, h.Handle(context.Background(), Event{Entity: "x", Action: "y", EntityID: "z", Type: "t", Actor: "a"}))
	})
}

type testPermissionResource struct{ PK, SK string }

func (r *testPermissionResource) UpdateKeys() error { return nil }
func (r *testPermissionResource) GetPK() string     { return r.PK }
func (r *testPermissionResource) GetSK() string     { return r.SK }

func TestServices_round09_additional_branches(t *testing.T) {
	ctx := context.Background()

	t.Run("validation_delete_action_and_is_field_empty", func(t *testing.T) {
		svc := NewDefaultValidationService()

		model := &testServicesModel{ID: "1", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		_ = model.UpdateKeys()
		assert.NoError(t, svc.ValidateBusinessRules(ctx, model, common.OperationDelete))

		assert.True(t, svc.isFieldEmpty(reflect.ValueOf("")))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf([]string{})))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf(map[string]string{})))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf([0]int{})))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf((*string)(nil))))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf(time.Time{})))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf(int64(0))))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf(uint64(0))))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf(float64(0))))
		assert.True(t, svc.isFieldEmpty(reflect.ValueOf(false)))
		assert.False(t, svc.isFieldEmpty(reflect.ValueOf(struct{ X int }{X: 1})))
	})

	t.Run("permission_create_read_and_delete_denied", func(t *testing.T) {
		p := NewDefaultPermissionService()
		err := p.CheckPermissions(ctx, "alice", "create", &testPermissionResource{PK: "POST#1"})
		assert.NoError(t, err)
		err = p.CheckPermissions(ctx, "alice", "read", &testPermissionResource{PK: "POST#1"})
		assert.NoError(t, err)
		err = p.CheckPermissions(ctx, "bob", "delete", &testPermissionResource{PK: "POST#ALICE#1"})
		assert.Error(t, err)
	})

	t.Run("event_service_emit_success", func(t *testing.T) {
		svc := NewDefaultEventService()
		h1 := &testEventHandler{}
		svc.AddHandler(h1)
		require.NoError(t, svc.Emit(ctx, Event{Type: "t", Entity: "e"}))
	})
}
