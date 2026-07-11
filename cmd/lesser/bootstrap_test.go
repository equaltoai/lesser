package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydb "github.com/theory-cloud/tabletheory/v2/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
)

func TestDetermineBootstrapWallet_ExistingAddress(t *testing.T) {
	wallet, err := determineBootstrapWallet("0xAbC")
	require.NoError(t, err)
	require.Equal(t, "0xabc", wallet.Address)
	require.Empty(t, wallet.Mnemonic)
	require.Equal(t, defaultBootstrapDerivationPath, wallet.DerivationPath)
	require.Equal(t, 1, wallet.ChainID)
}

func TestGenerateBootstrapWalletAndKeyMaterialRoundTrip(t *testing.T) {
	wallet, err := generateBootstrapWallet()
	require.NoError(t, err)
	require.NotEmpty(t, wallet.Address)
	require.NotEmpty(t, wallet.Mnemonic)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "bootstrap.json")

	require.Error(t, writeBootstrapKeyMaterial(path, bootstrapWallet{Address: wallet.Address}))
	require.NoError(t, writeBootstrapKeyMaterial(path, wallet))

	loaded, err := readBootstrapKeyMaterial(path)
	require.NoError(t, err)
	require.Equal(t, wallet.Address, loaded.Address)
	require.Equal(t, wallet.Mnemonic, loaded.Mnemonic)
	require.Equal(t, wallet.DerivationPath, loaded.DerivationPath)
	require.Equal(t, wallet.ChainID, loaded.ChainID)
}

