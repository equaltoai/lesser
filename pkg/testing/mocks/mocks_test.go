package mocks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewMockStorage_InitializesMaps(t *testing.T) {
	m := NewMockStorage()
	require.NotNil(t, m)
	require.NotNil(t, m.actors)
	require.NotNil(t, m.activities)
	require.NotNil(t, m.objects)
	require.Len(t, m.actors, 0)
	require.Len(t, m.activities, 0)
	require.Len(t, m.objects, 0)
}
