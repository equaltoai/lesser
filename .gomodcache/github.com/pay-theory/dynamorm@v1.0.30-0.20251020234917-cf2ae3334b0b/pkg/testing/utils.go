package testing

import (
	"fmt"
	"reflect"
)

// getTypeString returns the type string for use with mock.AnythingOfType
func getTypeString(v interface{}) string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		return fmt.Sprintf("*%s", t.Elem().Name())
	}
	return t.Name()
}

// copyValue copies the value from src to dst using reflection
func copyValue(dst, src interface{}) {
	dstVal := reflect.ValueOf(dst)
	srcVal := reflect.ValueOf(src)

	if dstVal.Kind() == reflect.Ptr && srcVal.Kind() == reflect.Ptr {
		dstVal.Elem().Set(srcVal.Elem())
	} else if dstVal.Kind() == reflect.Ptr {
		dstVal.Elem().Set(srcVal)
	}
}
