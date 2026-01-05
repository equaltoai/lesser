package lift

import (
	stdErrors "errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

type fakeSmithyAPIError struct {
	code    string
	message string
}

func (f fakeSmithyAPIError) Error() string                 { return f.code }
func (f fakeSmithyAPIError) ErrorCode() string             { return f.code }
func (f fakeSmithyAPIError) ErrorMessage() string          { return f.message }
func (f fakeSmithyAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func TestMapCommonError_MapsAppErrorStatusCodes(t *testing.T) {
	err := pkgerrors.NewAppError(pkgerrors.CodeValidationFailed, pkgerrors.CategoryValidation, "bad")
	mapped := MapCommonError(err)
	liftErr, ok := mapped.(*liftPkg.LiftError)
	require.True(t, ok)
	require.Equal(t, 422, liftErr.StatusCode)
}

func TestMapStorageError_HandlesLegacyAndDynamoDBErrors(t *testing.T) {
	require.Nil(t, MapStorageError(nil))

	liftErr := MapStorageError(storage.ErrNotFound).(*liftPkg.LiftError)
	require.Equal(t, 404, liftErr.StatusCode)

	liftErr = MapStorageError(storage.ErrAlreadyExists).(*liftPkg.LiftError)
	require.Equal(t, 409, liftErr.StatusCode)

	liftErr = MapStorageError(storage.ErrInvalidInput).(*liftPkg.LiftError)
	require.Equal(t, 422, liftErr.StatusCode)

	liftErr = MapStorageError(&types.ResourceNotFoundException{}).(*liftPkg.LiftError)
	require.Equal(t, 404, liftErr.StatusCode)

	liftErr = MapStorageError(&types.ConditionalCheckFailedException{}).(*liftPkg.LiftError)
	require.Equal(t, 409, liftErr.StatusCode)

	liftErr = MapStorageError(&types.RequestLimitExceeded{}).(*liftPkg.LiftError)
	require.Equal(t, 429, liftErr.StatusCode)

	liftErr = MapStorageError(&types.ProvisionedThroughputExceededException{}).(*liftPkg.LiftError)
	require.Equal(t, 429, liftErr.StatusCode)
}

func TestMapAWSError_MapsSmithyAPIErrorCodes(t *testing.T) {
	liftErr := MapAWSError(fakeSmithyAPIError{code: "AccessDeniedException", message: "nope"}).(*liftPkg.LiftError)
	require.Equal(t, 403, liftErr.StatusCode)

	liftErr = MapAWSError(fakeSmithyAPIError{code: "ValidationException", message: "bad"}).(*liftPkg.LiftError)
	require.Equal(t, 422, liftErr.StatusCode)
}

func TestMapAWSError_MapsMessagePatterns(t *testing.T) {
	liftErr := MapAWSError(stdErrors.New("NoSuchKey")).(*liftPkg.LiftError)
	require.Equal(t, 404, liftErr.StatusCode)

	liftErr = MapAWSError(stdErrors.New("RequestTimeout")).(*liftPkg.LiftError)
	require.Equal(t, 504, liftErr.StatusCode)
}

func TestErrorContextHelpers(t *testing.T) {
	ctx := createTestContext()
	ctx.Logger = &liftPkg.NoOpLogger{}
	ctx.RequestID = "req-1"

	require.NoError(t, LogAndReturnError(ctx, nil, "msg", nil))

	liftErr := liftPkg.NewLiftError("X", "x", 400)
	require.Same(t, liftErr, LogAndReturnError(ctx, liftErr, "msg", nil))

	mapped := LogAndReturnError(ctx, stdErrors.New("boom"), "msg", nil).(*liftPkg.LiftError)
	require.Equal(t, 500, mapped.StatusCode)

	wrappedDB := WrapDatabaseError(ctx, storage.ErrNotFound, "get", "user").(*liftPkg.LiftError)
	require.Equal(t, 404, wrappedDB.StatusCode)
	require.Equal(t, "get", wrappedDB.Details["operation"])

	wrappedExternal := WrapExternalServiceError(ctx, stdErrors.New("boom"), "svc", "op").(*liftPkg.LiftError)
	require.Equal(t, 503, wrappedExternal.StatusCode)
	require.Equal(t, "op", wrappedExternal.Details["operation"])

	notFound := ResourceNotFound(ctx, "thing", "1").(*liftPkg.LiftError)
	require.Equal(t, 404, notFound.StatusCode)
	require.Equal(t, "1", notFound.Details["id"])

	validation := ValidationFailed(ctx, map[string]string{"field": "bad"}).(*liftPkg.LiftError)
	require.Equal(t, 422, validation.StatusCode)
	require.Equal(t, "field", validation.Details["field"])
	require.Equal(t, "bad", validation.Details["validation.field"])

	denied := AccessDenied(ctx, "read", "thing", "user").(*liftPkg.LiftError)
	require.Equal(t, 403, denied.StatusCode)
}
