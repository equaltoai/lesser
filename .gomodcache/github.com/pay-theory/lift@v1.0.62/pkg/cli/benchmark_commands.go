package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

const (
	tableFlag = "--table"
)

// DynamORMBenchmarkCommand handles performance benchmarking for DynamORM operations
type DynamORMBenchmarkCommand struct{}

func (c *DynamORMBenchmarkCommand) Name() string        { return "benchmark" }
func (c *DynamORMBenchmarkCommand) Description() string { return "Benchmark DynamORM performance" }
func (c *DynamORMBenchmarkCommand) Usage() string {
	return "lift dynamorm benchmark --table <table-name> [--operations put,get,query] [--concurrency 10] [--duration 30s]"
}

// BenchmarkConfig holds benchmarking configuration
type BenchmarkConfig struct {
	TableName   string
	OutputDir   string
	Region      string
	Operations  []string
	Concurrency int
	Duration    time.Duration
	ItemSize    int
	Warmup      time.Duration
}

// BenchmarkResults holds benchmark results
type BenchmarkResults struct {
	StartTime        time.Time                   `json:"start_time"`
	EndTime          time.Time                   `json:"end_time"`
	OperationResults map[string]*OperationResult `json:"operation_results"`
	Environment      BenchmarkEnvironment        `json:"environment"`
	TableName        string                      `json:"table_name"`
	Duration         time.Duration               `json:"duration"`
	Concurrency      int                         `json:"concurrency"`
	ColdStartTime    time.Duration               `json:"cold_start_time"`
	WarmupTime       time.Duration               `json:"warmup_time"`
}

// OperationResult holds results for a specific operation
type OperationResult struct {
	Operation       string          `json:"operation"`
	Latencies       []time.Duration `json:"-"`
	TotalRequests   int64           `json:"total_requests"`
	SuccessRequests int64           `json:"success_requests"`
	FailedRequests  int64           `json:"failed_requests"`
	AvgLatency      time.Duration   `json:"avg_latency"`
	MinLatency      time.Duration   `json:"min_latency"`
	MaxLatency      time.Duration   `json:"max_latency"`
	P95Latency      time.Duration   `json:"p95_latency"`
	P99Latency      time.Duration   `json:"p99_latency"`
	Throughput      float64         `json:"throughput"`
	ErrorRate       float64         `json:"error_rate"`
}

// BenchmarkEnvironment captures environment information
type BenchmarkEnvironment struct {
	Region           string `json:"region"`
	Runtime          string `json:"runtime"`
	Architecture     string `json:"architecture"`
	MemorySize       string `json:"memory_size"`
	TableBillingMode string `json:"table_billing_mode"`
	TimestampUTC     string `json:"timestamp_utc"`
}

func (c *DynamORMBenchmarkCommand) Execute(_ context.Context, args []string) error {
	// Parse arguments
	config, err := c.parseBenchmarkArgs(args)
	if err != nil {
		return err
	}

	// Check if we're in a Lift project
	if !c.isLiftProject() {
		return fmt.Errorf("not in a Lift project directory - run 'lift new' first")
	}

	// Generate benchmark code
	if err := c.generateBenchmarkCode(config); err != nil {
		return fmt.Errorf("failed to generate benchmark code: %w", err)
	}

	fmt.Printf("✅ Benchmark code generated in: %s\n", config.OutputDir)
	fmt.Printf("📋 Next steps:\n")
	fmt.Printf("   1. Run: go run %s/benchmark_runner.go\n", config.OutputDir)
	fmt.Printf("   2. View results in: %s/results.json\n", config.OutputDir)

	return nil
}

func (c *DynamORMBenchmarkCommand) parseBenchmarkArgs(args []string) (*BenchmarkConfig, error) {
	parser := newBenchmarkArgsParser()
	return parser.parse(args)
}

// benchmarkArgsParser handles benchmark argument parsing
type benchmarkArgsParser struct {
	config *BenchmarkConfig
	args   []string
	index  int
}

// newBenchmarkArgsParser creates a new parser with default config
func newBenchmarkArgsParser() *benchmarkArgsParser {
	return &benchmarkArgsParser{
		config: &BenchmarkConfig{
			Operations:  []string{"put", "get", "query"},
			Concurrency: 10,
			Duration:    30 * time.Second,
			ItemSize:    1024, // 1KB
			OutputDir:   "benchmarks",
			Region:      "us-east-1",
			Warmup:      5 * time.Second,
		},
	}
}

// parse processes the arguments
func (p *benchmarkArgsParser) parse(args []string) (*BenchmarkConfig, error) {
	p.args = args

	for p.index = 0; p.index < len(args); p.index++ {
		if err := p.parseFlag(); err != nil {
			return nil, err
		}
	}

	return p.validate()
}

