#!/usr/bin/env python3
"""
Analyze test results from Lesser API tests
"""

import json
import sys
import os
from datetime import datetime
from collections import defaultdict

def load_results(filename):
    """Load test results from JSON file"""
    try:
        with open(filename, 'r') as f:
            return json.load(f)
    except Exception as e:
        print(f"Error loading {filename}: {e}")
        return None

def analyze_results(results):
    """Analyze test results and generate insights"""
    if not results or 'results' not in results:
        return None
    
    # Group by status
    by_status = defaultdict(list)
    for test in results['results']:
        by_status[test['status']].append(test)
    
    # Group by category (extract from test name)
    by_category = defaultdict(lambda: defaultdict(int))
    for test in results['results']:
        # Extract category from test name
        test_name = test['test']
        if ' /api/' in test_name:
            # API endpoint test
            parts = test_name.split(' /api/')
            method = parts[0]
            endpoint = '/api/' + parts[1]
            
            # Determine category
            if '/statuses' in endpoint:
                category = 'Statuses'
            elif '/accounts' in endpoint:
                category = 'Accounts'
            elif '/timelines' in endpoint:
                category = 'Timelines'
            elif '/lists' in endpoint:
                category = 'Lists'
            elif '/instance' in endpoint:
                category = 'Instance'
            elif '/media' in endpoint:
                category = 'Media'
            elif '/notifications' in endpoint:
                category = 'Notifications'
            elif '/search' in endpoint:
                category = 'Search'
            elif '/trends' in endpoint:
                category = 'Trends'
            elif '/filters' in endpoint:
                category = 'Filters'
            elif '/preferences' in endpoint:
                category = 'Preferences'
            elif '/moderation' in endpoint:
                category = 'Moderation'
            elif '/ai' in endpoint:
                category = 'AI Features'
            elif '/admin' in endpoint:
                category = 'Admin'
            else:
                category = 'Other'
        else:
            # Non-API test
            if 'Instance' in test_name:
                category = 'Instance'
            elif 'Auth' in test_name:
                category = 'Authentication'
            elif 'App' in test_name:
                category = 'App Registration'
            else:
                category = 'Other'
        
        by_category[category][test['status']] += 1
    
    return {
        'by_status': dict(by_status),
        'by_category': dict(by_category),
        'summary': results.get('summary', {})
    }

def print_analysis(results, analysis):
    """Print analysis results"""
    print("\n" + "=" * 80)
    print("📊 LESSER API TEST ANALYSIS")
    print("=" * 80)
    
    # Basic info
    print(f"\nInstance: {results.get('instance_url', 'Unknown')}")
    print(f"Test Date: {results.get('timestamp', 'Unknown')}")
    
    # Summary stats
    summary = analysis['summary']
    print(f"\n📈 Overall Results:")
    print(f"  Total Tests: {summary.get('total', 0)}")
    print(f"  ✅ Passed: {summary.get('passed', 0)}")
    print(f"  ❌ Failed: {summary.get('failed', 0)}")
    print(f"  ⚠️ Skipped: {summary.get('skipped', 0)}")
    if summary.get('total', 0) > 0:
        print(f"  Success Rate: {summary.get('success_rate', 0):.1f}%")
    
    # By category
    print(f"\n📂 Results by Category:")
    categories = sorted(analysis['by_category'].keys())
    for category in categories:
        stats = analysis['by_category'][category]
        total = sum(stats.values())
        passed = stats.get('PASS', 0)
        failed = stats.get('FAIL', 0)
        skipped = stats.get('SKIP', 0)
        
        if total > 0:
            success_rate = (passed / (passed + failed) * 100) if (passed + failed) > 0 else 0
            print(f"\n  {category}:")
            print(f"    Total: {total} | ✅ {passed} | ❌ {failed} | ⚠️ {skipped}")
            if passed + failed > 0:
                print(f"    Success Rate: {success_rate:.1f}%")
    
    # Failed tests details
    failed_tests = analysis['by_status'].get('FAIL', [])
    if failed_tests:
        print(f"\n❌ Failed Tests ({len(failed_tests)}):")
        for test in failed_tests:
            print(f"  - {test['test']}")
            if test.get('details'):
                print(f"    Details: {test['details']}")
    
    # Skipped tests summary
    skipped_tests = analysis['by_status'].get('SKIP', [])
    if skipped_tests:
        print(f"\n⚠️ Skipped Tests ({len(skipped_tests)}):")
        skip_reasons = defaultdict(int)
        for test in skipped_tests:
            reason = test.get('details', 'Unknown reason')
            skip_reasons[reason] += 1
        
        for reason, count in skip_reasons.items():
            print(f"  - {reason}: {count} tests")
    
    # Recommendations
    print("\n💡 Recommendations:")
    if failed_tests:
        print("  - Investigate failed tests and check error logs")
        print("  - Verify API endpoints are correctly implemented")
        print("  - Check authentication and permissions")
    
    if len(skipped_tests) > 10:
        print("  - Many tests were skipped - check test prerequisites")
        print("  - Ensure test user has proper permissions")
    
    if summary.get('success_rate', 0) < 80:
        print("  - Success rate is below 80% - prioritize fixing failures")
    elif summary.get('success_rate', 0) >= 95:
        print("  - Excellent test coverage! Consider adding edge cases")
    
    print("\n" + "=" * 80)

def main():
    """Main entry point"""
    if len(sys.argv) < 2:
        # Find the most recent results file
        result_files = [f for f in os.listdir('.') if f.startswith('api-test-results-') and f.endswith('.json')]
        if not result_files:
            print("Usage: python analyze_test_results.py <results.json>")
            print("Or run from a directory containing api-test-results-*.json files")
            sys.exit(1)
        
        # Use the most recent file
        result_files.sort()
        filename = result_files[-1]
        print(f"Using most recent results: {filename}")
    else:
        filename = sys.argv[1]
    
    # Load results
    results = load_results(filename)
    if not results:
        print(f"Failed to load results from {filename}")
        sys.exit(1)
    
    # Analyze
    analysis = analyze_results(results)
    if not analysis:
        print("Failed to analyze results")
        sys.exit(1)
    
    # Print analysis
    print_analysis(results, analysis)

if __name__ == '__main__':
    main() 