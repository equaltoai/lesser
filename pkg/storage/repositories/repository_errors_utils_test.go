package repositories

import (
	"errors"
	"testing"

	repoerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

func TestErrorUtils_Coverage(t *testing.T) {
	utils := NewErrorUtils()
	require.NotNil(t, utils)

	// HandleNotFound
	err := utils.HandleNotFound(dynamormerrors.ErrItemNotFound, "entity", "id")
	require.Error(t, err)
	var appErr *repoerrors.AppError
	require.True(t, errors.As(err, &appErr))
	require.True(t, dynamormerrors.IsNotFound(err))
	require.ErrorIs(t, err, dynamormerrors.ErrItemNotFound)

	other := errors.New("x")
	require.Same(t, other, utils.HandleNotFound(other, "entity", "id"))

	// HandleGetError
	require.NoError(t, utils.HandleGetError(nil, "entity", "id"))
	err = utils.HandleGetError(storage.ErrNotFound, "entity", "id")
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrNotFound)
	require.True(t, repoerrors.HasCode(err, repoerrors.CodeNotFound))
	err = utils.HandleGetError(dynamormerrors.ErrItemNotFound, "entity", "id")
	require.Error(t, err)
	require.True(t, dynamormerrors.IsNotFound(err))
	require.ErrorIs(t, err, dynamormerrors.ErrItemNotFound)
	err = utils.HandleGetError(errors.New("boom"), "entity", "id")
	require.Error(t, err)

	// HandleCreateError
	require.NoError(t, utils.HandleCreateError(nil, "entity", "id"))
	err = utils.HandleCreateError(storage.ErrAlreadyExists, "entity", "id")
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrAlreadyExists)
	require.True(t, repoerrors.HasCode(err, repoerrors.CodeAlreadyExists))
	err = utils.HandleCreateError(dynamormerrors.ErrConditionFailed, "entity", "id")
	require.Error(t, err)
	require.True(t, dynamormerrors.IsConditionFailed(err))
	require.ErrorIs(t, err, dynamormerrors.ErrConditionFailed)
	err = utils.HandleCreateError(errors.New("boom"), "entity", "id")
	require.Error(t, err)

	// HandleUpdateError
	require.NoError(t, utils.HandleUpdateError(nil, "entity", "id"))
	err = utils.HandleUpdateError(dynamormerrors.ErrItemNotFound, "entity", "id")
	require.Error(t, err)
	require.True(t, dynamormerrors.IsNotFound(err))
	require.ErrorIs(t, err, dynamormerrors.ErrItemNotFound)
	err = utils.HandleUpdateError(errors.New("boom"), "entity", "id")
	require.Error(t, err)

	// HandleDeleteError
	require.NoError(t, utils.HandleDeleteError(nil, "entity", "id"))
	require.NoError(t, utils.HandleDeleteError(storage.ErrNotFound, "entity", "id"))
	require.NoError(t, utils.HandleDeleteError(dynamormerrors.ErrItemNotFound, "entity", "id"))
	err = utils.HandleDeleteError(errors.New("boom"), "entity", "id")
	require.Error(t, err)

	// HandleQueryError
	require.NoError(t, utils.HandleQueryError(nil, "entity", "q"))
	err = utils.HandleQueryError(errors.New("boom"), "entity", "q")
	require.Error(t, err)

	// Sentinel predicates
	require.True(t, utils.IsNotFound(dynamormerrors.ErrItemNotFound))
	require.True(t, utils.IsConditionalCheckFailed(dynamormerrors.ErrConditionFailed))
}

func TestRepositoryErrors_MoreConstructors_Coverage(t *testing.T) {
	require.NotNil(t, AccountValidationFailed("reason"))
	require.NotNil(t, DeviceValidationFailed("reason"))
	require.NotNil(t, SessionValidationFailed("reason"))
	require.NotNil(t, WebAuthnValidationFailed("reason"))
	require.NotNil(t, WalletValidationFailed("reason"))

	require.NotNil(t, DeviceNotFound("device-1"))
	require.NotNil(t, WebAuthnCredentialNotFound("cred-1"))

	require.NotNil(t, AccountSearchInvalidWebfingerFormat("bad-format"))
	require.NotNil(t, OAuthClientNameRequired())
	require.NotNil(t, OAuthRedirectURIsRequired())
	require.NotNil(t, OAuthNoUpdatesProvided())
	require.NotNil(t, OAuthClientAlreadyExists("client-1"))
	require.NotNil(t, OAuthStateExpired("state-1"))

	require.NotNil(t, QueryOperationFailed("op", errors.New("boom")))
	require.NotNil(t, QueryCollectionAddFailed("collection", errors.New("boom")))
	require.NotNil(t, QueryExecutionFailed("query", errors.New("boom")))
	require.NotNil(t, QueryValidationFailed("reason"))

	require.NotNil(t, InvalidHashtagTrendType("x"))
	require.NotNil(t, InvalidStatusTrendType("x"))
	require.NotNil(t, InvalidLinkTrendType("x"))
	require.NotNil(t, HashtagBatchUnknownModelType("x"))
	require.NotNil(t, StatusRepoDependencyMissing())
	require.NotNil(t, InvalidQueryParameters("x"))
}

func TestRepositoryErrors_MappingAndHelpers_Coverage(t *testing.T) {
	require.NoError(t, MapDynamoDBError(nil))
	require.ErrorIs(t, MapDynamoDBError(dynamormerrors.ErrItemNotFound), storage.ErrNotFound)
	require.ErrorIs(t, MapDynamoDBError(dynamormerrors.ErrConditionFailed), storage.ErrAlreadyExists)
	require.True(t, dynamormerrors.IsNotFound(MapDynamoDBError(dynamormerrors.ErrItemNotFound)))
	require.True(t, dynamormerrors.IsConditionFailed(MapDynamoDBError(dynamormerrors.ErrConditionFailed)))

	require.ErrorIs(t, MapDynamoDBError(errors.New("validation failed")), storage.ErrInvalidInput)
	require.ErrorIs(t, MapDynamoDBError(errors.New("unauthorized")), storage.ErrUnauthorized)

	// These return AppError values (not storage sentinels).
	require.Error(t, MapDynamoDBError(errors.New("ProvisionedThroughputExceededException")))
	require.Error(t, MapDynamoDBError(errors.New("Item size")))

	mapped := MapErrorWithContext(errors.New("validation failed"), "ctx")
	require.Error(t, mapped)

	require.True(t, containsAny("abc def", "zzz", "abc"))

	appErr := NewRepositoryError(repoerrors.CodeInternal, "msg")
	require.NotNil(t, appErr)
	appErr = NewRepositoryInternalError(repoerrors.CodeInternal, "msg", errors.New("boom"))
	require.NotNil(t, appErr)
	appErr = WrapRepositoryError(errors.New("boom"), repoerrors.CodeInternal, "msg")
	require.NotNil(t, appErr)

	require.True(t, IsRepositoryNotFoundError(repoerrors.ItemNotFound("x")))
	require.True(t, IsRepositoryConflictError(repoerrors.ItemAlreadyExists("x")))
}
