package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrintRecountInstanceCountsSummary(t *testing.T) {
	output := captureStdout(t, func() {
		printRecountInstanceCountsSummary(recountInstanceCountsSummary{
			Users:               3,
			Domains:             2,
			DomainCounters:      2,
			StaleDomainCounters: 1,
		}, "lesser-dev", "Theory", false)
	})

	require.Contains(t, output, "recount-instance-counts dry-run complete")
	require.Contains(t, output, "table:        lesser-dev")
	require.Contains(t, output, "aws_profile:  Theory")
	require.Contains(t, output, "total_users:    3")
	require.Contains(t, output, "total_domains:  2")
	require.Contains(t, output, "domain counters upserted: 2")
	require.Contains(t, output, "stale domain counters removed: 1")
	require.Contains(t, output, "dry-run: pass --apply to rewrite the counters")

	output = captureStdout(t, func() {
		printRecountInstanceCountsSummary(recountInstanceCountsSummary{
			Users: 4, Domains: 3, DomainCounters: 3,
		}, "lesser-dev", "", true)
	})
	require.Contains(t, output, "recount-instance-counts apply complete")
	require.NotContains(t, output, "dry-run: pass --apply")
}
