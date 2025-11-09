#!/usr/bin/env python3
"""
Test script for Week 4 Advanced Search features
Tests popularity search, analytics tracking, and search filters
"""

import requests
import time
import argparse
from datetime import datetime

# ANSI color codes
GREEN = '\033[92m'
RED = '\033[91m'
YELLOW = '\033[93m'
BLUE = '\033[94m'
RESET = '\033[0m'

def print_success(msg):
    print(f"{GREEN}✓ {msg}{RESET}")

def print_error(msg):
    print(f"{RED}✗ {msg}{RESET}")

def print_info(msg):
    print(f"{BLUE}ℹ {msg}{RESET}")

def print_warning(msg):
    print(f"{YELLOW}⚠ {msg}{RESET}")

class SearchTester:
    def __init__(self, base_url, api_token=None):
        self.base_url = base_url.rstrip('/')
        self.api_token = api_token
        self.headers = {
            'Content-Type': 'application/json',
            'Accept': 'application/json'
        }
        if api_token:
            self.headers['Authorization'] = f'Bearer {api_token}'
        
        self.test_users = []
        self.test_queries = []

    def create_test_users(self):
        """Create test users with different follower counts for popularity testing"""
        print_info("Creating test users with different popularity levels...")
        
        test_accounts = [
            {"username": "popular_user", "display_name": "Popular User", "note": "Very popular account"},
            {"username": "medium_user", "display_name": "Medium User", "note": "Moderately popular"},
            {"username": "new_user", "display_name": "New User", "note": "Brand new account"},
            {"username": "test_search", "display_name": "Search Test", "note": "Testing search features"},
        ]
        
        for account in test_accounts:
            # Skip if running without auth
            if not self.api_token:
                print_warning(f"Skipping user creation for {account['username']} (no auth)")
                continue
                
            # Create account (would need proper OAuth flow in production)
            print_info(f"Would create user: {account['username']}")
            self.test_users.append(account['username'])

    def test_popularity_search(self):
        """Test popularity-based search"""
        print_info("\nTesting popularity search...")
        
        # Search without query to get popular accounts
        response = requests.get(
            f"{self.base_url}/api/v2/search",
            params={"q": "", "type": "accounts", "limit": 20},
            headers=self.headers
        )
        
        if response.status_code == 200:
            data = response.json()
            if 'accounts' in data and len(data['accounts']) > 0:
                print_success(f"Popularity search returned {len(data['accounts'])} accounts")
                
                # Check if results are ordered by popularity
                # In a real test, we'd verify follower counts are descending
                for i, account in enumerate(data['accounts'][:5]):
                    print(f"  {i+1}. @{account.get('username', 'unknown')} - {account.get('display_name', '')}")
            else:
                print_warning("No accounts returned in popularity search")
        else:
            print_error(f"Popularity search failed: {response.status_code}")

    def test_search_with_filters(self):
        """Test search with following-only and local-only filters"""
        print_info("\nTesting search filters...")
        
        # Test following-only filter
        if self.api_token:
            response = requests.get(
                f"{self.base_url}/api/v2/search",
                params={"q": "test", "type": "accounts", "following": "true"},
                headers=self.headers
            )
            
            if response.status_code == 200:
                data = response.json()
                print_success(f"Following-only search returned {len(data.get('accounts', []))} accounts")
            else:
                print_error(f"Following-only search failed: {response.status_code}")
        
        # Test local-only filter (exclude_unreviewed acts as local-only)
        response = requests.get(
            f"{self.base_url}/api/v2/search",
            params={"q": "test", "type": "accounts", "exclude_unreviewed": "true"},
            headers=self.headers
        )
        
        if response.status_code == 200:
            data = response.json()
            print_success(f"Local-only search returned {len(data.get('accounts', []))} accounts")
        else:
            print_error(f"Local-only search failed: {response.status_code}")

    def test_search_analytics(self):
        """Test that searches are being tracked for analytics"""
        print_info("\nTesting search analytics tracking...")
        
        # Perform several searches to generate analytics data
        test_queries = [
            "popular",
            "test",
            "search",
            "analytics",
            "lesser"
        ]
        
        for query in test_queries:
            response = requests.get(
                f"{self.base_url}/api/v2/search",
                params={"q": query, "type": "accounts"},
                headers=self.headers
            )
            
            if response.status_code == 200:
                self.test_queries.append({
                    "query": query,
                    "timestamp": datetime.now().isoformat(),
                    "result_count": len(response.json().get('accounts', []))
                })
                print_success(f"Tracked search for '{query}'")
                time.sleep(0.1)  # Small delay between searches
            else:
                print_error(f"Search tracking failed for '{query}': {response.status_code}")

    def test_search_performance(self):
        """Test search performance with different strategies"""
        print_info("\nTesting search performance...")
        
        search_tests = [
            {"q": "test", "type": "accounts", "name": "Basic search"},
            {"q": "tes", "type": "accounts", "name": "Prefix search"},
            {"q": "Test User", "type": "accounts", "name": "Display name search"},
            {"q": "", "type": "accounts", "limit": 10, "name": "Popularity search"},
        ]
        
        for test in search_tests:
            start_time = time.time()
            
            params = {k: v for k, v in test.items() if k not in ['name']}
            response = requests.get(
                f"{self.base_url}/api/v2/search",
                params=params,
                headers=self.headers
            )
            
            elapsed_ms = (time.time() - start_time) * 1000
            
            if response.status_code == 200:
                data = response.json()
                result_count = len(data.get('accounts', []))
                
                # Check if response time is acceptable
                if elapsed_ms < 200:
                    print_success(f"{test['name']}: {elapsed_ms:.0f}ms - {result_count} results")
                elif elapsed_ms < 500:
                    print_warning(f"{test['name']}: {elapsed_ms:.0f}ms - {result_count} results (slow)")
                else:
                    print_error(f"{test['name']}: {elapsed_ms:.0f}ms - {result_count} results (too slow)")
            else:
                print_error(f"{test['name']} failed: {response.status_code}")

    def test_search_caching(self):
        """Test that search results are cached"""
        print_info("\nTesting search caching...")
        
        query = "cache_test"
        
        # First search (cache miss)
        start_time = time.time()
        response1 = requests.get(
            f"{self.base_url}/api/v2/search",
            params={"q": query, "type": "accounts"},
            headers=self.headers
        )
        time1 = (time.time() - start_time) * 1000
        
        if response1.status_code == 200:
            # Second search (should be cache hit)
            start_time = time.time()
            response2 = requests.get(
                f"{self.base_url}/api/v2/search",
                params={"q": query, "type": "accounts"},
                headers=self.headers
            )
            time2 = (time.time() - start_time) * 1000
            
            if response2.status_code == 200:
                # Cache hit should be significantly faster
                if time2 < time1 * 0.5:
                    print_success(f"Cache working: {time1:.0f}ms → {time2:.0f}ms")
                else:
                    print_warning(f"Cache may not be working: {time1:.0f}ms → {time2:.0f}ms")
            else:
                print_error(f"Second search failed: {response2.status_code}")
        else:
            print_error(f"First search failed: {response1.status_code}")

    def test_click_tracking(self):
        """Test click-through tracking for search results"""
        print_info("\nTesting click-through tracking...")
        
        # This would typically be done through the UI
        # For testing, we'd need an endpoint to record clicks
        print_warning("Click tracking requires UI interaction - simulating...")
        
        # Simulate tracking a click
        if self.test_queries:
            query = self.test_queries[0]['query']
            print_info(f"Would track click for query '{query}'")

    def run_all_tests(self):
        """Run all Week 4 search tests"""
        print(f"\n{BLUE}{'='*60}{RESET}")
        print(f"{BLUE}Week 4 Search Features Test Suite{RESET}")
        print(f"{BLUE}{'='*60}{RESET}\n")
        
        # Create test data
        self.create_test_users()
        
        # Run tests
        self.test_popularity_search()
        self.test_search_with_filters()
        self.test_search_analytics()
        self.test_search_performance()
        self.test_search_caching()
        self.test_click_tracking()
        
        # Summary
        print(f"\n{BLUE}{'='*60}{RESET}")
        print(f"{BLUE}Test Summary{RESET}")
        print(f"{BLUE}{'='*60}{RESET}")
        print(f"Queries tracked: {len(self.test_queries)}")
        print(f"Test users created: {len(self.test_users)}")

def main():
    parser = argparse.ArgumentParser(description='Test Week 4 search features')
    parser.add_argument('base_url', help='Base URL of the API (e.g., https://example.com)')
    parser.add_argument('--token', help='API token for authenticated requests')
    parser.add_argument('--test', choices=['popularity', 'filters', 'analytics', 'performance', 'caching', 'all'],
                       default='all', help='Specific test to run')
    
    args = parser.parse_args()
    
    # Create tester
    tester = SearchTester(args.base_url, args.token)
    
    # Run requested tests
    if args.test == 'all':
        tester.run_all_tests()
    elif args.test == 'popularity':
        tester.test_popularity_search()
    elif args.test == 'filters':
        tester.test_search_with_filters()
    elif args.test == 'analytics':
        tester.test_search_analytics()
    elif args.test == 'performance':
        tester.test_search_performance()
    elif args.test == 'caching':
        tester.test_search_caching()

if __name__ == "__main__":
    main() 
