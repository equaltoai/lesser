package main

import "testing"

func TestAuthorizedEmailSkillRequiresReview(t *testing.T) {
	t.Chdir("../..")

	if err := checkAuthorizedEmailSkillRequiresReview(); err != nil {
		t.Fatalf("authorized email skill gate failed: %v", err)
	}
}
