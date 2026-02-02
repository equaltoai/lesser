package interfaces_test

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
)

func TestStorageAdapter_ImplementsStorage(_ *testing.T) {
	var _ interfaces.Storage = (*theorydb.StorageAdapter)(nil)
}
