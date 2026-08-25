package inventory

import (
	"slices"
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

func TestVAPIDConsumersDeclareRequiredEnvironmentAndRoleClasses(t *testing.T) {
	byName := make(map[string]LambdaSpec, len(LambdaInventory.Lambdas))
	for _, spec := range LambdaInventory.Lambdas {
		byName[spec.Name] = spec
	}

	api := byName["api"]
	if api.Role != RoleClassEncryption {
		t.Fatalf("api must retain the encryption role for VAPID rotation, got %q", api.Role)
	}
	if !slices.Contains(api.RequiredEnvVars, "VAPID_SECRET_ARN") {
		t.Fatalf("api must receive VAPID_SECRET_ARN for VAPID reads and rotation")
	}

	pushDelivery := byName["push-delivery"]
	if pushDelivery.Role != RoleClassBasic {
		t.Fatalf("push-delivery must retain the basic role, got %q", pushDelivery.Role)
	}
	if !slices.Contains(pushDelivery.RequiredEnvVars, "VAPID_SECRET_ARN") {
		t.Fatalf("push-delivery must receive VAPID_SECRET_ARN for VAPID reads")
	}
}
