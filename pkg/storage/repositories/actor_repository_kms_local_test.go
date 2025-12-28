package repositories

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestActorRepository_GetActorPrivateKey_decrypt_error_local_kms_endpoint(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"__type":"InternalFailure","message":"boom"}`))
	}))
	defer srv.Close()

	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_ENDPOINT_URL_KMS", srv.URL)
	t.Setenv("AWS_MAX_ATTEMPTS", "1")

	cfg := lesserconfig.Get()
	prevKey := cfg.KMSKeyID
	cfg.KMSKeyID = "alias/test"
	defer func() { cfg.KMSKeyID = prevKey }()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Select", mock.Anything).Return(mockQuery)

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		m := args.Get(0).(*models.Actor)
		m.PrivateKey = base64.StdEncoding.EncodeToString([]byte("ciphertext"))
	}).Once()

	repo := NewActorRepository(mockDB, "test-table", logger)
	_, err := repo.GetActorPrivateKey(ctx, "alice")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt private key")
}

