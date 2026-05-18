package main

import "testing"

func TestNoDefaultInitStorageContinuation(t *testing.T) {
	t.Chdir("../..")

	if err := checkNoDefaultInitStorageContinuation(); err != nil {
		t.Fatalf("default init storage continuation gate failed: %v", err)
	}
}