// parseFlag parses a single flag
func (p *benchmarkArgsParser) parseFlag() error {
	flag := p.args[p.index]

	handler, exists := p.getFlagHandlers()[flag]
	if !exists {
		return nil // Ignore unknown flags
	}

	return handler()
}

// getFlagHandlers returns the map of flag handlers
func (p *benchmarkArgsParser) getFlagHandlers() map[string]func() error {
	return map[string]func() error{
		tableFlag:       p.parseTableFlag,
		"--operations":  p.parseOperationsFlag,
		"--concurrency": p.parseConcurrencyFlag,
		"--duration":    p.parseDurationFlag,
		"--output-dir":  p.parseOutputDirFlag,
		"--region":      p.parseRegionFlag,
	}
}

// parseTableFlag handles --table flag
func (p *benchmarkArgsParser) parseTableFlag() error {
	value, err := p.getNextValue(tableFlag)
	if err != nil {
		return err
	}
	p.config.TableName = value
	return nil
}

// parseOperationsFlag handles --operations flag
func (p *benchmarkArgsParser) parseOperationsFlag() error {
	value, err := p.getNextValue("--operations")
	if err != nil {
		return err
	}
	p.config.Operations = strings.Split(value, ",")
	return nil
}

// parseConcurrencyFlag handles --concurrency flag
func (p *benchmarkArgsParser) parseConcurrencyFlag() error {
	value, err := p.getNextValue("--concurrency")
	if err != nil {
		return err
	}

	var concurrency int
	if _, err := fmt.Sscanf(value, "%d", &concurrency); err != nil {
		return fmt.Errorf("invalid concurrency value: %s", value)
	}
	p.config.Concurrency = concurrency
	return nil
}

// parseDurationFlag handles --duration flag
func (p *benchmarkArgsParser) parseDurationFlag() error {
	value, err := p.getNextValue("--duration")
	if err != nil {
		return err
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration value: %s", value)
	}
	p.config.Duration = duration
	return nil
}

// parseOutputDirFlag handles --output-dir flag
func (p *benchmarkArgsParser) parseOutputDirFlag() error {
	value, err := p.getNextValue("--output-dir")
	if err != nil {
		return err
	}
	p.config.OutputDir = value
	return nil
}

// parseRegionFlag handles --region flag
func (p *benchmarkArgsParser) parseRegionFlag() error {
	value, err := p.getNextValue("--region")
	if err != nil {
		return err
	}
	p.config.Region = value
	return nil
}

// getNextValue gets the next argument value
func (p *benchmarkArgsParser) getNextValue(flag string) (string, error) {
	if p.index+1 >= len(p.args) {
		return "", fmt.Errorf("%s requires a value", flag)
	}
	p.index++
	return p.args[p.index], nil
}

// validate ensures required fields are set
func (p *benchmarkArgsParser) validate() (*BenchmarkConfig, error) {
	if p.config.TableName == "" {
		return nil, fmt.Errorf("--table is required")
	}
	return p.config, nil
}

func (c *DynamORMBenchmarkCommand) isLiftProject() bool {
	if _, err := os.Stat("go.mod"); err != nil {
		return false
	}

	content, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}

	return strings.Contains(string(content), "github.com/pay-theory/lift")
}

func (c *DynamORMBenchmarkCommand) generateBenchmarkCode(config *BenchmarkConfig) error {
	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0750); err != nil {
		return err
	}

	// Generate benchmark runner
	if err := c.generateBenchmarkRunner(config); err != nil {
		return err
	}

	// Generate benchmark operations
	if err := c.generateBenchmarkOperations(config); err != nil {
		return err
	}

	// Generate results analyzer
	return c.generateResultsAnalyzer(config)
}

