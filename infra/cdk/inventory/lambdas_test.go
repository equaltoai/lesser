package inventory

import (
	"strings"
	"testing"
)

func TestStreamTriggersDeclareFiniteRetryAgeAndPoisonDestination(t *testing.T) {
	for _, spec := range LambdaInventory.Lambdas {
		for idx, trig := range spec.StreamTriggers {
			if trig.MaxRetryAttempts < 0 {
				t.Fatalf("%s stream trigger %d has negative retry attempts: %d", spec.Name, idx, trig.MaxRetryAttempts)
			}
			if trig.MaxRetryAttempts == 0 {
				t.Fatalf("%s stream trigger %d must declare finite retry attempts", spec.Name, idx)
			}
			if trig.MaxRecordAgeSeconds < 60 {
				t.Fatalf("%s stream trigger %d must declare finite max record age >= 60 seconds, got %d", spec.Name, idx, trig.MaxRecordAgeSeconds)
			}
			if strings.TrimSpace(trig.PoisonRecordQueue) == "" {
				t.Fatalf("%s stream trigger %d must declare a poison record queue", spec.Name, idx)
			}
		}
	}
}
