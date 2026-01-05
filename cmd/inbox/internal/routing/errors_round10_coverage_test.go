package routing

import (
	"testing"

	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestInboxErrors_Round10_Coverage(t *testing.T) {
	t.Run("constructors return AppError", func(t *testing.T) {
		constructors := []func() *pkgerrors.AppError{
			dynamoDBInterfaceError,
			dynamORMInitError,
			repositoryFactoryInitError,
			dmToAddressingError,
			dmCcAddressingError,
			dmBtoAddressingError,
			dmBccAddressingError,
			dmPublicAddressError,
			dmRecipientURLError,
			dmNoRecipientsError,
			requestConversionError,
			createRequestError,
			fetchActorError,
			actorResponseError,
			parseActorError,
			noPublicKeyError,
			parsePublicKeyError,
			marshalFollowError,
			parseFollowError,
			invalidNoteError,
			createLikeError,
			createAnnounceError,
			createBlockError,
			deleteBlockError,
			unauthorizedBlockUndoError,
			addNoTargetError,
			addNoObjectError,
			addObjectNoIDError,
			invalidCollectionTargetError,
			unauthorizedAddError,
			addItemFailedError,
			removeNoTargetError,
			removeNoObjectError,
			removeObjectNoIDError,
			unauthorizedRemoveError,
			removeItemFailedError,
			targetURLEmptyError,
			targetURLFormatError,
			invalidFlagError,
			flagNoObjectsError,
			storeModerationFlagError,
			moveNoTargetError,
			moveAuthorizationError,
			storeMigrationError,
			unsupportedFlagObjectError,
			extractUsernameError,
			verifyMoveAuthError,
			moveNotAuthorizedError,
			determineCollectionOwnerError,
			extractCollectionOwnerError,
			unauthorizedCollectionError,
			unauthorizedCollectionModifyError,
			activityBlockedError,
			determineObjectOwnerError,
			unauthorizedUpdateError,
			unauthorizedDeleteError,
			serializeObjectError,
			deserializeObjectError,
			createUpdateHistoryError,
			unsupportedDeleteObjectError,
			getObjectLikesError,
			getRepliesError,
			createTombstoneError,
			updateTrusteeConfirmationError,
		}

		for _, build := range constructors {
			err := build()
			require.NotNil(t, err)
			require.NotEmpty(t, string(err.Code))
			require.NotEmpty(t, string(err.Category))
			require.NotEmpty(t, err.Message)
		}
	})
}
