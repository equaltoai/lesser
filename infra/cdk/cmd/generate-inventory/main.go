package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cdk/inventory"
)

const (
	startMarker   = "<!-- INVENTORY_TABLE_START -->"
	endMarker     = "<!-- INVENTORY_TABLE_END -->"
	preamble      = "> This table is generated from `infra/cdk/inventory/LambdaInventory` via `./lesser generate inventory`. Do not edit the table manually; update the inventory and re-run the generator."
	targetRelPath = "../../docs/specs/01-lambda-inventory-matrix.md"
)

type row struct {
	name        string
	lambdaType  string
	triggers    string
	requiredEnv string
	role        string
	operations  string
}

type httpRouteOutput struct {
	Lambda string `json:"lambda"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

func main() {
	outPath := flag.String("out", targetRelPath, "target path for generated inventory doc")
	checkOnly := flag.Bool("check", false, "fail if the generated content would differ without writing")
	printHTTPRoutes := flag.Bool("print-http-routes", false, "print inventory HTTP routes as JSON and exit")
	flag.Parse()

	if *printHTTPRoutes {
		if err := emitHTTPRoutes(os.Stdout, inventory.LambdaInventory); err != nil {
			fmt.Fprintf(os.Stderr, "generate-inventory: %v\n", err)
			os.Exit(1)
		}
		return
	}

	targetPath := filepath.Clean(*outPath)
	if err := writeInventoryTable(targetPath, *checkOnly); err != nil {
		fmt.Fprintf(os.Stderr, "generate-inventory: %v\n", err)
		os.Exit(1)
	}
}

func writeInventoryTable(targetPath string, checkOnly bool) error {
	content, err := os.ReadFile(targetPath) // #nosec G304 -- local doc generation tool reads a developer-supplied path
	if err != nil {
		return fmt.Errorf("read target file %s: %w", targetPath, err)
	}

	rendered := renderTable(inventory.LambdaInventory)
	replacement := preamble + "\n\n" + rendered + "\n"

	updated, err := splice(string(content), replacement)
	if err != nil {
		return err
	}

	if checkOnly {
		if updated == string(content) {
			fmt.Println("inventory doc is up to date")
			return nil
		}
		return fmt.Errorf("inventory doc is stale; run './lesser generate inventory'")
	}

	// #nosec G306 -- doc output is not sensitive
	if err := os.WriteFile(targetPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write target file %s: %w", targetPath, err)
	}
	return nil
}

func emitHTTPRoutes(out *os.File, inv inventory.Inventory) error {
	routes := collectHTTPRoutes(inv)
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(routes)
}

func collectHTTPRoutes(inv inventory.Inventory) []httpRouteOutput {
	routes := make([]httpRouteOutput, 0)
	for _, spec := range inv.Lambdas {
		for _, route := range spec.HTTPRoutes {
			routes = append(routes, httpRouteOutput{
				Lambda: spec.Name,
				Method: route.Method,
				Path:   route.Path,
			})
		}
	}
	return routes
}

func splice(existing, replacement string) (string, error) {
	start := strings.Index(existing, startMarker)
	if start == -1 {
		return "", fmt.Errorf("start marker %q not found", startMarker)
	}
	end := strings.Index(existing, endMarker)
	if end == -1 {
		return "", fmt.Errorf("end marker %q not found", endMarker)
	}
	if end < start {
		return "", fmt.Errorf("end marker appears before start marker")
	}

	prefix := existing[:start+len(startMarker)]
	suffix := existing[end:]

	var buf bytes.Buffer
	buf.WriteString(prefix)
	buf.WriteString("\n\n")
	buf.WriteString(replacement)
	buf.WriteString(suffix)
	return buf.String(), nil
}

func renderTable(inv inventory.Inventory) string {
	rows := collectRows(inv)
	var b strings.Builder
	b.WriteString("| Name | Type | Triggers | Required Env Vars | Role | Operational Defaults |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			r.name, r.lambdaType, r.triggers, r.requiredEnv, r.role, r.operations)
	}
	return b.String()
}

func collectRows(inv inventory.Inventory) []row {
	defaults := inv.Defaults
	rows := make([]row, 0, len(inv.Lambdas))
	for _, spec := range inv.Lambdas {
		operations := formatOperations(spec, defaults)
		triggers := formatTriggers(spec)
		requiredEnv := joinOrDash(spec.RequiredEnvVars)
		rows = append(rows, row{
			name:        spec.Name,
			lambdaType:  string(spec.Type),
			triggers:    triggers,
			requiredEnv: requiredEnv,
			role:        string(spec.Role),
			operations:  operations,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})
	return rows
}

func formatOperations(spec inventory.LambdaSpec, defaults inventory.OperationalDefaults) string {
	mem := defaults.MemoryMB
	if spec.Overrides.MemoryMB != nil {
		mem = *spec.Overrides.MemoryMB
	}
	timeout := defaults.TimeoutSeconds
	if spec.Overrides.TimeoutSeconds != nil {
		timeout = *spec.Overrides.TimeoutSeconds
	}
	logs := defaults.LogRetentionDays
	if spec.Overrides.LogRetentionDays != nil {
		logs = *spec.Overrides.LogRetentionDays
	}
	return fmt.Sprintf("memory=%dMB; timeout=%ds; logs=%dd", mem, timeout, logs)
}

func formatTriggers(spec inventory.LambdaSpec) string {
	var parts []string
	for _, route := range spec.HTTPRoutes {
		parts = append(parts, fmt.Sprintf("HTTP: %s %s", route.Method, route.Path))
	}
	for _, route := range spec.WebSocketRoutes {
		parts = append(parts, formatWebSocketRoute(route))
	}
	for _, sqs := range spec.SQSTriggers {
		parts = append(parts, formatSQSTrigger(sqs))
	}
	for _, stream := range spec.StreamTriggers {
		parts = append(parts, formatStreamTrigger(stream))
	}
	for _, schedule := range spec.ScheduleTriggers {
		parts = append(parts, formatScheduleTrigger(schedule))
	}
	return joinOrDash(parts)
}

func formatWebSocketRoute(r inventory.WebSocketRoute) string {
	if r.API != "" {
		return fmt.Sprintf("WS: api=%s; route=%s", r.API, r.RouteKey)
	}
	return fmt.Sprintf("WS: route=%s", r.RouteKey)
}

func formatSQSTrigger(t inventory.SQSTrigger) string {
	entries := []string{fmt.Sprintf("queue=%s", t.Queue)}
	if t.ConsumeDeadLetterQueue {
		entries = append(entries, "consume=dlq")
	}
	if t.DeadLetterQueue != "" {
		entries = append(entries, fmt.Sprintf("dlq=%s", t.DeadLetterQueue))
	}
	if t.BatchSize > 0 {
		entries = append(entries, fmt.Sprintf("batch=%d", t.BatchSize))
	}
	if t.MaxBatchingWindowSeconds > 0 {
		entries = append(entries, fmt.Sprintf("window=%ds", t.MaxBatchingWindowSeconds))
	}
	if t.EnablePartialFailure {
		entries = append(entries, "partialFailure=true")
	}
	return "SQS: " + strings.Join(entries, "; ")
}

func formatStreamTrigger(t inventory.StreamTrigger) string {
	var entries []string
	if t.SourceTable != "" {
		entries = append(entries, fmt.Sprintf("table=%s", t.SourceTable))
	} else if t.StreamArn != "" {
		entries = append(entries, fmt.Sprintf("streamArn=%s", t.StreamArn))
	}
	if t.StartingPosition != "" {
		entries = append(entries, fmt.Sprintf("start=%s", t.StartingPosition))
	}
	if t.BatchSize > 0 {
		entries = append(entries, fmt.Sprintf("batch=%d", t.BatchSize))
	}
	if t.MaxBatchingWindowSeconds > 0 {
		entries = append(entries, fmt.Sprintf("window=%ds", t.MaxBatchingWindowSeconds))
	}
	if t.ParallelizationFactor > 0 {
		entries = append(entries, fmt.Sprintf("parallel=%d", t.ParallelizationFactor))
	}
	if t.MaxRetryAttempts > 0 {
		entries = append(entries, fmt.Sprintf("maxRetry=%d", t.MaxRetryAttempts))
	}
	if t.MaxRecordAgeSeconds > 0 {
		entries = append(entries, fmt.Sprintf("maxAge=%ds", t.MaxRecordAgeSeconds))
	}
	if t.PoisonRecordQueue != "" {
		entries = append(entries, fmt.Sprintf("poisonQueue=%s", t.PoisonRecordQueue))
	}
	if t.EnableBisectOnError {
		entries = append(entries, "bisect=true")
	}
	if t.ReportBatchItemFailures {
		entries = append(entries, "reportBatchItemFailures=true")
	}
	return "Stream: " + strings.Join(entries, "; ")
}

func formatScheduleTrigger(t inventory.ScheduleTrigger) string {
	if t.Input != "" {
		return fmt.Sprintf("Schedule: expression=%s; input=%s", t.Expression, t.Input)
	}
	return fmt.Sprintf("Schedule: expression=%s", t.Expression)
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "—"
	}
	return strings.Join(values, "<br>")
}
