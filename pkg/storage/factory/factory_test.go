package factory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterStorageConverters_nilDB(t *testing.T) {
	require.ErrorContains(t, registerStorageConverters(nil), "dynamorm DB is nil")
}

func TestNewRepositoryFactory_nilDB(t *testing.T) {
	f, err := NewRepositoryFactory(nil, "table", nil)
	require.Nil(t, f)
	require.ErrorContains(t, err, "dynamorm DB is nil")
}
