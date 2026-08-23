package main

import (
	"slices"
	"testing"
)

func TestStageConfigContextKeysIncludeVAPIDConfiguration(t *testing.T) {
	for _, key := range []string{"vapidSecretArn", "vapidPublicKey", "vapidSubject"} {
		if !slices.Contains(stageConfigContextKeys, key) {
			t.Fatalf("stage config context must pass through %s", key)
		}
	}
}
