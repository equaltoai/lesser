package lift

import "errors"

// Error constants for account operations
var (
	ErrInvalidActorIDFormat           = errors.New("invalid actor ID format")
	ErrUnableToParseRequestBody       = errors.New("unable to parse request body")
	ErrMissingBoundaryInContentType   = errors.New("missing boundary in content type")
	ErrInvalidContentType             = errors.New("invalid content type")
	ErrUnsupportedCollectionType      = errors.New("unsupported collection type")
)

// Error constants for helper operations
var (
	ErrInvalidAccountURL              = errors.New("invalid account URL")
	ErrRemoteAccountsNotSupported     = errors.New("remote accounts not yet supported")
	ErrHelperUnauthorized             = errors.New("unauthorized")
	ErrHelperInsufficientScope        = errors.New("insufficient scope")
	ErrInvalidAccountID               = errors.New("invalid account ID")
	ErrFailedToInitializeAuthService  = errors.New("failed to initialize auth service")
)

// Error constants for import operations
var (
	ErrInvalidImportType    = errors.New("invalid import type")
	ErrInvalidImportMode    = errors.New("mode must be 'merge' or 'overwrite'")
	ErrUnsupportedFileFormat = errors.New("unsupported file format")
	ErrJobQueueServiceCreationFailed = errors.New("failed to create job queue service")
)

// Error constants for app operations
var (
	ErrUnableToParseRequestBodyAsFormOrJSON = errors.New("unable to parse request body as form data or JSON")
	ErrInvalidRedirectURIFormat             = errors.New("invalid redirect_uri format")
	ErrFailedToParseMultipartForm           = errors.New("failed to parse multipart form")
	ErrFailedToParseFormData                = errors.New("failed to parse form data")
)

// Error constants for media operations
var (
	ErrEmptyRequestBody         = errors.New("empty request body")
	ErrNoFileDataFoundInRequest = errors.New("no file data found in request")
	ErrFailedToExtractBoundary  = errors.New("failed to extract boundary")
)

// Error constants for search operations
var (
	ErrSearchFailed                     = errors.New("search failed")
	ErrPrivacyAwareSearchFailed         = errors.New("privacy-aware search failed")
	ErrStatusSearchFailed               = errors.New("status search failed")
	ErrPrivacyAwareStatusSearchFailed   = errors.New("privacy-aware status search failed")
)

// Error constants for OAuth operations
var (
	ErrFailedToGenerateTokens    = errors.New("failed to generate tokens")
	ErrFailedToValidateRefreshToken = errors.New("failed to validate refresh token")
	ErrFailedToGenerateNewTokens = errors.New("failed to generate new tokens")
	ErrFailedToStoreNewRefreshToken = errors.New("failed to store new refresh token")
	ErrFailedToCreateOAuthSession = errors.New("failed to create OAuth session")
	ErrFailedToCreateUserSession = errors.New("failed to create user session")
)

// Error constants for VAPID key operations
var (
	ErrFailedToGenerateVAPIDPrivateKey = errors.New("failed to generate VAPID private key")
	ErrFailedToConvertToECDHKey        = errors.New("failed to convert to ECDH key")
	ErrFailedToStoreVAPIDKeys          = errors.New("failed to store VAPID keys")
)

// Error constants for export operations
var (
	ErrFailedToCreateJobQueueService = errors.New("failed to create job queue service")
	ErrInvalidStartDate              = errors.New("invalid start date")
	ErrInvalidEndDate                = errors.New("invalid end date")
)

// Error constants for follow request operations
var (
	ErrFailedToGetFollowerActor = errors.New("failed to get follower actor")
	ErrFailedToGetFollowedActor = errors.New("failed to get followed actor")
)

// Error constants for translation operations
var (
	ErrInvalidSourceLanguageCode = errors.New("invalid source language code")
	ErrInvalidTargetLanguageCode = errors.New("invalid target language code")
)

// Error constants for WebSocket cost analytics operations
var (
	ErrFailedToGetCostRecords = errors.New("failed to get cost records")
)

// Error constants for status delivery operations
var (
	ErrFailedToDetermineDeliveryRecipients = errors.New("failed to determine delivery recipients")
)

// Error constants for status conversion operations
var (
	ErrFailedToConvertStatus = errors.New("failed to convert status")
)