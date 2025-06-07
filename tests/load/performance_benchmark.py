#!/usr/bin/env python3
"""
Lesser Performance Benchmarking Tool
Measures and analyzes performance of the Lesser ActivityPub implementation
"""

import time
import json
import statistics
import concurrent.futures
from datetime import datetime, timezone
from typing import Dict, List, Any, Optional, Callable, Tuple
import requests
from dataclasses import dataclass, field
import sys

# Try to import optional dependencies
try:
    import matplotlib
    matplotlib.use('Agg')  # Use non-GUI backend
    import matplotlib.pyplot as plt
    HAS_PLOTTING = True
except ImportError:
    HAS_PLOTTING = False
    print("Warning: matplotlib not installed. Plotting features disabled.")

@dataclass
class BenchmarkResult:
    """Container for benchmark test results"""
    test_name: str
    requests_made: int
    total_time: float
    response_times: List[float] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)
    status_codes: Dict[int, int] = field(default_factory=dict)
    
    @property
    def success_rate(self) -> float:
        """Calculate success rate"""
        if self.requests_made == 0:
            return 0.0
        return (self.requests_made - len(self.errors)) / self.requests_made * 100
    
    @property
    def avg_response_time(self) -> float:
        """Calculate average response time"""
        if not self.response_times:
            return 0.0
        return statistics.mean(self.response_times)
    
    @property
    def p50_response_time(self) -> float:
        """Calculate 50th percentile response time"""
        if not self.response_times:
            return 0.0
        return statistics.median(self.response_times)
    
    @property
    def p95_response_time(self) -> float:
        """Calculate 95th percentile response time"""
        if not self.response_times:
            return 0.0
        sorted_times = sorted(self.response_times)
        idx = int(len(sorted_times) * 0.95)
        return sorted_times[min(idx, len(sorted_times) - 1)]
    
    @property
    def p99_response_time(self) -> float:
        """Calculate 99th percentile response time"""
        if not self.response_times:
            return 0.0
        sorted_times = sorted(self.response_times)
        idx = int(len(sorted_times) * 0.99)
        return sorted_times[min(idx, len(sorted_times) - 1)]
    
    @property
    def requests_per_second(self) -> float:
        """Calculate requests per second"""
        if self.total_time == 0:
            return 0.0
        return self.requests_made / self.total_time


