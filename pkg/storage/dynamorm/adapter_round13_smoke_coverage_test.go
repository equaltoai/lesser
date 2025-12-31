package dynamorm

import (
	"context"
	"reflect"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestStorageAdapter_ExportedMethods_DoNotPanic_WithNilRepos(t *testing.T) {
	adapter := NewStorageAdapter(&SimpleRepositoryStorage{
		db:        nil,
		tableName: "test-table",
		logger:    zaptest.NewLogger(t),
	})

	adapterValue := reflect.ValueOf(adapter)
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()

	for i := 0; i < adapterValue.NumMethod(); i++ {
		method := adapterValue.Method(i)
		methodType := method.Type()
		methodName := adapterValue.Type().Method(i).Name

		args := make([]reflect.Value, methodType.NumIn())
		for j := 0; j < methodType.NumIn(); j++ {
			args[j] = testValueForType(methodType.In(j), contextType)
		}

		t.Run(methodName, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked: %v", r)
				}
			}()

			_ = method.Call(args)
		})
	}
}

func testValueForType(paramType reflect.Type, contextType reflect.Type) reflect.Value {
	if paramType == contextType || paramType.Implements(contextType) {
		return reflect.ValueOf(context.Background())
	}

	switch paramType.Kind() {
	case reflect.Bool:
		return reflect.ValueOf(false)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(1)).Convert(paramType)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflect.ValueOf(uint64(1)).Convert(paramType)
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(float64(0)).Convert(paramType)
	case reflect.String:
		return reflect.ValueOf("x")
	case reflect.Slice:
		return reflect.MakeSlice(paramType, 0, 0)
	case reflect.Map:
		return reflect.MakeMapWithSize(paramType, 0)
	case reflect.Pointer:
		return reflect.New(paramType.Elem())
	case reflect.Interface:
		return reflect.Zero(paramType)
	case reflect.Func:
		return reflect.MakeFunc(paramType, func(_ []reflect.Value) []reflect.Value {
			outs := make([]reflect.Value, paramType.NumOut())
			for i := 0; i < paramType.NumOut(); i++ {
				outs[i] = reflect.Zero(paramType.Out(i))
			}
			return outs
		})
	default:
		return reflect.Zero(paramType)
	}
}
