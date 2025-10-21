package dynamorm

import (
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"

	"github.com/pay-theory/lift/pkg/dynamorm/mocks"
)

// DBFactory interface for creating DynamORM instances
type DBFactory interface {
	CreateDB(config session.Config) (core.ExtendedDB, error)
}

// DefaultDBFactory creates real DynamORM instances
type DefaultDBFactory struct{}

func (f *DefaultDBFactory) CreateDB(config session.Config) (core.ExtendedDB, error) {
	// Use the dynamorm.New function which returns core.ExtendedDB
	return dynamorm.New(config)
}

// MockDBFactory creates mock DynamORM instances for testing
type MockDBFactory struct {
	MockDB     core.ExtendedDB
	Error      error
	OnCreateDB func(config session.Config)
}

func (f *MockDBFactory) CreateDB(config session.Config) (core.ExtendedDB, error) {
	if f.OnCreateDB != nil {
		f.OnCreateDB(config)
	}
	if f.Error != nil {
		return nil, f.Error
	}
	return f.MockDB, nil
}

// NewMockDBFactory creates a factory with a MockExtendedDB
func NewMockDBFactory() *MockDBFactory {
	return &MockDBFactory{
		MockDB: mocks.NewMockExtendedDB(),
	}
}
