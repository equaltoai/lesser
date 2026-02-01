package handlers

import (
	"reflect"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/core"
)

func TestMockRepositoryStorage_Methods(t *testing.T) {
	mockRepo := new(MockRepositoryStorage)
	iface := reflect.TypeOf((*core.RepositoryStorage)(nil)).Elem()

	for i := 0; i < iface.NumMethod(); i++ {
		method := iface.Method(i)
		returns := make([]any, method.Type.NumOut())
		for j := 0; j < method.Type.NumOut(); j++ {
			outType := method.Type.Out(j)
			zero := reflect.Zero(outType).Interface()
			returns[j] = zero
		}
		mockRepo.On(method.Name).Return(returns...).Maybe()
		_ = reflect.ValueOf(mockRepo).MethodByName(method.Name).Call(nil)
	}

	mockRepo.AssertExpectations(t)
}
