// Package main provides error constants for the init-deploy Lambda function.
package main

import "errors"

// Error constants for init-deploy operations
var (
	// AWS Secrets Manager errors
	ErrFailedToCreateOrUpdateSecret = errors.New("failed to create or update secret")
	
	// VAPID key generation errors  
	ErrFailedToGeneratePrivateKey = errors.New("failed to generate private key")
	ErrFailedToMarshalPrivateKey  = errors.New("failed to marshal private key")
	ErrFailedToConvertToECDHKey   = errors.New("failed to convert to ECDH key")
)