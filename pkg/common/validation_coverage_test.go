package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"valid lowercase", "alice", false},
		{"valid with numbers", "alice123", false},
		{"valid with underscore", "alice_bob", false},
		{"empty", "", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz12345", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateStatusID(t *testing.T) {
	tests := []struct {
		name     string
		statusID string
		wantErr  bool
	}{
		{"valid numeric", "12345678901234567890", false},
		{"valid alphanumeric", "abc123def456", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatusID(tt.statusID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAccountIDCoverage(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		wantErr   bool
	}{
		{"valid numeric", "12345678901234567890", false},
		{"valid alphanumeric", "abc123", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 501), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountID(tt.accountID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateLimitCoverage(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		max     int
		wantErr bool
	}{
		{"valid within range", 10, 40, false},
		{"zero allowed", 0, 40, false},
		{"negative", -1, 40, true},
		{"exceeds max", 50, 40, true},
		{"equals max", 40, 40, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLimit(tt.limit, tt.max)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateOffsetCoverage(t *testing.T) {
	tests := []struct {
		name    string
		offset  int
		wantErr bool
	}{
		{"zero", 0, false},
		{"positive", 100, false},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOffset(tt.offset)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseLimitFunctions(t *testing.T) {
	t.Run("ParseFollowLimit", func(t *testing.T) {
		limit, err := ParseFollowLimit("50")
		require.NoError(t, err)
		assert.LessOrEqual(t, limit, 80)
		assert.GreaterOrEqual(t, limit, 1)
	})

	t.Run("ParseFollowLimit invalid returns error", func(t *testing.T) {
		_, err := ParseFollowLimit("invalid")
		assert.Error(t, err)
	})

	t.Run("ParseSearchLimit", func(t *testing.T) {
		limit, err := ParseSearchLimit("25")
		require.NoError(t, err)
		assert.LessOrEqual(t, limit, 80)
	})

	t.Run("ParseHashtagLimit", func(t *testing.T) {
		limit, err := ParseHashtagLimit("10")
		require.NoError(t, err)
		assert.LessOrEqual(t, limit, 200)
	})

	t.Run("ParseAdminLimit", func(t *testing.T) {
		limit, err := ParseAdminLimit("50")
		require.NoError(t, err)
		assert.LessOrEqual(t, limit, 100)
	})

	t.Run("ParseFederationLimit", func(t *testing.T) {
		limit, err := ParseFederationLimit("100")
		require.NoError(t, err)
		assert.LessOrEqual(t, limit, 200)
	})
}

func TestValidateAccountParamIDCoverage(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "12345", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 501), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountParamID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateStatusParamIDCoverage(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "12345", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatusParamID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFilterParamIDCoverage(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "12345", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 101), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilterParamID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateKeywordParamIDCoverage(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "12345", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 101), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeywordParamID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsPubliclyVisibleCoverage(t *testing.T) {
	assert.True(t, IsPubliclyVisible("public"))
	assert.True(t, IsPubliclyVisible("unlisted"))
	assert.False(t, IsPubliclyVisible("private"))
	assert.False(t, IsPubliclyVisible("direct"))
	assert.False(t, IsPubliclyVisible("unknown"))
}

func TestValidateFilterContextCoverage(t *testing.T) {
	tests := []struct {
		name     string
		contexts []string
		wantErr  bool
	}{
		{"home", []string{"home"}, false},
		{"notifications", []string{"notifications"}, false},
		{"public", []string{"public"}, false},
		{"thread", []string{"thread"}, false},
		{"account", []string{"account"}, false},
		{"multiple", []string{"home", "public"}, false},
		{"empty", []string{}, true},
		{"invalid", []string{"invalid"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilterContext(tt.contexts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSearchQueryCoverage(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"valid query", "hello world", false},
		{"short query", "hi", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 501), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSearchQuery(tt.query)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFloatRangeCoverage(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		min     float64
		max     float64
		wantErr bool
	}{
		{"in range", 5.0, 0.0, 10.0, false},
		{"at min", 0.0, 0.0, 10.0, false},
		{"at max", 10.0, 0.0, 10.0, false},
		{"below min", -1.0, 0.0, 10.0, true},
		{"above max", 11.0, 0.0, 10.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFloatRange("test_field", tt.value, tt.min, tt.max)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateIntRangeCoverage(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		wantErr bool
	}{
		{"in range", 5, 0, 10, false},
		{"at min", 0, 0, 10, false},
		{"at max", 10, 0, 10, false},
		{"below min", -1, 0, 10, true},
		{"above max", 11, 0, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIntRange("test_field", tt.value, tt.min, tt.max)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateUsernameParamIDCoverage(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"valid", "alice", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsernameParamID(tt.username)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAcctParameterCoverage(t *testing.T) {
	tests := []struct {
		name    string
		acct    string
		wantErr bool
	}{
		{"username only", "alice", false},
		{"username@domain", "alice@example.com", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 501), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAcctParameter(tt.acct)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseAndValidateActivityPubLimitCoverage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		limit, err := ParseAndValidateActivityPubLimit("50")
		require.NoError(t, err)
		assert.Equal(t, 50, limit)
	})

	t.Run("empty uses default", func(t *testing.T) {
		limit, err := ParseAndValidateActivityPubLimit("")
		require.NoError(t, err)
		assert.Equal(t, 20, limit)
	})
}

func TestParseStatusContextLimitCoverage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		limit, err := ParseStatusContextLimit("30")
		require.NoError(t, err)
		assert.Equal(t, 30, limit)
	})
}

func TestParseStatusTimelineLimitCoverage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		limit, err := ParseStatusTimelineLimit("30")
		require.NoError(t, err)
		assert.Equal(t, 30, limit)
	})

	t.Run("empty uses default", func(t *testing.T) {
		limit, err := ParseStatusTimelineLimit("")
		require.NoError(t, err)
		assert.Equal(t, 20, limit)
	})
}

func TestParseAccountStatusesLimitCoverage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		limit, err := ParseAccountStatusesLimit("30")
		require.NoError(t, err)
		assert.Equal(t, 30, limit)
	})

	t.Run("empty uses default", func(t *testing.T) {
		limit, err := ParseAccountStatusesLimit("")
		require.NoError(t, err)
		assert.Equal(t, 20, limit)
	})
}

func TestValidateJSONFieldCoverage(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		err := ValidateJSONField(`{"key": "value"}`, "data")
		assert.NoError(t, err)
	})

	t.Run("empty allowed", func(t *testing.T) {
		err := ValidateJSONField("", "data")
		assert.NoError(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		err := ValidateJSONField("{invalid}", "data")
		assert.Error(t, err)
	})
}

func TestValidateEnumFieldCoverage(t *testing.T) {
	t.Run("valid enum", func(t *testing.T) {
		err := ValidateEnumField("red", []string{"red", "green", "blue"}, "color")
		assert.NoError(t, err)
	})

	t.Run("empty allowed", func(t *testing.T) {
		err := ValidateEnumField("", []string{"red", "green"}, "color")
		assert.NoError(t, err)
	})

	t.Run("invalid enum", func(t *testing.T) {
		err := ValidateEnumField("yellow", []string{"red", "green"}, "color")
		assert.Error(t, err)
	})
}

func TestValidateRepositoryAccessCoverage(t *testing.T) {
	t.Run("valid access", func(t *testing.T) {
		err := ValidateRepositoryAccess("user123", "resource456", "read")
		assert.NoError(t, err)
	})

	t.Run("missing user", func(t *testing.T) {
		err := ValidateRepositoryAccess("", "resource456", "read")
		assert.Error(t, err)
	})

	t.Run("missing resource", func(t *testing.T) {
		err := ValidateRepositoryAccess("user123", "", "read")
		assert.Error(t, err)
	})

	t.Run("missing operation", func(t *testing.T) {
		err := ValidateRepositoryAccess("user123", "resource456", "")
		assert.Error(t, err)
	})

	t.Run("invalid operation", func(t *testing.T) {
		err := ValidateRepositoryAccess("user123", "resource456", "invalid")
		assert.Error(t, err)
	})
}

func TestValidateEntityIDsListCoverage(t *testing.T) {
	t.Run("valid list", func(t *testing.T) {
		err := ValidateEntityIDsList([]string{"id1", "id2"}, "item")
		assert.NoError(t, err)
	})

	t.Run("empty list", func(t *testing.T) {
		err := ValidateEntityIDsList([]string{}, "item")
		assert.Error(t, err)
	})
}
