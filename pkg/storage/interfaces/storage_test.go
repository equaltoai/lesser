package interfaces_test

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

func TestStorageAdapter_ImplementsStorage(_ *testing.T) {
	var _ interfaces.Storage = (*dynamorm.StorageAdapter)(nil)
}
