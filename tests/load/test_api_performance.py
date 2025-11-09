#!/usr/bin/env python3
"""
Performance Benchmark for Lesser
Measures response times and throughput for key endpoints
"""

import time
import statistics
import concurrent.futures
import requests
from typing import List, Dict, Callable
import json
import logging
from datetime import datetime

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)


class PerformanceBenchmark:
    """Run performance benchmarks against Lesser instance"""
    
    def __init__(self, instance_url: str, access_token: str = None):
        self.instance_url = instance_url.rstrip('/')
        self.access_token = access_token
        self.session = requests.Session()
        if access_token:
            self.session.headers['Authorization'] = f'Bearer {access_token}'
            
    def measure_endpoint(self, method: str, endpoint: str, iterations: int = 100, **kwargs) -> Dict:
        """Measure performance of a single endpoint"""
        url = f"{self.instance_url}{endpoint}"
        timings = []
        errors = 0
        
        logger.info(f"Benchmarking {method} {endpoint} ({iterations} iterations)")
        
        for i in range(iterations):
            try:
                start = time.perf_counter()
                response = self.session.request(method, url, **kwargs)
                elapsed = (time.perf_counter() - start) * 1000  # Convert to ms
                
                if response.status_code >= 400:
                    errors += 1
                    logger.debug(f"Request {i+1} failed: {response.status_code}")
                else:
                    timings.append(elapsed)
                    
                # Small delay to avoid overwhelming the server
                time.sleep(0.01)
                
            except Exception as e:
                errors += 1
                logger.debug(f"Request {i+1} error: {e}")
                
        if not timings:
            return {
                'endpoint': endpoint,
                'method': method,
                'error': 'All requests failed',
                'error_rate': 1.0
            }
            
        # Calculate statistics
        timings.sort()
        
        return {
            'endpoint': endpoint,
            'method': method,
            'iterations': iterations,
            'successful': len(timings),
            'errors': errors,
            'error_rate': errors / iterations,
            'min': timings[0],
            'max': timings[-1],
            'mean': statistics.mean(timings),
            'median': statistics.median(timings),
            'stdev': statistics.stdev(timings) if len(timings) > 1 else 0,
            'p50': timings[int(len(timings) * 0.50)],
            'p90': timings[int(len(timings) * 0.90)],
            'p95': timings[int(len(timings) * 0.95)],
            'p99': timings[int(len(timings) * 0.99)] if len(timings) > 100 else timings[-1],
        }
        
    def concurrent_load_test(self, endpoint: str, concurrent_users: int = 10, 
                           duration_seconds: int = 30) -> Dict:
        """Test endpoint under concurrent load"""
        url = f"{self.instance_url}{endpoint}"
        results = []
        start_time = time.time()
        
        logger.info(f"Load testing {endpoint} with {concurrent_users} concurrent users for {duration_seconds}s")
        
        def make_request():
            timings = []
            errors = 0
            
            while time.time() - start_time < duration_seconds:
                try:
                    req_start = time.perf_counter()
                    response = self.session.get(url)
                    elapsed = (time.perf_counter() - req_start) * 1000
                    
                    if response.status_code >= 400:
                        errors += 1
                    else:
                        timings.append(elapsed)
                        
                except Exception:
                    errors += 1
                    
                time.sleep(0.1)  # Small delay between requests
                
            return {'timings': timings, 'errors': errors}
            
        # Run concurrent requests
        with concurrent.futures.ThreadPoolExecutor(max_workers=concurrent_users) as executor:
            futures = [executor.submit(make_request) for _ in range(concurrent_users)]
            
            for future in concurrent.futures.as_completed(futures):
                result = future.result()
                results.extend(result['timings'])
                
        total_requests = len(results) + sum(r['errors'] for r in results)
        actual_duration = time.time() - start_time
        
        if not results:
            return {
                'endpoint': endpoint,
                'error': 'No successful requests',
                'total_requests': total_requests
            }
            
        results.sort()
        
        return {
            'endpoint': endpoint,
            'concurrent_users': concurrent_users,
            'duration': actual_duration,
            'total_requests': total_requests,
            'successful_requests': len(results),
            'requests_per_second': len(results) / actual_duration,
            'min': results[0],
            'max': results[-1],
            'mean': statistics.mean(results),
            'median': statistics.median(results),
            'p50': results[int(len(results) * 0.50)],
            'p90': results[int(len(results) * 0.90)],
            'p95': results[int(len(results) * 0.95)],
            'p99': results[int(len(results) * 0.99)] if len(results) > 100 else results[-1],
        }
        
    def run_comprehensive_benchmark(self) -> Dict:
        """Run comprehensive performance benchmarks"""
        results = {
            'instance': self.instance_url,
            'timestamp': datetime.utcnow().isoformat(),
            'benchmarks': {}
        }
        
        # Define endpoints to test
        endpoints = [
            # Public endpoints (no auth required)
            ('GET', '/api/v1/instance', False),
            ('GET', '/.well-known/nodeinfo', False),
            ('GET', '/.well-known/webfinger?resource=acct:admin@lesser.example.com', False),
        ]
        
        # Add authenticated endpoints if token provided
        if self.access_token:
            endpoints.extend([
                ('GET', '/api/v1/accounts/verify_credentials', True),
                ('GET', '/api/v1/timelines/home?limit=20', True),
                ('GET', '/api/v1/timelines/public?limit=20', True),
                ('GET', '/api/v1/notifications?limit=20', True),
                ('GET', '/api/v2/search?q=test', True),
            ])
            
        # Single-user benchmarks
        logger.info("\n=== Single User Benchmarks ===")
        for method, endpoint, _ in endpoints:
            result = self.measure_endpoint(method, endpoint, iterations=50)
            results['benchmarks'][endpoint] = result
            
            if 'error' not in result:
                logger.info(f"{endpoint}:")
                logger.info(f"  Mean: {result['mean']:.2f}ms")
                logger.info(f"  P95: {result['p95']:.2f}ms")
                logger.info(f"  P99: {result['p99']:.2f}ms")
            else:
                logger.error(f"{endpoint}: {result['error']}")
                
        # Concurrent load tests
        if self.access_token:
            logger.info("\n=== Concurrent Load Tests ===")
            load_endpoints = [
                '/api/v1/timelines/home?limit=20',
                '/api/v1/timelines/public?limit=20',
            ]
            
            for endpoint in load_endpoints:
                result = self.concurrent_load_test(endpoint, concurrent_users=10, duration_seconds=30)
                results['benchmarks'][f"{endpoint}_concurrent"] = result
                
                if 'error' not in result:
                    logger.info(f"{endpoint} (concurrent):")
                    logger.info(f"  RPS: {result['requests_per_second']:.2f}")
                    logger.info(f"  Mean: {result['mean']:.2f}ms")
                    logger.info(f"  P95: {result['p95']:.2f}ms")
                    
        # Cost analysis
        if self.access_token:
            logger.info("\n=== Cost Analysis ===")
            response = self.session.get(f"{self.instance_url}/api/v1/timelines/home")
            if 'X-Cost-Total-Cents' in response.headers:
                cost_cents = float(response.headers['X-Cost-Total-Cents'])
                logger.info(f"Sample request cost: ${cost_cents:.6f}")
                logger.info(f"Estimated cost per 1M requests: ${cost_cents * 1000000:.2f}")
                
        return results
        
    def generate_report(self, results: Dict, output_file: str = None):
        """Generate a performance report"""
        report = []
        report.append("Lesser Performance Benchmark Report")
        report.append("=" * 50)
        report.append(f"Instance: {results['instance']}")
        report.append(f"Timestamp: {results['timestamp']}")
        report.append("")
        
        # Summary table
        report.append("Performance Summary")
        report.append("-" * 50)
        report.append(f"{'Endpoint':<40} {'Mean':>8} {'P95':>8} {'P99':>8}")
        report.append("-" * 50)
        
        for endpoint, data in results['benchmarks'].items():
            if 'error' not in data and not endpoint.endswith('_concurrent'):
                report.append(
                    f"{endpoint[:40]:<40} "
                    f"{data['mean']:>7.1f}ms "
                    f"{data['p95']:>7.1f}ms "
                    f"{data.get('p99', data['p95']):>7.1f}ms"
                )
                
        report.append("")
        
        # Performance assessment
        report.append("Performance Assessment")
        report.append("-" * 50)
        
        excellent = []
        good = []
        needs_improvement = []
        
        for endpoint, data in results['benchmarks'].items():
            if 'error' not in data and not endpoint.endswith('_concurrent'):
                if data['p95'] < 100:
                    excellent.append(endpoint)
                elif data['p95'] < 300:
                    good.append(endpoint)
                else:
                    needs_improvement.append(endpoint)
                    
        if excellent:
            report.append(f"✅ Excellent (<100ms P95): {len(excellent)} endpoints")
            
        if good:
            report.append(f"👍 Good (100-300ms P95): {len(good)} endpoints")
            
        if needs_improvement:
            report.append(f"⚠️  Needs Improvement (>300ms P95): {len(needs_improvement)} endpoints")
            for endpoint in needs_improvement:
                report.append(f"   - {endpoint}")
                
        # Write report
        report_text = "\n".join(report)
        
        if output_file:
            with open(output_file, 'w') as f:
                f.write(report_text)
                # Also write raw JSON data
                f.write("\n\n" + "="*50 + "\n")
                f.write("Raw Data (JSON)\n")
                f.write("="*50 + "\n")
                f.write(json.dumps(results, indent=2))
        else:
            print(report_text)
            
        return report_text


def main():
    """Main benchmark runner"""
    import argparse
    
    parser = argparse.ArgumentParser(description='Performance Benchmark for Lesser')
    parser.add_argument('instance_url', help='Lesser instance URL')
    parser.add_argument('--token', help='Access token for authenticated tests')
    parser.add_argument('--output', help='Output file for report')
    
    args = parser.parse_args()
    
    benchmark = PerformanceBenchmark(args.instance_url, args.token)
    results = benchmark.run_comprehensive_benchmark()
    benchmark.generate_report(results, args.output)
    
    # Return non-zero if any endpoints are slow
    slow_endpoints = sum(1 for _, data in results['benchmarks'].items() 
                        if 'error' not in data and data.get('p95', 0) > 500)
    return slow_endpoints


if __name__ == '__main__':
    import sys
    sys.exit(main()) 