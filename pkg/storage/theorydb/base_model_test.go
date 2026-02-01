package theorydb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardModel_Hooks(t *testing.T) {
	var m StandardModel
	require.NoError(t, m.BeforeCreate())

	assert.False(t, m.CreatedAt.IsZero())
	assert.False(t, m.UpdatedAt.IsZero())
	assert.True(t, m.CreatedAt.Equal(m.UpdatedAt))

	prevUpdated := time.Now().Add(-1 * time.Hour)
	m.UpdatedAt = prevUpdated
	require.NoError(t, m.BeforeUpdate())
	assert.True(t, m.UpdatedAt.After(prevUpdated))
}

func TestGenerateKeys(t *testing.T) {
	pk, sk := GenerateKeys(KeyComponents{
		EntityType: "user",
		ID:         "123",
	})
	assert.Equal(t, "user#123", pk)
	assert.Equal(t, "user#123", sk)

	pk, sk = GenerateKeys(KeyComponents{
		EntityType: "user",
		ID:         "123",
		Tenant:     "t1",
		Sort:       "custom#sort",
	})
	assert.Equal(t, "tenant#t1#user#123", pk)
	assert.Equal(t, "custom#sort", sk)
}

func TestKeyHelpers(t *testing.T) {
	pk, sk := GenerateSimpleKeys("note", "n1")
	assert.Equal(t, "note#n1", pk)
	assert.Equal(t, "note#n1", sk)

	pk, sk = GenerateTenantKeys("t1", "note", "n1")
	assert.Equal(t, "tenant#t1", pk)
	assert.Equal(t, "note#n1", sk)

	pk, sk = GenerateHierarchicalKeys("user", "u1", "post", "p1")
	assert.Equal(t, "user#u1", pk)
	assert.Equal(t, "post#p1", sk)
}

func TestExtractIDFromKey(t *testing.T) {
	assert.Equal(t, "123", ExtractIDFromKey("user#123", "user"))
	assert.Equal(t, "user#123", ExtractIDFromKey("user#123", "post"))
	assert.Equal(t, "user#", ExtractIDFromKey("user#", "user"))
}

func TestTTLModel_SetTTL(t *testing.T) {
	var m TTLModel
	start := time.Now()
	m.SetTTL(10 * time.Second)
	end := time.Now()

	assert.GreaterOrEqual(t, m.TTL, start.Add(10*time.Second).Unix())
	assert.LessOrEqual(t, m.TTL, end.Add(10*time.Second).Unix())
}