func TestReadBootstrapKeyMaterial_RejectsInvalid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bootstrap.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"wallet":{"address":"","mnemonic":""}}`), 0o644))

	_, err := readBootstrapKeyMaterial(path)
	require.Error(t, err)
}

func TestStageMainTableName(t *testing.T) {
	name := stageMainTableName("app", naming.StageDev)
	require.NotEmpty(t, name)
	require.Contains(t, name, "app")
}

func TestTableNotFoundError(t *testing.T) {
	err := tableNotFoundError{TableName: "tbl"}
	require.Contains(t, err.Error(), "tbl")
}

func TestGetInstanceStateItem_ParsesAndHandlesNotFound(t *testing.T) {
	t.Run("table not found maps to typed error", func(t *testing.T) {
		ctx := context.Background()
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Return(theorydbErrors.ErrTableNotFound).Once()

		_, err := getInstanceStateItem(ctx, db, "tbl")
		require.Error(t, err)
		var tnf tableNotFoundError
		require.ErrorAs(t, err, &tnf)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("dynamodb ResourceNotFoundException maps to typed error", func(t *testing.T) {
		ctx := context.Background()
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Return(fakeSmithyAPIError{code: "ResourceNotFoundException"}).Once()

		_, err := getInstanceStateItem(ctx, db, "tbl")
		require.Error(t, err)
		var tnf tableNotFoundError
		require.ErrorAs(t, err, &tnf)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("dynamodb ResourceNotFoundException message maps to typed error", func(t *testing.T) {
		ctx := context.Background()
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Return(errors.New("operation error DynamoDB: GetItem, ResourceNotFoundException: Requested resource not found")).Once()

		_, err := getInstanceStateItem(ctx, db, "tbl")
		require.Error(t, err)
		var tnf tableNotFoundError
		require.ErrorAs(t, err, &tnf)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("missing item locks stage", func(t *testing.T) {
		ctx := context.Background()
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

		item, err := getInstanceStateItem(ctx, db, "tbl")
		require.NoError(t, err)
		require.False(t, item.Exists)
		require.True(t, item.Locked)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("parses address and unlocked", func(t *testing.T) {
		ctx := context.Background()
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*bootstrapInstanceStateRecord)
			dest.Locked = false
			dest.BootstrapWalletAddress = "0xAbC"
		}).Return(nil).Once()

		item, err := getInstanceStateItem(ctx, db, "tbl")
		require.NoError(t, err)
		require.True(t, item.Exists)
		require.False(t, item.Locked)
		require.Equal(t, "0xabc", item.BootstrapWalletAddress)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})
}

func TestEnsureStageBootstrapState_HandlesExistingAndUpsert(t *testing.T) {
	ctx := context.Background()
	app := "app"
	stage := naming.StageDev
	table := stageMainTableName(app, stage)

	t.Run("returns existing unlocked", func(t *testing.T) {
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		db.On("Close").Return(nil).Once()

		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*bootstrapInstanceStateRecord)
			dest.Locked = false
			dest.BootstrapWalletAddress = "0xAbC"
		}).Return(nil).Once()

		newDB := func() (theorydb.DB, error) { return db, nil }
		state, err := ensureStageBootstrapState(ctx, newDB, app, stage, "")
		require.NoError(t, err)
		require.False(t, state.Locked)
		require.Equal(t, "0xabc", state.Address)
		require.False(t, state.Updated)

		require.Equal(t, table, bootstrapTableName)
		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("locked stage refuses overwrite", func(t *testing.T) {
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		db.On("Close").Return(nil).Once()

		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*bootstrapInstanceStateRecord)
			dest.Locked = true
			dest.BootstrapWalletAddress = "0xAbC"
		}).Return(nil).Once()

		newDB := func() (theorydb.DB, error) { return db, nil }
		_, err := ensureStageBootstrapState(ctx, newDB, app, stage, "0xdef")
		require.Error(t, err)
		require.Contains(t, err.Error(), "refusing to overwrite")

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("requires desired address when missing", func(t *testing.T) {
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		db.On("Close").Return(nil).Once()

		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

		newDB := func() (theorydb.DB, error) { return db, nil }
		_, err := ensureStageBootstrapState(ctx, newDB, app, stage, "   ")
		require.Error(t, err)
		require.Contains(t, err.Error(), "bootstrap address is empty")

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("upserts when not configured", func(t *testing.T) {
		db := new(mocks.MockDB)
		readQuery := new(mocks.MockQuery)
		writeQuery := new(mocks.MockQuery)
		builder := new(mocks.MockUpdateBuilder)

		db.On("WithContext", ctx).Return(db).Twice()
		db.On("Model", mock.Anything).Return(readQuery).Once()
		db.On("Model", mock.Anything).Return(writeQuery).Once()
		db.On("Close").Return(nil).Once()

		readQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(readQuery).Once()
		readQuery.On("Where", "SK", "=", "STATE").Return(readQuery).Once()
		readQuery.On("ConsistentRead").Return(readQuery).Once()
		readQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

		writeQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(writeQuery).Once()
		writeQuery.On("Where", "SK", "=", "STATE").Return(writeQuery).Once()
		writeQuery.On("UpdateBuilder").Return(builder).Once()

		builder.On("Set", "Locked", true).Return(builder).Once()
		builder.On("Set", "BootstrapUsername", "bootstrap").Return(builder).Once()
		builder.On("Set", "BootstrapWalletAddress", "0xabc").Return(builder).Once()
		builder.On("Set", "UpdatedAt", mock.Anything).Return(builder).Once()
		builder.On("SetIfNotExists", "CreatedAt", mock.Anything, mock.Anything).Return(builder).Once()
		builder.On("Remove", "ActivatedAt").Return(builder).Once()
		builder.On("Remove", "PrimaryAdminUsername").Return(builder).Once()
		builder.On("Execute").Return(nil).Once()

		newDB := func() (theorydb.DB, error) { return db, nil }
		state, err := ensureStageBootstrapState(ctx, newDB, app, stage, "0xAbC")
		require.NoError(t, err)
		require.True(t, state.Locked)
		require.True(t, state.Updated)
		require.Equal(t, "0xabc", state.Address)

		db.AssertExpectations(t)
		readQuery.AssertExpectations(t)
		writeQuery.AssertExpectations(t)
		builder.AssertExpectations(t)
	})

	t.Run("upsert failure surfaces error", func(t *testing.T) {
		db := new(mocks.MockDB)
		readQuery := new(mocks.MockQuery)
		writeQuery := new(mocks.MockQuery)
		builder := new(mocks.MockUpdateBuilder)

		db.On("WithContext", ctx).Return(db).Twice()
		db.On("Model", mock.Anything).Return(readQuery).Once()
		db.On("Model", mock.Anything).Return(writeQuery).Once()
		db.On("Close").Return(nil).Once()

		readQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(readQuery).Once()
		readQuery.On("Where", "SK", "=", "STATE").Return(readQuery).Once()
		readQuery.On("ConsistentRead").Return(readQuery).Once()
		readQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

		writeQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(writeQuery).Once()
		writeQuery.On("Where", "SK", "=", "STATE").Return(writeQuery).Once()
		writeQuery.On("UpdateBuilder").Return(builder).Once()

		builder.On("Set", mock.Anything, mock.Anything).Return(builder).Maybe()
		builder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(builder).Maybe()
		builder.On("Remove", mock.Anything).Return(builder).Maybe()
		builder.On("Execute").Return(errors.New("update failed")).Once()

		newDB := func() (theorydb.DB, error) { return db, nil }
		_, err := ensureStageBootstrapState(ctx, newDB, app, stage, "0xabc")
		require.Error(t, err)
		require.Contains(t, err.Error(), "update instance state")
	})
}

func TestInspectBootstrapRequirements_CombinesStages(t *testing.T) {
	ctx := context.Background()
	app := "app"

	t.Run("marks required when table missing", func(t *testing.T) {
		devDB := new(mocks.MockDB)
		devQuery := new(mocks.MockQuery)
		liveDB := new(mocks.MockDB)
		liveQuery := new(mocks.MockQuery)

		devDB.On("WithContext", ctx).Return(devDB).Once()
		devDB.On("Model", mock.Anything).Return(devQuery).Once()
		devDB.On("Close").Return(nil).Once()
		devQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(devQuery).Once()
		devQuery.On("Where", "SK", "=", "STATE").Return(devQuery).Once()
		devQuery.On("ConsistentRead").Return(devQuery).Once()
		devQuery.On("First", mock.Anything).Return(theorydbErrors.ErrTableNotFound).Once()

		liveDB.On("WithContext", ctx).Return(liveDB).Once()
		liveDB.On("Model", mock.Anything).Return(liveQuery).Once()
		liveDB.On("Close").Return(nil).Once()
		liveQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(liveQuery).Once()
		liveQuery.On("Where", "SK", "=", "STATE").Return(liveQuery).Once()
		liveQuery.On("ConsistentRead").Return(liveQuery).Once()
		liveQuery.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

		calls := 0
		newDB := func() (theorydb.DB, error) {
			calls++
			if calls == 1 {
				return devDB, nil
			}
			return liveDB, nil
		}

		addr, required, err := inspectBootstrapRequirements(ctx, newDB, app, []naming.Stage{naming.StageDev, naming.StageLive})
		require.NoError(t, err)
		require.True(t, required)
		require.Empty(t, addr)
	})

	t.Run("returns address and still requires when any stage locked without address", func(t *testing.T) {
		devDB := new(mocks.MockDB)
		devQuery := new(mocks.MockQuery)
		liveDB := new(mocks.MockDB)
		liveQuery := new(mocks.MockQuery)

		devDB.On("WithContext", ctx).Return(devDB).Once()
		devDB.On("Model", mock.Anything).Return(devQuery).Once()
		devDB.On("Close").Return(nil).Once()
		devQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(devQuery).Once()
		devQuery.On("Where", "SK", "=", "STATE").Return(devQuery).Once()
		devQuery.On("ConsistentRead").Return(devQuery).Once()
		devQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*bootstrapInstanceStateRecord)
			dest.Locked = true
			dest.BootstrapWalletAddress = ""
		}).Return(nil).Once()

		liveDB.On("WithContext", ctx).Return(liveDB).Once()
		liveDB.On("Model", mock.Anything).Return(liveQuery).Once()
		liveDB.On("Close").Return(nil).Once()
		liveQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(liveQuery).Once()
		liveQuery.On("Where", "SK", "=", "STATE").Return(liveQuery).Once()
		liveQuery.On("ConsistentRead").Return(liveQuery).Once()
		liveQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*bootstrapInstanceStateRecord)
			dest.Locked = true
			dest.BootstrapWalletAddress = "0xAbC"
		}).Return(nil).Once()

		calls := 0
		newDB := func() (theorydb.DB, error) {
			calls++
			if calls == 1 {
				return devDB, nil
			}
			return liveDB, nil
		}

		addr, required, err := inspectBootstrapRequirements(ctx, newDB, app, []naming.Stage{naming.StageDev, naming.StageLive})
		require.NoError(t, err)
		require.True(t, required)
		require.Equal(t, "0xabc", addr)
	})

	t.Run("errors when multiple addresses", func(t *testing.T) {
		devDB := new(mocks.MockDB)
		devQuery := new(mocks.MockQuery)
		liveDB := new(mocks.MockDB)
		liveQuery := new(mocks.MockQuery)

		devDB.On("WithContext", ctx).Return(devDB).Once()
		devDB.On("Model", mock.Anything).Return(devQuery).Once()
		devDB.On("Close").Return(nil).Once()
		devQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(devQuery).Once()
		devQuery.On("Where", "SK", "=", "STATE").Return(devQuery).Once()
		devQuery.On("ConsistentRead").Return(devQuery).Once()
		devQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*bootstrapInstanceStateRecord)
			dest.Locked = true
			dest.BootstrapWalletAddress = "0x1"
		}).Return(nil).Once()

		liveDB.On("WithContext", ctx).Return(liveDB).Once()
		liveDB.On("Model", mock.Anything).Return(liveQuery).Once()
		liveDB.On("Close").Return(nil).Once()
		liveQuery.On("Where", "PK", "=", instanceConfigKeyPK).Return(liveQuery).Once()
		liveQuery.On("Where", "SK", "=", "STATE").Return(liveQuery).Once()
		liveQuery.On("ConsistentRead").Return(liveQuery).Once()
		liveQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*bootstrapInstanceStateRecord)
			dest.Locked = true
			dest.BootstrapWalletAddress = "0x2"
		}).Return(nil).Once()

		calls := 0
		newDB := func() (theorydb.DB, error) {
			calls++
			if calls == 1 {
				return devDB, nil
			}
			return liveDB, nil
		}

		_, _, err := inspectBootstrapRequirements(ctx, newDB, app, []naming.Stage{naming.StageDev, naming.StageLive})
		require.Error(t, err)
		require.Contains(t, err.Error(), "multiple bootstrap wallet addresses")
	})

	t.Run("returns error for unexpected query failure", func(t *testing.T) {
		db := new(mocks.MockDB)
		q := new(mocks.MockQuery)

		db.On("WithContext", ctx).Return(db).Once()
		db.On("Model", mock.Anything).Return(q).Once()
		db.On("Close").Return(nil).Once()
		q.On("Where", "PK", "=", instanceConfigKeyPK).Return(q).Once()
		q.On("Where", "SK", "=", "STATE").Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", mock.Anything).Return(errors.New("boom")).Once()

		newDB := func() (theorydb.DB, error) { return db, nil }
		_, _, err := inspectBootstrapRequirements(ctx, newDB, app, []naming.Stage{naming.StageDev})
		require.Error(t, err)
	})
}
