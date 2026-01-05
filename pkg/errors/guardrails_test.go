package errors

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGuardrail_NoPointerEqualityAgainstStorageSentinels(t *testing.T) {
	root := findRepoRoot(t)

	// These comparisons are fragile and must never re-enter production code.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`==\s*storage\.ErrNotFound\b`),
		regexp.MustCompile(`!=\s*storage\.ErrNotFound\b`),
		regexp.MustCompile(`==\s*storage\.ErrAlreadyExists\b`),
		regexp.MustCompile(`!=\s*storage\.ErrAlreadyExists\b`),
		regexp.MustCompile(`==\s*storage\.ErrInvalidInput\b`),
		regexp.MustCompile(`!=\s*storage\.ErrInvalidInput\b`),
		regexp.MustCompile(`==\s*storage\.ErrUnauthorized\b`),
		regexp.MustCompile(`!=\s*storage\.ErrUnauthorized\b`),
		regexp.MustCompile(`==\s*storage\.ErrRateLimited\b`),
		regexp.MustCompile(`!=\s*storage\.ErrRateLimited\b`),
	}

	var offenders []string
	for _, dir := range []string{"cmd", "pkg", "graph"} {
		searchRoot := filepath.Join(root, dir)
		if _, err := os.Stat(searchRoot); err != nil {
			continue
		}

		err := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				switch d.Name() {
				case ".git", "bin", "tmp", "node_modules":
					return filepath.SkipDir
				}
				return nil
			}

			if filepath.Ext(path) != ".go" {
				return nil
			}

			b, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			for _, re := range patterns {
				if re.Find(b) != nil {
					offenders = append(offenders, fmt.Sprintf("%s matches %s", path, re.String()))
				}
			}
			return nil
		})
		require.NoError(t, err)
	}

	require.Empty(t, offenders, "found banned pointer-equality patterns:\n%s", offenders)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (go.mod) from %q", dir)
		}
		dir = parent
	}
}
