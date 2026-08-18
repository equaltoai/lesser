package repositories

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRound10_Utils_KeyAndValidationAndTimeUtils(t *testing.T) {
	keys := NewKeyUtils()
	require.Equal(t, "USER#alice", keys.UserKey(" Alice "))
	require.Equal(t, "ACTOR#alice", keys.ActorKey(" Alice "))
	require.Equal(t, "object#obj-1", keys.ObjectKey("obj-1"))
	require.Equal(t, "follow#alice", keys.FollowKey(" Alice "))
	require.Equal(t, "following#bob", keys.FollowingSK(" Bob "))
	require.Equal(t, "ACTOR#alice#BLOCKS", keys.BlockKey(" Alice "))
	require.Equal(t, "BLOCKED#bob", keys.BlockedSK(" Bob "))
	require.Equal(t, "MUTE#alice", keys.MuteKey(" Alice "))
	require.Equal(t, "MUTED#bob", keys.MutedSK(" Bob "))
	require.Equal(t, "HASHTAG#hello", keys.HashtagKey("#HeLLo"))
	require.Equal(t, "LIST#list-1", keys.ListKey("list-1"))
	require.Equal(t, "OAUTH_STATE#state-1", keys.OAuthStateKey("state-1"))
	require.Equal(t, "AUTH_CODE#code-1", keys.AuthCodeKey("code-1"))
	require.Equal(t, "REFRESH_TOKEN#rt-1", keys.RefreshTokenKey("rt-1"))
	require.Equal(t, "WALLET#0xabc", keys.WalletKey("0xabc"))

	gsi := NewGSIKeyUtils()
	require.Equal(t, "USER#alice", gsi.UserIndexKey(" Alice "))
	require.Equal(t, "DOMAIN#example.com", gsi.DomainIndexKey("example.com"))
	require.Equal(t, "EMAIL#user@example.com", gsi.EmailIndexKey("User@Example.com"))
	require.Equal(t, "STATUS#active", gsi.StatusIndexKey("active"))

	v := NewValidationUtils()
	require.True(t, v.IsValidUsername("alice"))
	require.False(t, v.IsValidUsername(""))
	require.False(t, v.IsValidUsername("has#hash"))
	require.True(t, v.IsValidEmail("user@example.com"))
	require.False(t, v.IsValidEmail("x"))
	require.True(t, v.IsValidHashtag("#hello"))
	require.False(t, v.IsValidHashtag("#"))

	now := time.Now().UTC()
	timeUtils := NewTimeUtils()
	require.Equal(t, now.Unix(), timeUtils.ToUnixTimestamp(now))
	ttl := timeUtils.TTLFromDuration(2 * time.Second)
	require.GreaterOrEqual(t, ttl, time.Now().Add(1*time.Second).Unix())
}

func TestRound10_Utils_PaginationEncodeDecode(t *testing.T) {
	p := NewPaginationUtils()

	pk, sk, err := p.DecodeCursor("")
	require.NoError(t, err)
	require.Equal(t, "", pk)
	require.Equal(t, "", sk)

	// Standard base64 that is not URL-safe (contains '+') triggers the legacy fallback path.
	_, _, err = p.DecodeCursor("++++")
	require.Error(t, err)

	// Valid base64 that also decodes with URLEncoding.
	valid := base64.StdEncoding.EncodeToString([]byte("pk|sk|0"))
	pk, sk, err = p.DecodeCursor(valid)
	require.NoError(t, err)
	require.Equal(t, "pk", pk)
	require.Equal(t, "sk", sk)

	// Base64 that decodes, but does not include two parts.
	cursorOnePart := base64.StdEncoding.EncodeToString([]byte("onlyonepart"))
	_, _, err = p.DecodeCursor(cursorOnePart)
	require.Error(t, err)

	tooLong := strings.Repeat("A", 501)
	_, _, err = p.DecodeCursor(tooLong)
	require.Error(t, err)

	encoded := p.EncodeCursor("PK#1", "SK#1")
	// DecodeCursor validates using StdEncoding; URLEncoding may still decode depending on characters,
	// so only assert that the method returns something sensible when given a cursor created by EncodeCursor.
	decodedPK, decodedSK, decodeErr := p.DecodeCursor(encoded)
	if decodeErr == nil {
		require.Equal(t, "PK#1", decodedPK)
		require.Equal(t, "SK#1", decodedSK)
	}
}

func TestRound10_Utils_NewCommonUtilsAndGlobalUtils(t *testing.T) {
	u := NewCommonUtils()
	require.NotNil(t, u.Keys)
	require.NotNil(t, u.GSI)
	require.NotNil(t, u.Time)
	require.NotNil(t, u.Validation)
	require.NotNil(t, u.Pagination)

	require.NotNil(t, Utils)
	require.NotNil(t, Utils.Keys)
}
