package models

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOAuthRefreshAuthorityKeysAreStableAndTupleBound(t *testing.T) {
	one := OAuthRefreshAuthorityPK("alice", "client", "https://api.example/mcp/alice")
	two := OAuthRefreshAuthorityPK("alice", "client", "https://api.example/mcp/bob")
	require.True(t, strings.HasPrefix(one, "OAUTH_REFRESH_AUTHORITY#"))
	require.NotEqual(t, one, two)
	require.Equal(t, one, OAuthRefreshAuthorityPK("alice", "client", "https://api.example/mcp/alice"))
}

func TestOAuthRefreshSuccessorArtifactCredentialIsEncrypted(t *testing.T) {
	field, ok := reflect.TypeOf(OAuthRefreshSuccessorArtifact{}).FieldByName("SuccessorToken")
	require.True(t, ok)
	require.Contains(t, field.Tag.Get("theorydb"), "encrypted")

	artifact := OAuthRefreshSuccessorArtifact{
		FamilyID: "family", PredecessorHash: "before", SuccessorHash: "after",
		SuccessorToken: "secret", SuccessorGeneration: 2, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, artifact.BeforeCreate())
	require.Equal(t, "OAUTH_REFRESH_FAMILY#family", artifact.PK)
	require.Equal(t, "SUCCESSOR#before", artifact.SK)
}
