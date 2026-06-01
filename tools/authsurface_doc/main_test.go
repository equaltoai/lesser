package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityPublicSurfaceDocGeneratedSectionIsCurrent(t *testing.T) {
	t.Parallel()

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	docPath := filepath.Join(repoRoot, filepath.FromSlash(docRelPath))
	if err := checkDoc(docPath); err != nil {
		t.Fatal(err)
	}
}

func TestSecurityPublicSurfaceDocCheckDetectsStaleGeneratedSection(t *testing.T) {
	t.Parallel()

	current := "before\n" + renderGeneratedSection() + "\nafter\n"
	stale := strings.Replace(current, "GET/HEAD", "GET", 1)
	updated, changed, err := renderUpdatedDoc(stale)
	if err != nil {
		t.Fatalf("render stale doc: %v", err)
	}
	if !changed {
		t.Fatal("stale generated section was not detected")
	}
	if updated != current {
		t.Fatalf("rendered doc mismatch after stale-section repair")
	}
}
