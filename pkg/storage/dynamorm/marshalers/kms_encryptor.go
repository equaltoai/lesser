package marshalers

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KMSEncryptor implements encryption using AWS KMS
type KMSEncryptor struct {
	client *kms.Client
	keyID  string
}

// NewKMSEncryptor creates a new KMS encryptor
func NewKMSEncryptor(keyID string) (*KMSEncryptor, error) {
	if keyID == "" {
		return nil, fmt.Errorf("KMS key ID is required")
	}

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &KMSEncryptor{
		client: kms.NewFromConfig(cfg),
		keyID:  keyID,
	}, nil
}

// Encrypt encrypts plaintext using KMS
func (e *KMSEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	result, err := e.client.Encrypt(context.Background(), &kms.EncryptInput{
		KeyId:     aws.String(e.keyID),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, fmt.Errorf("KMS encryption failed: %w", err)
	}

	return result.CiphertextBlob, nil
}

// Decrypt decrypts ciphertext using KMS
func (e *KMSEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	result, err := e.client.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: ciphertext,
		KeyId:          aws.String(e.keyID), // Optional but good for validation
	})
	if err != nil {
		return nil, fmt.Errorf("KMS decryption failed: %w", err)
	}

	return result.Plaintext, nil
}

// EncryptToBase64 encrypts and returns base64-encoded ciphertext
func (e *KMSEncryptor) EncryptToBase64(plaintext []byte) (string, error) {
	ciphertext, err := e.Encrypt(plaintext)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptFromBase64 decrypts from base64-encoded ciphertext
func (e *KMSEncryptor) DecryptFromBase64(encoded string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}
	return e.Decrypt(ciphertext)
}

