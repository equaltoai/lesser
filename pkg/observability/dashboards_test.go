package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboards_Basics(t *testing.T) {
	d := CreateLesserOverviewDashboard("us-east-1", "test")
	require.NotNil(t, d)
	assert.Contains(t, d.Name, "Lesser-Overview-test")
	require.NotEmpty(t, d.Widgets)

	jsonStr, err := d.ToJSON()
	require.NoError(t, err)
	assert.Contains(t, jsonStr, `"name"`)

	all := GetAllDashboards("us-east-1", "test")
	require.Len(t, all, 4)
}
