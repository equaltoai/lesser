package main

import "strings"

func goListLines(out string) []string {
	lines := make([]string, 0)
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "go: ") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
