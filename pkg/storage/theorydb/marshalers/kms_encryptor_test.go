package marshalers

import (
	"context"
	"encoding/base64"
	stdErrors "errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKMS struct {
	encryptInput *kms.EncryptInput
	decryptInput *kms.DecryptInput

	encryptOut *kms.EncryptOutput
	decryptOut *kms.DecryptOutput

	encryptErr error
	decryptErr error
}

func (f *fakeKMS) Encrypt(_ context.Context, params *kms.EncryptInput, _ ...func(*kms.Options)) (*kms.EncryptOutput, error) {
	f.encryptInput = params
	return f.encryptOut, f.encryptErr
}

func (f *fakeKMS) Decrypt(_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	f.decryptInput = params
	return f.decryptOut, f.decryptErr
}

func TestNewKMSEncryptor_KeyIDRequired(t *testing.T) {
	enc, err := NewKMSEncryptor("")
	require.Error(t, err)
	require.Nil(t, enc)
}

func TestKMSEncryptor_EncryptAndDecrypt(t *testing.T) {
	kmsClient := &fakeKMS{
		encryptOut: &kms.EncryptOutput{CiphertextBlob: []byte{0xde, 0xad, 0xbe, 0xef}},
		decryptOut: &kms.DecryptOutput{Plaintext: []byte("plaintext")},
	}

	enc := &KMSEncryptor{
		client: kmsClient,
		keyID:  "test-key",
	}

	ciphertext, err := enc.Encrypt([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, ciphertext)
	require.NotNil(t, kmsClient.encryptInput)
	assert.Equal(t, "test-key", aws.ToString(kmsClient.encryptInput.KeyId))
	assert.Equal(t, []byte("hello"), kmsClient.encryptInput.Plaintext)

	plaintext, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, []byte("plaintext"), plaintext)
	require.NotNil(t, kmsClient.decryptInput)
	assert.Equal(t, "test-key", aws.ToString(kmsClient.decryptInput.KeyId))
	assert.Equal(t, ciphertext, kmsClient.decryptInput.CiphertextBlob)
}

func TestKMSEncryptor_EncryptErrorsPropagate(t *testing.T) {
	kmsClient := &fakeKMS{
		encryptErr: stdErrors.New("boom"),
	}

	enc := &KMSEncryptor{client: kmsClient, keyID: "test-key"}

	_, err := enc.Encrypt([]byte("hello"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KMS encryption failed")
}

func TestKMSEncryptor_DecryptErrorsPropagate(t *testing.T) {
	kmsClient := &fakeKMS{
		decryptErr: stdErrors.New("boom"),
	}

	enc := &KMSEncryptor{client: kmsClient, keyID: "test-key"}

	_, err := enc.Decrypt([]byte("cipher"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KMS decryption failed")
}

func TestKMSEncryptor_Base64Helpers(t *testing.T) {
	kmsClient := &fakeKMS{
		encryptOut: &kms.EncryptOutput{CiphertextBlob: []byte{0x01, 0x02, 0x03}},
		decryptOut: &kms.DecryptOutput{Plaintext: []byte("ok")},
	}

	enc := &KMSEncryptor{client: kmsClient, keyID: "test-key"}

	encoded, err := enc.EncryptToBase64([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03}), encoded)

	plaintext, err := enc.DecryptFromBase64(encoded)
	require.NoError(t, err)
	assert.Equal(t, []byte("ok"), plaintext)
}

func TestKMSEncryptor_DecryptFromBase64_Invalid(t *testing.T) {
	enc := &KMSEncryptor{client: &fakeKMS{}, keyID: "test-key"}

	_, err := enc.DecryptFromBase64("%%%")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base64")
}
