package streaming

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandInfoHelpers(t *testing.T) {
	info := GetCommandInfo()
	require.NotEmpty(t, info)

	createStatus, ok := info[CmdCreateStatus]
	require.True(t, ok)
	assert.Equal(t, CategoryStatus, createStatus.Category)
	assert.True(t, createStatus.RequiresAuth)
	assert.NotEmpty(t, createStatus.RequiredFields)

	categories := GetCommandsByCategory()
	require.NotEmpty(t, categories)
	assert.NotEmpty(t, categories[CategoryStatus])

	assert.True(t, GetRequiredAuth(CmdCreateStatus))
	assert.True(t, GetRequiredAuth("unknown_command_type"))

	assert.True(t, IsAdminOnly(CmdAdminSuspendUser))
	assert.False(t, IsAdminOnly("unknown_command_type"))
}