class LesserPerformanceBenchmark:
    """Performance benchmarking tool for Lesser"""
    
    def __init__(self, base_url: str, auth_token: Optional[str] = None):
        self.base_url = base_url.rstrip('/')
        self.auth_token = auth_token
        self.headers = {
            'User-Agent': 'Lesser-Performance-Benchmark/1.0'
        }
        if auth_token:
            self.headers['Authorization'] = f'Bearer {auth_token}'
        
        self.results: List[BenchmarkResult] = []
    
    def _make_request(self, method: str, endpoint: str, 
                     data: Optional[Dict] = None) -> Tuple[float, int, Optional[str]]:
        """Make a single request and measure response time"""
        url = f"{self.base_url}{endpoint}"
        
        try:
            start_time = time.time()
            
            if method == 'GET':
                response = requests.get(url, headers=self.headers)
            elif method == 'POST':
                response = requests.post(url, json=data, headers=self.headers)
            elif method == 'PUT':
                response = requests.put(url, json=data, headers=self.headers)
            elif method == 'DELETE':
                response = requests.delete(url, headers=self.headers)
            else:
                raise ValueError(f"Unsupported method: {method}")
            
            elapsed_time = time.time() - start_time
            
            return elapsed_time, response.status_code, None
        except Exception as e:
            elapsed_time = time.time() - start_time
            return elapsed_time, 0, str(e)
    
    def benchmark_endpoint(self, name: str, method: str, endpoint: str,
                         iterations: int = 100, concurrent: bool = False,
                         max_workers: int = 10, data: Optional[Dict] = None) -> BenchmarkResult:
        """Benchmark a single endpoint"""
        print(f"\nBenchmarking: {name}")
        print(f"Method: {method} {endpoint}")
        print(f"Iterations: {iterations}, Concurrent: {concurrent}")
        
        result = BenchmarkResult(test_name=name, requests_made=iterations, total_time=0)
        
        start_time = time.time()
        
        if concurrent:
            # Concurrent requests
            with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
                futures = [
                    executor.submit(self._make_request, method, endpoint, data)
                    for _ in range(iterations)
                ]
                
                for future in concurrent.futures.as_completed(futures):
                    try:
                        elapsed, status_code, error = future.result()
                        result.response_times.append(elapsed)
                        
                        if error:
                            result.errors.append(error)
                        else:
                            result.status_codes[status_code] = result.status_codes.get(status_code, 0) + 1
                    except Exception as e:
                        result.errors.append(str(e))
        else:
            # Sequential requests
            for i in range(iterations):
                elapsed, status_code, error = self._make_request(method, endpoint, data)
                result.response_times.append(elapsed)
                
                if error:
                    result.errors.append(error)
                else:
                    result.status_codes[status_code] = result.status_codes.get(status_code, 0) + 1
                
                # Progress indicator
                if (i + 1) % 10 == 0:
                    print(f"  Progress: {i + 1}/{iterations}", end='\r')
        
        result.total_time = time.time() - start_time
        
        # Print summary
        print(f"\n  Completed: {iterations} requests in {result.total_time:.2f}s")
        print(f"  Success rate: {result.success_rate:.1f}%")
        print(f"  RPS: {result.requests_per_second:.1f}")
        print(f"  Response times - Avg: {result.avg_response_time*1000:.1f}ms, "
              f"P50: {result.p50_response_time*1000:.1f}ms, "
              f"P95: {result.p95_response_time*1000:.1f}ms, "
              f"P99: {result.p99_response_time*1000:.1f}ms")
        
        self.results.append(result)
        return result
    
    def run_standard_benchmarks(self) -> List[BenchmarkResult]:
        """Run a standard set of benchmarks"""
        benchmarks = [
            # Public endpoints
            ("Instance Info", "GET", "/api/v1/instance", 50, False),
            ("Public Timeline", "GET", "/api/v1/timelines/public", 100, False),
            
            # Authentication required endpoints (if token provided)
            ("Home Timeline", "GET", "/api/v1/timelines/home", 100, False),
            ("Notifications", "GET", "/api/v1/notifications", 50, False),
            ("Account Lookup", "GET", "/api/v1/accounts/verify_credentials", 100, False),
            
            # Search endpoints
            ("Account Search", "GET", "/api/v2/search?q=test&type=accounts", 50, False),
            
            # Concurrent tests
            ("Concurrent Timeline", "GET", "/api/v1/timelines/public", 100, True),
            ("Concurrent Instance", "GET", "/api/v1/instance", 100, True),
        ]
        
        for name, method, endpoint, iterations, concurrent in benchmarks:
            # Skip auth-required endpoints if no token
            if not self.auth_token and endpoint in ["/api/v1/timelines/home", 
                                                   "/api/v1/notifications",
                                                   "/api/v1/accounts/verify_credentials"]:
                print(f"\nSkipping {name} - requires authentication")
                continue
            
            self.benchmark_endpoint(name, method, endpoint, iterations, concurrent)
            time.sleep(1)  # Brief pause between benchmarks
        
        return self.results
    
    def generate_report(self, output_file: str = "benchmark_report.json"):
        """Generate a comprehensive benchmark report"""
        report = {
            "metadata": {
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "base_url": self.base_url,
                "total_tests": len(self.results),
                "total_requests": sum(r.requests_made for r in self.results),
                "total_time": sum(r.total_time for r in self.results)
            },
            "summary": {
                "overall_success_rate": sum(r.success_rate * r.requests_made for r in self.results) / 
                                      sum(r.requests_made for r in self.results) if self.results else 0,
                "average_rps": sum(r.requests_per_second for r in self.results) / len(self.results) 
                             if self.results else 0
            },
            "tests": []
        }
        
        for result in self.results:
            test_data = {
                "name": result.test_name,
                "requests": result.requests_made,
                "total_time": result.total_time,
                "success_rate": result.success_rate,
                "rps": result.requests_per_second,
                "response_times": {
                    "avg": result.avg_response_time * 1000,
                    "p50": result.p50_response_time * 1000,
                    "p95": result.p95_response_time * 1000,
                    "p99": result.p99_response_time * 1000,
                    "min": min(result.response_times) * 1000 if result.response_times else 0,
                    "max": max(result.response_times) * 1000 if result.response_times else 0
                },
                "status_codes": result.status_codes,
                "errors": len(result.errors)
            }
            report["tests"].append(test_data)
        
        with open(output_file, 'w') as f:
            json.dump(report, f, indent=2)
        
        print(f"\nReport saved to: {output_file}")
        return report
    
    def plot_results(self, output_dir: str = "."):
        """Generate visualization plots of benchmark results"""
        if not HAS_PLOTTING:
            print("Plotting disabled - matplotlib not installed")
            return
        
        if not self.results:
            print("No results to plot")
            return
        
        # Response time comparison
        plt.figure(figsize=(12, 6))
        
        test_names = [r.test_name for r in self.results]
        avg_times = [r.avg_response_time * 1000 for r in self.results]
        p95_times = [r.p95_response_time * 1000 for r in self.results]
        
        x = range(len(test_names))
        width = 0.35
        
        plt.bar([i - width/2 for i in x], avg_times, width, label='Average')
        plt.bar([i + width/2 for i in x], p95_times, width, label='P95')
        
        plt.xlabel('Test')
        plt.ylabel('Response Time (ms)')
        plt.title('Response Time Comparison')
        plt.xticks(x, test_names, rotation=45, ha='right')
        plt.legend()
        plt.tight_layout()
        plt.savefig(f"{output_dir}/response_times.png")
        plt.close()
        
        # Requests per second
        plt.figure(figsize=(10, 6))
        rps_values = [r.requests_per_second for r in self.results]
        plt.bar(test_names, rps_values)
        plt.xlabel('Test')
        plt.ylabel('Requests per Second')
        plt.title('Throughput Comparison')
        plt.xticks(rotation=45, ha='right')
        plt.tight_layout()
        plt.savefig(f"{output_dir}/throughput.png")
        plt.close()
        
        print(f"\nPlots saved to: {output_dir}/")


