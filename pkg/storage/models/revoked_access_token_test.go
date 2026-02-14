package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRevokedAccessToken_TableName(t *testing.T) {
	model := RevokedAccessToken{}
	assert.Equal(t, MainTableName, model.TableName())
}

func TestRevokedAccessToken_BeforeCreate_SetsKeysRevokedAtAndTTL(t *testing.T) {
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	model := &RevokedAccessToken{
		JTI:       "jti-1",
		ExpiresAt: expiresAt,
	}

	err := model.BeforeCreate()
	assert.NoError(t, err)
	assert.Equal(t, "REVOKEDTOKEN#jti-1", model.PK)
	assert.Equal(t, SKToken, model.SK)
	assert.False(t, model.RevokedAt.IsZero())
	assert.Equal(t, expiresAt.Unix(), model.TTL)
}

func TestRevokedAccessToken_BeforeCreate_PreservesRevokedAtAndTTL(t *testing.T) {
	revokedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	model := &RevokedAccessToken{
		JTI:       "jti-2",
		ExpiresAt: expiresAt,
		RevokedAt: revokedAt,
		TTL:       123,
	}

	err := model.BeforeCreate()
	assert.NoError(t, err)
	assert.Equal(t, revokedAt, model.RevokedAt)
	assert.Equal(t, int64(123), model.TTL)
}

func TestRevokedAccessToken_UpdateKeysAndGetters(t *testing.T) {
	model := &RevokedAccessToken{JTI: "jti-3"}

	err := model.UpdateKeys()
	assert.NoError(t, err)
	assert.Equal(t, "REVOKEDTOKEN#jti-3", model.GetPK())
	assert.Equal(t, SKToken, model.GetSK())
}
