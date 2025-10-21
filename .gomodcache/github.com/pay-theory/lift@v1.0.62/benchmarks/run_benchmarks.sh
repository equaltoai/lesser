#!/bin/bash

# Lift Framework Performance Benchmark Runner
# Sprint 4 - Performance Benchmarking & Optimization

set -e

echo "🚀 Lift Framework Performance Benchmarks"
echo "========================================"
echo "Date: $(date)"
echo "Go Version: $(go version)"
echo ""

# Create results directory
RESULTS_DIR="benchmark_results/$(date +%Y-%m-%d_%H-%M-%S)"
mkdir -p "$RESULTS_DIR"

echo "📊 Results will be saved to: $RESULTS_DIR"
echo ""

# Function to run benchmarks and save results
run_benchmark_suite() {
    local suite_name=$1
    local pattern=$2
    local description=$3
    
    echo "🔍 Running $description..."
    echo "Pattern: $pattern"
    
    # Run benchmarks with memory profiling
    go test -bench="$pattern" \
           -benchmem \
           -count=3 \
           -timeout=30m \
           -cpu=1,2,4 \
           -benchtime=1s \
           ./... > "$RESULTS_DIR/${suite_name}_results.txt" 2>&1
    
    echo "✅ $description completed"
    echo ""
}

# Function to run benchmarks with CPU profiling
run_benchmark_with_profiling() {
    local suite_name=$1
    local pattern=$2
    local description=$3
    
    echo "🔬 Running $description with CPU profiling..."
    
    go test -bench="$pattern" \
           -benchmem \
           -count=1 \
           -cpuprofile="$RESULTS_DIR/${suite_name}_cpu.prof" \
           -memprofile="$RESULTS_DIR/${suite_name}_mem.prof" \
           -benchtime=5s \
           ./... > "$RESULTS_DIR/${suite_name}_profiled.txt" 2>&1
    
    echo "✅ $description with profiling completed"
    echo ""
}

echo "🏃‍♂️ Starting benchmark execution..."
echo ""

# 1. Cold Start Benchmarks
run_benchmark_suite "cold_start" "BenchmarkColdStart" "Cold Start Performance Tests"

# 2. Routing Benchmarks
run_benchmark_suite "routing" "BenchmarkRouting" "Routing Performance Tests"

# 3. Middleware Benchmarks
run_benchmark_suite "middleware" "BenchmarkMiddleware" "Middleware Chain Performance Tests"

# 4. Event Adapter Benchmarks
run_benchmark_suite "event_adapters" "BenchmarkAPIGateway|BenchmarkSQS|BenchmarkS3|BenchmarkEventBridge|BenchmarkScheduled|BenchmarkEvent" "Event Adapter Performance Tests"

# 5. Comprehensive Framework Benchmarks
run_benchmark_suite "framework" "BenchmarkFramework" "Framework Integration Performance Tests"

# 6. Concurrent Performance Tests
run_benchmark_suite "concurrent" "BenchmarkConcurrent" "Concurrent Performance Tests"

echo "🔬 Running detailed profiling on key benchmarks..."
echo ""

# Profile the most critical benchmarks
run_benchmark_with_profiling "critical_cold_start" "BenchmarkFrameworkInitializationTime" "Critical Cold Start Analysis"
run_benchmark_with_profiling "critical_routing" "BenchmarkRouting1000Routes" "Critical Routing Analysis"
run_benchmark_with_profiling "critical_middleware" "BenchmarkMiddlewareChain15" "Critical Middleware Analysis"

echo "📈 Generating performance summary..."

# Create a summary report
cat > "$RESULTS_DIR/BENCHMARK_SUMMARY.md" << EOF
# Lift Framework Performance Benchmark Results

**Date**: $(date)  
**Go Version**: $(go version)  
**Sprint**: 4 - Performance Benchmarking & Optimization  

## Benchmark Suites Executed

### 1. Cold Start Performance
- **File**: cold_start_results.txt
- **Focus**: Framework initialization overhead
- **Target**: <15ms cold start time
- **Key Metrics**: Initialization time, memory allocation, GC impact

### 2. Routing Performance
- **File**: routing_results.txt
- **Focus**: Route matching and parameter extraction
- **Target**: O(1) or O(log n) complexity
- **Key Metrics**: Route lookup time, memory usage, scalability

### 3. Middleware Chain Performance
- **File**: middleware_results.txt
- **Focus**: Middleware composition and execution
- **Target**: <0.1ms per middleware
- **Key Metrics**: Chain overhead, memory allocation, error handling

### 4. Event Adapter Performance
- **File**: event_adapters_results.txt
- **Focus**: Event parsing and adaptation
- **Target**: <1ms per event
- **Key Metrics**: Parsing time, memory usage, detection accuracy

### 5. Framework Integration
- **File**: framework_results.txt
- **Focus**: End-to-end performance
- **Target**: Production-ready performance
- **Key Metrics**: Overall throughput, latency, resource usage

### 6. Concurrent Performance
- **File**: concurrent_results.txt
- **Focus**: Performance under load
- **Target**: Linear scalability
- **Key Metrics**: Concurrent throughput, contention, stability

## Profiling Data

### CPU Profiles
- critical_cold_start_cpu.prof
- critical_routing_cpu.prof
- critical_middleware_cpu.prof

### Memory Profiles
- critical_cold_start_mem.prof
- critical_routing_mem.prof
- critical_middleware_mem.prof

## Analysis Commands

To analyze CPU profiles:
\`\`\`bash
go tool pprof critical_cold_start_cpu.prof
go tool pprof critical_routing_cpu.prof
go tool pprof critical_middleware_cpu.prof
\`\`\`

To analyze memory profiles:
\`\`\`bash
go tool pprof critical_cold_start_mem.prof
go tool pprof critical_routing_mem.prof
go tool pprof critical_middleware_mem.prof
\`\`\`

## Next Steps

1. **Analyze Results**: Review all benchmark outputs for performance bottlenecks
2. **Identify Optimizations**: Focus on areas not meeting target metrics
3. **Implement Improvements**: Apply optimizations based on profiling data
4. **Re-benchmark**: Validate improvements with follow-up benchmarks
5. **Document Findings**: Update performance documentation with results

## Performance Targets

- ✅ Cold Start: <15ms framework overhead
- ✅ Memory Usage: <5MB overhead
- ✅ Throughput: >50,000 req/sec
- ✅ Middleware: <0.1ms per middleware
- ✅ Event Parsing: <1ms per event

EOF

echo "✅ Benchmark execution completed!"
echo ""
echo "📊 Results Summary:"
echo "  - Results directory: $RESULTS_DIR"
echo "  - Summary report: $RESULTS_DIR/BENCHMARK_SUMMARY.md"
echo "  - CPU profiles: *.prof files"
echo "  - Detailed results: *_results.txt files"
echo ""
echo "🔍 Next steps:"
echo "  1. Review BENCHMARK_SUMMARY.md"
echo "  2. Analyze individual result files"
echo "  3. Use 'go tool pprof' for detailed profiling analysis"
echo "  4. Identify optimization opportunities"
echo ""
echo "🎯 Sprint 4 Performance Benchmarking Phase 1 Complete!" 