def main():
    """Example usage of the performance benchmark tool"""
    import argparse
    
    parser = argparse.ArgumentParser(description='Lesser Performance Benchmark Tool')
    parser.add_argument('--url', default='http://localhost:3000', 
                       help='Base URL of the Lesser instance')
    parser.add_argument('--token', help='Authentication token')
    parser.add_argument('--output', default='benchmark_report.json',
                       help='Output file for the report')
    parser.add_argument('--plot', action='store_true',
                       help='Generate visualization plots')
    
    args = parser.parse_args()
    
    print(f"Lesser Performance Benchmark")
    print(f"Target: {args.url}")
    print("=" * 50)
    
    benchmark = LesserPerformanceBenchmark(args.url, args.token)
    
    # Run standard benchmarks
    benchmark.run_standard_benchmarks()
    
    # Generate report
    report = benchmark.generate_report(args.output)
    
    # Generate plots if requested
    if args.plot:
        benchmark.plot_results()
    
    # Print summary
    print("\n" + "=" * 50)
    print("BENCHMARK SUMMARY")
    print("=" * 50)
    print(f"Total tests run: {report['metadata']['total_tests']}")
    print(f"Total requests made: {report['metadata']['total_requests']}")
    print(f"Total time: {report['metadata']['total_time']:.2f}s")
    print(f"Overall success rate: {report['summary']['overall_success_rate']:.1f}%")
    print(f"Average RPS: {report['summary']['average_rps']:.1f}")


if __name__ == "__main__":
    main()
