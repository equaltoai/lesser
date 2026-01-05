package auth

import "github.com/equaltoai/lesser/pkg/common"

// OAuthClientSecretHashPrefix marks hashed secrets stored for OAuth clients.
const OAuthClientSecretHashPrefix = common.OAuthClientSecretHashPrefix

// HashOAuthClientSecret hashes an OAuth client secret for storage.
func HashOAuthClientSecret(secret string) (string, error) {
	return common.HashOAuthClientSecret(secret)
}

// VerifyOAuthClientSecret verifies a provided OAuth client secret against a stored value.
func VerifyOAuthClientSecret(providedSecret, storedValue string) (bool, bool, error) {
	return common.VerifyOAuthClientSecret(providedSecret, storedValue)
}
