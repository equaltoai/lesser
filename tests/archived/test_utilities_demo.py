#!/usr/bin/env python3
"""
Lesser Test Utilities Demo
Demonstrates usage of test data generator, federation harness, and performance benchmarks
"""

import asyncio
import json
from test_data_generator import LesserTestDataGenerator
from federation_test_harness import FederationTestHarness


def demo_test_data_generator():
    """Demonstrate test data generation"""
    print("\n" + "="*60)
    print("TEST DATA GENERATOR DEMO")
    print("="*60)
    
    # Initialize generator
    generator = LesserTestDataGenerator("https://lesser.example.com")
    
    # Generate various types of data
    print("\n1. Generating sample actors...")
    actors = []
    for i in range(5):
        actor = generator.generate_actor()
        actors.append(actor)
        print(f"   - @{actor['preferredUsername']} ({actor['name']})")
    
    print("\n2. Generating sample notes...")
    for i in range(5):
        note = generator.generate_note(content_length="short")
        print(f"   - Note {i+1}: {note['content'][:50]}...")
    
    print("\n3. Generating a conversation thread...")
    thread = generator.generate_conversation_thread(depth=4, participants=3)
    print(f"   - Thread with {len(thread)} posts")
    for i, post in enumerate(thread):
        indent = "     " if post.get('inReplyTo') else "   "
        print(f"{indent}- Post {i+1} by @{post['attributedTo'].split('/')[-1]}")
    
    print("\n4. Generating follow network...")
    network = generator.generate_follow_network(actors_count=8, follow_probability=0.4)
    print(f"   - Network stats: {network['stats']}")
    
    print("\n5. Generating timeline data...")
    timeline = generator.generate_timeline_data(days=3, posts_per_day=10)
    print(f"   - Generated {len(timeline)} timeline activities over 3 days")
    
    # Export data
    generator.export_test_data("demo_test_data.json")
    print(f"\n✓ Test data exported to demo_test_data.json")


async def demo_federation_harness():
    """Demonstrate federation testing"""
    print("\n" + "="*60)
    print("FEDERATION TEST HARNESS DEMO")
    print("="*60)
    
    # Note: This is a demo with a fake URL - replace with your actual instance
    target_url = "https://lesser.example.com"
    
    async with FederationTestHarness(target_url) as harness:
        print(f"\nTesting federation against: {target_url}")
        print("(Note: This is a demo - actual federation requires a running instance)")
        
        # Create mock instances
        print("\n1. Creating mock federation instances...")
        mastodon = harness.create_mock_instance("mastodon.social")
        pixelfed = harness.create_mock_instance("pixelfed.social")
        
        # Create actors
        print("\n2. Creating mock actors...")
        harness.create_mock_actor(mastodon, "alice")
        harness.create_mock_actor(mastodon, "bob")
        harness.create_mock_actor(pixelfed, "photographer")
        
        print("\n3. Simulating federation activities...")
        print("   - Follow: alice@mastodon.social -> testuser@lesser.example.com")
        print("   - Note: bob@mastodon.social mentions testuser")
        print("   - Like: photographer@pixelfed.social likes a post")
        
        # In a real test, these would actually send activities
        # For demo purposes, we're just showing the structure
        
        harness.export_results("demo_federation_results.json")
        print("\n✓ Federation test results exported to demo_federation_results.json")


def demo_performance_benchmark():
    """Demonstrate performance benchmarking"""
    print("\n" + "="*60)
    print("PERFORMANCE BENCHMARK DEMO")
    print("="*60)
    
    # Note: This is a demo - replace with your actual instance URL
    base_url = "http://localhost:3000"
    
    print(f"\nBenchmarking target: {base_url}")
    print("(Note: This is a demo - actual benchmarking requires a running instance)")
    
    # Initialize benchmark
    
    # Demonstrate benchmark configuration
    print("\n1. Available benchmark types:")
    print("   - Endpoint latency testing")
    print("   - Concurrent request handling")
    print("   - Throughput measurement")
    print("   - Response time percentiles (P50, P95, P99)")
    
    print("\n2. Standard benchmark suite includes:")
    print("   - Instance info endpoint")
    print("   - Public timeline")
    print("   - Account search")
    print("   - Authenticated endpoints (with token)")
    
    print("\n3. Benchmark output includes:")
    print("   - JSON report with detailed metrics")
    print("   - Optional visualization plots")
    print("   - Success rates and error tracking")
    
    # Create a sample report structure
    sample_report = {
        "metadata": {
            "timestamp": "2024-01-15T10:00:00Z",
            "base_url": base_url,
            "total_tests": 8,
            "total_requests": 600
        },
        "summary": {
            "overall_success_rate": 99.5,
            "average_rps": 150.2
        },
        "tests": [
            {
                "name": "Instance Info",
                "requests": 50,
                "success_rate": 100.0,
                "rps": 200.5,
                "response_times": {
                    "avg": 5.2,
                    "p50": 4.8,
                    "p95": 8.1,
                    "p99": 12.3
                }
            }
        ]
    }
    
    with open("demo_benchmark_report.json", "w") as f:
        json.dump(sample_report, f, indent=2)
    
    print("\n✓ Sample benchmark report saved to demo_benchmark_report.json")


def main():
    """Run all demos"""
    print("\n🚀 LESSER TEST UTILITIES DEMONSTRATION")
    print("=====================================")
    
    # Run test data generator demo
    demo_test_data_generator()
    
    # Run federation harness demo
    asyncio.run(demo_federation_harness())
    
    # Run performance benchmark demo
    demo_performance_benchmark()
    
    print("\n" + "="*60)
    print("DEMO COMPLETE!")
    print("="*60)
    print("\nCreated files:")
    print("  - demo_test_data.json         : Sample ActivityPub test data")
    print("  - demo_federation_results.json : Federation test results")
    print("  - demo_benchmark_report.json   : Performance benchmark report")
    
    print("\nNext steps:")
    print("1. Use test_data_generator.py to create realistic test data")
    print("2. Use federation_test_harness.py to test federation flows")
    print("3. Use performance_benchmark.py to measure performance")
    
    print("\nExample commands:")
    print("  python3 test_data_generator.py")
    print("  python3 federation_test_harness.py")
    print("  python3 performance_benchmark.py --url http://localhost:3000 --plot")


if __name__ == "__main__":
    main() 
