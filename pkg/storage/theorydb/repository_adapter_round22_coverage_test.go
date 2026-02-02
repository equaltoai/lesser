package theorydb

import (
	"context"
	"errors"
	"testing"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormMocks "github.com/theory-cloud/tabletheory/pkg/mocks"
)

type round22Entity struct {
	PK string
	SK string
	ID string

	keysSet bool
}

func (e *round22Entity) SetKeys() {
	e.keysSet = true
	if e.ID != "" {
		e.PK, e.SK = GenerateSimpleKeys("user", e.ID)
	}
}

type errTxOps struct {
	putErr error
}

func (e *errTxOps) Put(_ any) error { return e.putErr }
func (e *errTxOps) Delete(_ any) error {
	return nil
}
func (e *errTxOps) Update(_ any) error {
	return nil
}
func (e *errTxOps) ConditionCheck(_ string, _ map[string]any, _ string, _ ...any) error {
	return nil
}

func TestGenericRepository_CRUD_Round22(t *testing.T) {
	t.Run("create without tx sets keys and calls Create", func(t *testing.T) {
		db := new(dynamormMocks.MockDB)
		q := new(dynamormMocks.MockQuery)

		entity := &round22Entity{ID: "123"}
		db.On("Model", entity).Return(q).Once()
		q.On("Create").Return(nil).Once()

		repo := NewGenericRepository(db, "t", "user")
		require.NoError(t, repo.Create(context.Background(), entity))
		require.True(t, entity.keysSet)
	})

	t.Run("create with tx maps errors", func(t *testing.T) {
		db := new(dynamormMocks.MockDB)
		repo := NewGenericRepository(db, "t", "user")

		tx := &Transaction{tx: &errTxOps{putErr: errors.New("conditional check failed")}}
		repo = repo.WithTransaction(tx)

		entity := &round22Entity{ID: "123"}
		err := repo.Create(context.Background(), entity)
		require.Error(t, err)
		require.True(t, appErrors.HasCode(err, appErrors.CodeConflict))
	})

	t.Run("get uses generated keys", func(t *testing.T) {
		db := new(dynamormMocks.MockDB)
		q := new(dynamormMocks.MockQuery)

		entity := &round22Entity{}
		db.On("Model", entity).Return(q).Once()
		q.On("Where", "PK", "=", "user#123").Return(q).Once()
		q.On("Where", "SK", "=", "user#123").Return(q).Once()
		q.On("First", entity).Return(nil).Once()

		repo := NewGenericRepository(db, "t", "user")
		require.NoError(t, repo.Get(context.Background(), "123", entity))
	})

	t.Run("update without tx calls Update", func(t *testing.T) {
		db := new(dynamormMocks.MockDB)
		q := new(dynamormMocks.MockQuery)

		entity := &round22Entity{ID: "123"}
		db.On("Model", entity).Return(q).Once()
		// handle either Update() calling style (varargs vs slice arg)
		q.On("Update").Return(nil).Maybe()
		q.On("Update", mock.Anything).Return(nil).Maybe()

		repo := NewGenericRepository(db, "t", "user")
		require.NoError(t, repo.Update(context.Background(), entity))
		require.True(t, entity.keysSet)
	})

	t.Run("delete builds key-only entity and calls Delete", func(t *testing.T) {
		db := new(dynamormMocks.MockDB)
		q := new(dynamormMocks.MockQuery)

		var seenPK, seenSK string
		db.On("Model", mock.Anything).Return(q).Run(func(args mock.Arguments) {
			arg := args.Get(0).(*round22Entity)
			seenPK, seenSK = arg.PK, arg.SK
		}).Once()

		q.On("Where", "PK", "=", "user#123").Return(q).Once()
		q.On("Where", "SK", "=", "user#123").Return(q).Once()
		q.On("Delete").Return(nil).Once()

		repo := NewGenericRepository(db, "t", "user")
		require.NoError(t, repo.Delete(context.Background(), "123", &round22Entity{}))
		require.Equal(t, "user#123", seenPK)
		require.Equal(t, "user#123", seenSK)
	})
}

func TestGenericRepository_List_Round22(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	db.On("Model", mock.Anything).Return(q).Once()
	q.On("Index", "gsi1").Return(q).Once()
	q.On("Where", "CreatedAt", ">", "2024-01-01").Return(q).Once()
	q.On("Where", "ActorID", "=", "alice").Return(q).Once()
	q.On("Where", "unrecognized", "<>", "ignored").Return(q).Once()
	q.On("All", mock.Anything).Return(nil).Once()

	repo := NewGenericRepository(db, "t", "user")
	var out []*round22Entity
	require.NoError(t, repo.List(context.Background(), map[string]any{
		"index:gsi1":      true,
		"CreatedAt:>":     "2024-01-01",
		"ActorID":         "alice",
		"unrecognized:<>": "ignored", // falls back to operator parsing
	}, &out))

	require.Error(t, repo.List(context.Background(), nil, out)) // not a pointer to slice
}

func TestGenericRepository_BatchGet_SkipsNotFound_Round22(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	q := new(dynamormMocks.MockQuery)

	repo := NewGenericRepository(db, "t", "user")

	// First ID -> not found, second -> found.
	db.On("Model", mock.Anything).Return(q).Twice()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Times(4)
	q.On("First", mock.Anything).Return(errors.New("record not found")).Once()
	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*round22Entity)
		dest.ID = "456"
	}).Return(nil).Once()

	var out []*round22Entity
	require.NoError(t, repo.BatchGet(context.Background(), []string{"123", "456"}, &out))
	require.Len(t, out, 1)
	require.Equal(t, "456", out[0].ID)
}

func TestRepositoryAdapterConfig_Round22(t *testing.T) {
	db := new(dynamormMocks.MockDB)
	cfg := NewRepositoryAdapterConfig(db, "t", "user").
		WithOriginalRepo("orig").
		WithConversion("toStorage", func() {})

	require.Equal(t, "t", cfg.TableName)
	require.Equal(t, "user", cfg.EntityType)
	require.Equal(t, "orig", cfg.OriginalRepo)
	require.Contains(t, cfg.ConversionFuncs, "toStorage")
}

func Test_getEntityID_Round22(t *testing.T) {
	require.Equal(t, "unknown", getEntityID(struct{}{}))
	require.Equal(t, "id", getEntityID(&struct{ ID string }{ID: "id"}))
	require.Equal(t, "id", getEntityID(&struct{ Id string }{Id: "id"}))
	require.Equal(t, "pk", getEntityID(&struct{ PK string }{PK: "pk"}))
}