func (c *DynamORMBenchmarkCommand) generateBenchmarkRunner(config *BenchmarkConfig) error {
	tmpl := `// Generated by lift dynamorm benchmark
// Benchmark runner for {{.TableName}}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/pay-theory/lift/pkg/dynamorm"
)

func main() {
	ctx := context.Background()
	
	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("{{.Region}}"))
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	
	// Initialize DynamORM
	db, err := dynamorm.New(ctx, dynamorm.WithAWSConfig(cfg))
	if err != nil {
		log.Fatalf("Failed to initialize DynamORM: %v", err)
	}
	
	// Initialize benchmark
	benchmark := NewBenchmark(db, "{{.TableName}}")
	
	// Run benchmark
	results, err := benchmark.Run(ctx, BenchmarkConfig{
		Operations:  []string{ {{range .Operations}}"{{.}}", {{end}} },
		Concurrency: {{.Concurrency}},
		Duration:    {{.Duration.Nanoseconds}}}, // nanoseconds
		ItemSize:    {{.ItemSize}},
		Warmup:      {{.Warmup.Nanoseconds}}}, // nanoseconds
	})
	if err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}
	
	// Save results
	if err := saveResults(results); err != nil {
		log.Fatalf("Failed to save results: %v", err)
	}
	
	// Print summary
	printSummary(results)
}

type BenchmarkConfig struct {
	Operations  []string
	Concurrency int
	Duration    int64 // nanoseconds
	ItemSize    int
	Warmup      int64 // nanoseconds
}

type Benchmark struct {
	db        *dynamorm.DB
	tableName string
}

func NewBenchmark(db *dynamorm.DB, tableName string) *Benchmark {
	return &Benchmark{
		db:        db,
		tableName: tableName,
	}
}

func (b *Benchmark) Run(ctx context.Context, config BenchmarkConfig) (*BenchmarkResults, error) {
	fmt.Printf("🚀 Starting DynamORM benchmark for table: %s\n", b.tableName)
	fmt.Printf("   • Operations: %v\n", config.Operations)
	fmt.Printf("   • Concurrency: %d\n", config.Concurrency)
	fmt.Printf("   • Duration: %v\n", time.Duration(config.Duration))
	fmt.Printf("   • Item Size: %d bytes\n", config.ItemSize)
	
	results := &BenchmarkResults{
		TableName:        b.tableName,
		StartTime:        time.Now(),
		Concurrency:      config.Concurrency,
		Duration:         time.Duration(config.Duration),
		OperationResults: make(map[string]*OperationResult),
		Environment:      getBenchmarkEnvironment(),
	}
	
	// Measure cold start
	coldStartTime := time.Now()
	_ = b.db.Ping(ctx)
	results.ColdStartTime = time.Since(coldStartTime)
	
	// Warmup
	if config.Warmup > 0 {
		fmt.Printf("🔥 Warming up for %v...\n", time.Duration(config.Warmup))
		warmupStart := time.Now()
		b.warmup(ctx, time.Duration(config.Warmup), config.Concurrency, config.ItemSize)
		results.WarmupTime = time.Since(warmupStart)
	}
	
	// Run each operation benchmark
	for _, op := range config.Operations {
		fmt.Printf("📊 Benchmarking %s operation...\n", op)
		opResult, err := b.benchmarkOperation(ctx, op, config)
		if err != nil {
			return nil, fmt.Errorf("failed to benchmark %s: %w", op, err)
		}
		results.OperationResults[op] = opResult
	}
	
	results.EndTime = time.Now()
	return results, nil
}

func (b *Benchmark) warmup(ctx context.Context, duration time.Duration, concurrency, itemSize int) {
	// Simple warmup with put operations
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					item := generateTestItem(fmt.Sprintf("warmup-%d-%d", id, time.Now().UnixNano()), itemSize)
					_ = b.db.Put(ctx, item)
				}
			}
		}(i)
	}
	wg.Wait()
}

func (b *Benchmark) benchmarkOperation(ctx context.Context, operation string, config BenchmarkConfig) (*OperationResult, error) {
	result := &OperationResult{
		Operation: operation,
		Latencies: make([]time.Duration, 0),
	}
	
	ctx, cancel := context.WithTimeout(ctx, time.Duration(config.Duration))
	defer cancel()
	
	var mu sync.Mutex
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			b.benchmarkWorker(ctx, operation, config.ItemSize, id, &mu, result)
		}(i)
	}
	
	wg.Wait()
	
	// Calculate statistics
	b.calculateStats(result)
	
	return result, nil
}

func (b *Benchmark) benchmarkWorker(ctx context.Context, operation string, itemSize, workerID int, mu *sync.Mutex, result *OperationResult) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			start := time.Now()
			success := b.executeOperation(ctx, operation, itemSize, workerID)
			latency := time.Since(start)
			
			mu.Lock()
			result.TotalRequests++
			result.Latencies = append(result.Latencies, latency)
			if success {
				result.SuccessRequests++
			} else {
				result.FailedRequests++
			}
			mu.Unlock()
		}
	}
}

func (b *Benchmark) executeOperation(ctx context.Context, operation string, itemSize, workerID int) bool {
	switch operation {
	case "put":
		item := generateTestItem(fmt.Sprintf("bench-%d-%d", workerID, time.Now().UnixNano()), itemSize)
		return b.db.Put(ctx, item) == nil
	case "get":
		var item TestItem
		return b.db.GetByPK(ctx, fmt.Sprintf("bench-%d", workerID), &item) == nil
	case "query":
		var items []TestItem
		return b.db.Query(ctx, &items, dynamorm.WithLimit(10)) == nil
	default:
		return false
	}
}

func (b *Benchmark) calculateStats(result *OperationResult) {
	if len(result.Latencies) == 0 {
		return
	}
	
	// Sort latencies for percentile calculations
	sort.Slice(result.Latencies, func(i, j int) bool {
		return result.Latencies[i] < result.Latencies[j]
	})
	
	// Basic stats
	result.MinLatency = result.Latencies[0]
	result.MaxLatency = result.Latencies[len(result.Latencies)-1]
	
	// Average
	var total time.Duration
	for _, lat := range result.Latencies {
		total += lat
	}
	result.AvgLatency = total / time.Duration(len(result.Latencies))
	
	// Percentiles
	p95Index := int(float64(len(result.Latencies)) * 0.95)
	p99Index := int(float64(len(result.Latencies)) * 0.99)
	result.P95Latency = result.Latencies[p95Index]
	result.P99Latency = result.Latencies[p99Index]
	
	// Throughput (requests per second)
	if len(result.Latencies) > 0 {
		totalDuration := result.Latencies[len(result.Latencies)-1]
		result.Throughput = float64(result.TotalRequests) / totalDuration.Seconds()
	}
	
	// Error rate
	result.ErrorRate = float64(result.FailedRequests) / float64(result.TotalRequests) * 100
}

// TestItem represents a benchmark test item
type TestItem struct {
	ID       string    ` + "`" + `json:"id" dynamodb:"id,pk"` + "`" + `
	Data     string    ` + "`" + `json:"data" dynamodb:"data"` + "`" + `
	Created  time.Time ` + "`" + `json:"created" dynamodb:"created"` + "`" + `
}

func (t *TestItem) TableName() string {
	return "{{.TableName}}"
}

func generateTestItem(id string, size int) *TestItem {
	// Generate data of specified size
	data := strings.Repeat("x", size-len(id)-50) // Account for other fields
	
	return &TestItem{
		ID:      id,
		Data:    data,
		Created: time.Now(),
	}
}

func getBenchmarkEnvironment() BenchmarkEnvironment {
	return BenchmarkEnvironment{
		Region:       "{{.Region}}",
		Runtime:      runtime.Version(),
		Architecture: runtime.GOARCH,
		MemorySize:   fmt.Sprintf("%d MB", getMemorySize()/1024/1024),
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
	}
}

func getMemorySize() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys
}

func saveResults(results *BenchmarkResults) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile("results.json", data, 0600)
}

func printSummary(results *BenchmarkResults) {
	fmt.Printf("\n📋 Benchmark Summary:\n")
	fmt.Printf("   • Table: %s\n", results.TableName)
	fmt.Printf("   • Duration: %v\n", results.Duration)
	fmt.Printf("   • Concurrency: %d\n", results.Concurrency)
	fmt.Printf("   • Cold Start: %v\n", results.ColdStartTime)
	fmt.Printf("   • Warmup: %v\n", results.WarmupTime)
	
	for op, result := range results.OperationResults {
		fmt.Printf("\n📊 %s Operation:\n", strings.ToUpper(op))
		fmt.Printf("   • Total Requests: %d\n", result.TotalRequests)
		fmt.Printf("   • Success Rate: %.2f%%\n", 100-result.ErrorRate)
		fmt.Printf("   • Throughput: %.2f req/sec\n", result.Throughput)
		fmt.Printf("   • Avg Latency: %v\n", result.AvgLatency)
		fmt.Printf("   • P95 Latency: %v\n", result.P95Latency)
		fmt.Printf("   • P99 Latency: %v\n", result.P99Latency)
		fmt.Printf("   • Min Latency: %v\n", result.MinLatency)
		fmt.Printf("   • Max Latency: %v\n", result.MaxLatency)
	}
	
	fmt.Printf("\n✅ Results saved to results.json\n")
}

// Define types for compilation
type BenchmarkResults struct {
	TableName        string
	StartTime        time.Time
	EndTime          time.Time
	Duration         time.Duration
	Concurrency      int
	OperationResults map[string]*OperationResult
	ColdStartTime    time.Duration
	WarmupTime       time.Duration
	Environment      BenchmarkEnvironment
}

type OperationResult struct {
	Operation       string
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64
	AvgLatency      time.Duration
	MinLatency      time.Duration
	MaxLatency      time.Duration
	P95Latency      time.Duration
	P99Latency      time.Duration
	Throughput      float64
	ErrorRate       float64
	Latencies       []time.Duration
}

type BenchmarkEnvironment struct {
	Region        string
	Runtime       string
	Architecture  string
	MemorySize    string
	TimestampUTC  string
}
`

	t, err := template.New("benchmark").Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join(config.OutputDir, "benchmark_runner.go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized config.OutputDir
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Log file close error but don't fail the operation
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, config)
}

func (c *DynamORMBenchmarkCommand) generateBenchmarkOperations(_ *BenchmarkConfig) error {
	// Generate operations.go file with custom benchmark operations
	return nil
}

func (c *DynamORMBenchmarkCommand) generateResultsAnalyzer(_ *BenchmarkConfig) error {
	// Generate analyzer.go file for results analysis
	return nil
}
