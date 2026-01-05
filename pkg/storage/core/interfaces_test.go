package core_test

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
)

func TestRepositoryFactory_ImplementsRepositoryStorage(_ *testing.T) {
	var _ core.RepositoryStorage = (*factory.RepositoryFactory)(nil)
}
