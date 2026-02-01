package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExampleMigrations_ConstructorsAndNoOps(t *testing.T) {
	ctx := context.Background()

	lookup := NewExampleAddUserLookupIndex()
	require.NotNil(t, lookup)
	assert.Equal(t, "20240115_add_user_lookup_index", lookup.ID())
	assert.Equal(t, int64(20240115120000), lookup.Version())
	assert.Contains(t, lookup.Description(), "lookup")
	assert.Empty(t, lookup.Dependencies())
	require.NoError(t, lookup.Up(ctx, nil))
	require.NoError(t, lookup.Down(ctx, nil))

	gsi := NewExampleGSIMigration()
	require.NotNil(t, gsi)
	assert.Equal(t, "20240116_add_activity_timestamp_index", gsi.ID())
	assert.Equal(t, int64(20240116120000), gsi.Version())
	assert.Contains(t, gsi.Description(), "timestamp")

	dep := NewExampleDependentMigration()
	require.NotNil(t, dep)
	assert.Equal(t, "20240117_add_user_preferences", dep.ID())
	assert.Contains(t, dep.Dependencies(), "20240115_add_user_lookup_index")
	require.NoError(t, dep.Up(ctx, nil))
	require.NoError(t, dep.Down(ctx, nil))
}
