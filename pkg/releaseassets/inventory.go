package releaseassets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// LambdaInventoryPath is the canonical lambda inventory source used to define
// the published bundle contents.
const LambdaInventoryPath = "infra/cdk/inventory/lambdas.go"

var lambdaInventoryNamePattern = regexp.MustCompile(`\bName:\s*"([^"]+)"`)

// CanonicalLambdaNames returns the sorted canonical Lambda names declared in
// the CDK inventory.
func CanonicalLambdaNames(repoRoot string) ([]string, error) {
	path := filepath.Join(repoRoot, LambdaInventoryPath)
	data, err := os.ReadFile(path) // #nosec G304 -- file path is derived from repo root
	if err != nil {
		return nil, fmt.Errorf("read lambda inventory: %w", err)
	}

	matches := lambdaInventoryNamePattern.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no Lambda names found in %s", path)
	}

	seen := map[string]struct{}{}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		name := strings.TrimSpace(string(match[1]))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	sort.Strings(names)
	return names, nil
}
