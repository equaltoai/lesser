package main

import "github.com/equaltoai/lesser/pkg/errors"

// Push delivery error functions

// Initialization errors

// ErrProcessorInitialization returns an error indicating processor initialization failed.
func ErrProcessorInitialization(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to initialize processor", err)
}

// ErrDynamoDBInit returns an error indicating DynamORM database initialization failed.
func ErrDynamoDBInit(err error) error {
	return errors.NewStorageInternalError(errors.CodeDatabaseConnection, "failed to initialize DynamORM database", err)
}

// ErrRepositoryFactory returns an error indicating repository factory creation failed.
func ErrRepositoryFactory(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to create repository factory", err)
}

// Message processing errors

// ErrInvalidMessageFormat returns an error indicating invalid message format.
func ErrInvalidMessageFormat(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "invalid message format")
}

// Storage/Database errors

// ErrGetPushSubscriptions returns an error indicating failed to get push subscriptions.
func ErrGetPushSubscriptions(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get push subscriptions", err)
}

// ErrGetVAPIDKeys returns an error indicating failed to get VAPID keys.
func ErrGetVAPIDKeys(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get VAPID keys", err)
}

// Push delivery errors

// ErrMarshalPayload returns an error indicating failed to marshal payload.
func ErrMarshalPayload(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to marshal payload", err)
}

// ErrEncryptPayload returns an error indicating failed to encrypt payload.
func ErrEncryptPayload(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to encrypt payload", err)
}

// ErrCreateVAPIDJWT returns an error indicating failed to create VAPID JWT.
func ErrCreateVAPIDJWT(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to create VAPID JWT", err)
}

// ErrCreateRequest returns an error indicating failed to create request.
func ErrCreateRequest(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to create request", err)
}

// ErrSendRequest returns an error indicating failed to send request.
func ErrSendRequest(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to send request", err)
}

// ErrPushServiceError returns an error indicating push service returned error.
func ErrPushServiceError() error {
	return errors.NewLambdaError(errors.CodeInternal, "push service returned error")
}

// Cryptographic/Security errors

// ErrParseEndpoint returns an error indicating failed to parse endpoint.
func ErrParseEndpoint(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "failed to parse endpoint")
}

// ErrDecodePrivateKey returns an error indicating failed to decode private key.
func ErrDecodePrivateKey(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to decode private key", err)
}

// ErrSign returns an error indicating failed to sign.
func ErrSign(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to sign", err)
}

// ErrDecodeP256dh returns an error indicating failed to decode p256dh.
func ErrDecodeP256dh(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "failed to decode p256dh")
}

// ErrDecodeAuth returns an error indicating failed to decode auth.
func ErrDecodeAuth(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "failed to decode auth")
}

// ErrGenerateServerKey returns an error indicating failed to generate server key.
func ErrGenerateServerKey(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to generate server key", err)
}

// ErrConvertToECDH returns an error indicating failed to convert server key to ECDH.
func ErrConvertToECDH(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to convert server key to ECDH", err)
}

// ErrParseClientPublicKey returns an error indicating failed to parse client public key.
func ErrParseClientPublicKey(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "failed to parse client public key")
}

// ErrPerformECDH returns an error indicating failed to perform ECDH.
func ErrPerformECDH(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to perform ECDH", err)
}

// ErrGenerateSalt returns an error indicating failed to generate salt.
func ErrGenerateSalt(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to generate salt", err)
}

// ErrGenerateNonce returns an error indicating failed to generate nonce.
func ErrGenerateNonce(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to generate nonce", err)
}

// ErrCreateCipher returns an error indicating failed to create cipher.
func ErrCreateCipher(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to create cipher", err)
}

// ErrCreateGCM returns an error indicating failed to create GCM.
func ErrCreateGCM(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to create GCM", err)
}
