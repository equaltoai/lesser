package main

import "errors"

// Push delivery error constants
var (
	// Initialization errors
	ErrProcessorInitialization = errors.New("failed to initialize processor")
	ErrDynamoDBInit           = errors.New("failed to initialize DynamORM database")
	ErrRepositoryFactory      = errors.New("failed to create repository factory")

	// Message processing errors
	ErrInvalidMessageFormat = errors.New("invalid message format")

	// Storage/Database errors
	ErrGetPushSubscriptions = errors.New("failed to get push subscriptions")
	ErrGetVAPIDKeys        = errors.New("failed to get VAPID keys")

	// Push delivery errors
	ErrMarshalPayload  = errors.New("failed to marshal payload")
	ErrEncryptPayload  = errors.New("failed to encrypt payload")
	ErrCreateVAPIDJWT  = errors.New("failed to create VAPID JWT")
	ErrCreateRequest   = errors.New("failed to create request")
	ErrSendRequest     = errors.New("failed to send request")
	ErrPushServiceError = errors.New("push service returned error")

	// Cryptographic/Security errors
	ErrParseEndpoint          = errors.New("failed to parse endpoint")
	ErrDecodePrivateKey       = errors.New("failed to decode private key")
	ErrSign                   = errors.New("failed to sign")
	ErrDecodeP256dh           = errors.New("failed to decode p256dh")
	ErrDecodeAuth             = errors.New("failed to decode auth")
	ErrGenerateServerKey      = errors.New("failed to generate server key")
	ErrConvertToECDH          = errors.New("failed to convert server key to ECDH")
	ErrParseClientPublicKey   = errors.New("failed to parse client public key")
	ErrPerformECDH            = errors.New("failed to perform ECDH")
	ErrGenerateSalt           = errors.New("failed to generate salt")
	ErrGenerateNonce          = errors.New("failed to generate nonce")
	ErrCreateCipher           = errors.New("failed to create cipher")
	ErrCreateGCM              = errors.New("failed to create GCM")
)