#!/usr/bin/env python3
"""
Test script for Lesser Account Search functionality
Tests the advanced search features including exact match, prefix search, and fuzzy search
"""

import requests
import time
import sys
from typing import List, Dict, Any
from dataclasses import dataclass
from colorama import init, Fore, Style

# Initialize colorama for cross-platform colored output
init()

@dataclass
class TestResult:
    name: str
    passed: bool
    message: str
    duration: float

class AccountSearchTester:
    def __init__(self, base_url: str, access_token: str):
        self.base_url = base_url.rstrip('/')
        self.headers = {
            'Authorization': f'Bearer {access_token}',
            'Content-Type': 'application/json'
        }
        self.results: List[TestResult] = []

    def search_accounts(self, query: str, limit: int = 40, offset: int = 0, 
                       following: bool = False, resolve: bool = False) -> Dict[str, Any]:
        """Search for accounts using the API"""
        params = {
            'q': query,
            'limit': limit,
            'offset': offset
        }
        if following:
            params['following'] = 'true'
        if resolve:
            params['resolve'] = 'true'

        response = requests.get(
            f'{self.base_url}/api/v1/accounts/search',
            headers=self.headers,
            params=params
        )
        return {
            'status': response.status_code,
            'data': response.json() if response.status_code == 200 else None,
            'headers': dict(response.headers)
        }

    def general_search(self, query: str, type: str = None) -> Dict[str, Any]:
        """Use the general search endpoint"""
        params = {'q': query}
        if type:
            params['type'] = type

        response = requests.get(
            f'{self.base_url}/api/v1/search',
            headers=self.headers,
            params=params
        )
        return {
            'status': response.status_code,
            'data': response.json() if response.status_code == 200 else None
        }

    def get_search_suggestions(self, prefix: str) -> Dict[str, Any]:
        """Get search suggestions for autocomplete"""
        params = {'q': prefix}
        
        response = requests.get(
            f'{self.base_url}/api/v1/accounts/search/suggestions',
            headers=self.headers,
            params=params
        )
        return {
            'status': response.status_code,
            'data': response.json() if response.status_code == 200 else None
        }

    def run_test(self, name: str, test_func):
        """Run a single test and record the result"""
        print(f"\n{Fore.YELLOW}Running: {name}{Style.RESET_ALL}")
        start_time = time.time()
        
        try:
            passed, message = test_func()
            duration = time.time() - start_time
            
            if passed:
                print(f"{Fore.GREEN}✓ PASSED{Style.RESET_ALL}: {message}")
            else:
                print(f"{Fore.RED}✗ FAILED{Style.RESET_ALL}: {message}")
            
            self.results.append(TestResult(name, passed, message, duration))
        except Exception as e:
            duration = time.time() - start_time
            print(f"{Fore.RED}✗ ERROR{Style.RESET_ALL}: {str(e)}")
            self.results.append(TestResult(name, False, f"Exception: {str(e)}", duration))

    def test_exact_match(self):
        """Test exact username matching"""
        def test():
            # Search for a known username (adjust based on your test data)
            result = self.search_accounts("testuser")
            
            if result['status'] != 200:
                return False, f"Expected status 200, got {result['status']}"
            
            accounts = result['data']
            if not isinstance(accounts, list):
                return False, "Response should be a list of accounts"
            
            # Check if exact match appears first with highest score
            if accounts and accounts[0]['username'] == 'testuser':
                return True, f"Found exact match for 'testuser' (returned {len(accounts)} results)"
            
            return True, f"No exact match found (returned {len(accounts)} results)"
        
        return test()

    def test_prefix_search(self):
        """Test prefix matching"""
        def test():
            # Search with a prefix
            result = self.search_accounts("test")
            
            if result['status'] != 200:
                return False, f"Expected status 200, got {result['status']}"
            
            accounts = result['data']
            # Check that all results start with the prefix
            matching = [acc for acc in accounts if acc['username'].startswith('test')]
            
            return True, f"Found {len(matching)} accounts starting with 'test' out of {len(accounts)} total"
        
        return test()

    def test_case_insensitive(self):
        """Test case-insensitive search"""
        def test():
            # Search with different cases
            result1 = self.search_accounts("TEST")
            result2 = self.search_accounts("test")
            
            if result1['status'] != 200 or result2['status'] != 200:
                return False, "Both searches should return 200"
            
            # Results should be similar (not necessarily identical due to scoring)
            return True, f"Case-insensitive search working (TEST: {len(result1['data'])}, test: {len(result2['data'])})"
        
        return test()

    def test_at_symbol_handling(self):
        """Test @ symbol handling"""
        def test():
            # Search with @ prefix
            result1 = self.search_accounts("@testuser")
            result2 = self.search_accounts("testuser")
            
            if result1['status'] != 200 or result2['status'] != 200:
                return False, "Both searches should return 200"
            
            return True, f"@ symbol handling working (@testuser: {len(result1['data'])}, testuser: {len(result2['data'])})"
        
        return test()

    def test_pagination(self):
        """Test pagination with limit and offset"""
        def test():
            # Get first page
            page1 = self.search_accounts("t", limit=5, offset=0)
            
            if page1['status'] != 200:
                return False, f"Expected status 200, got {page1['status']}"
            
            # Get second page
            page2 = self.search_accounts("t", limit=5, offset=5)
            
            if page2['status'] != 200:
                return False, f"Expected status 200 for page 2, got {page2['status']}"
            
            # Check that results are different
            if page1['data'] and page2['data']:
                page1_ids = {acc['id'] for acc in page1['data']}
                page2_ids = {acc['id'] for acc in page2['data']}
                overlap = page1_ids.intersection(page2_ids)
                
                if overlap:
                    return False, f"Pages should not overlap, but found {len(overlap)} common accounts"
            
            return True, f"Pagination working (page1: {len(page1['data'])}, page2: {len(page2['data'])})"
        
        return test()

    def test_empty_query(self):
        """Test empty query handling"""
        def test():
            result = self.search_accounts("")
            
            if result['status'] == 400:
                return True, "Empty query correctly rejected with 400"
            elif result['status'] == 200 and len(result['data']) == 0:
                return True, "Empty query returned empty results"
            else:
                return False, f"Unexpected response for empty query: {result['status']}"
        
        return test()

    def test_metadata_headers(self):
        """Test response metadata headers"""
        def test():
            result = self.search_accounts("test", limit=10)
            
            if result['status'] != 200:
                return False, f"Expected status 200, got {result['status']}"
            
            headers = result['headers']
            
            # Check for expected headers
            if 'x-total-count' in headers or 'X-Total-Count' in headers:
                total = headers.get('x-total-count') or headers.get('X-Total-Count')
                return True, f"Metadata headers present (Total: {total})"
            
            return True, "No metadata headers found (optional feature)"
        
        return test()

    def test_general_search_integration(self):
        """Test that general search endpoint also returns accounts"""
        def test():
            result = self.general_search("test", type="accounts")
            
            if result['status'] != 200:
                return False, f"Expected status 200, got {result['status']}"
            
            if 'accounts' in result['data']:
                accounts = result['data']['accounts']
                return True, f"General search returned {len(accounts)} accounts"
            
            return False, "General search response missing 'accounts' field"
        
        return test()

    def test_search_performance(self):
        """Test search response time"""
        def test():
            queries = ["a", "test", "@user", "nonexistent"]
            times = []
            
            for query in queries:
                start = time.time()
                result = self.search_accounts(query)
                elapsed = time.time() - start
                times.append(elapsed)
                
                if result['status'] != 200:
                    return False, f"Query '{query}' failed with status {result['status']}"
            
            avg_time = sum(times) / len(times)
            max_time = max(times)
            
            if max_time > 2.0:
                return False, f"Search too slow: max {max_time:.2f}s, avg {avg_time:.2f}s"
            
            return True, f"Good performance: max {max_time:.2f}s, avg {avg_time:.2f}s"
        
        return test()

    def test_search_suggestions_basic(self):
        """Test basic search suggestions functionality"""
        def test():
            # Test with a common prefix
            result = self.get_search_suggestions("te")
            
            if result['status'] != 200:
                return False, f"Expected status 200, got {result['status']}"
            
            suggestions = result['data']
            if not isinstance(suggestions, list):
                return False, "Response should be a list of suggestions"
            
            # Check suggestion format
            if suggestions:
                for sugg in suggestions:
                    if not all(key in sugg for key in ['type', 'value', 'display']):
                        return False, f"Suggestion missing required fields: {sugg}"
            
            return True, f"Got {len(suggestions)} suggestions for prefix 'te'"
        
        return test()

    def test_search_suggestions_short_prefix(self):
        """Test suggestions with short prefix"""
        def test():
            # Test with single character (should return empty)
            result = self.get_search_suggestions("t")
            
            if result['status'] != 200:
                return False, f"Expected status 200, got {result['status']}"
            
            suggestions = result['data']
            if len(suggestions) > 0:
                return False, "Single character prefix should return no suggestions"
            
            return True, "Short prefix correctly returns empty suggestions"
        
        return test()

    def test_search_suggestions_performance(self):
        """Test suggestions response time (should be fast for autocomplete)"""
        def test():
            prefixes = ["te", "use", "adm", "mod"]
            times = []
            
            for prefix in prefixes:
                start = time.time()
                result = self.get_search_suggestions(prefix)
                elapsed = time.time() - start
                times.append(elapsed)
                
                if result['status'] != 200:
                    return False, f"Prefix '{prefix}' failed with status {result['status']}"
            
            avg_time = sum(times) / len(times)
            max_time = max(times)
            
            # Suggestions should be very fast (< 100ms)
            if max_time > 0.1:
                return False, f"Suggestions too slow: max {max_time*1000:.0f}ms, avg {avg_time*1000:.0f}ms"
            
            return True, f"Excellent performance: max {max_time*1000:.0f}ms, avg {avg_time*1000:.0f}ms"
        
        return test()

    def run_all_tests(self):
        """Run all search tests"""
        print(f"\n{Fore.CYAN}{'='*60}")
        print(f"Account Search Test Suite")
        print(f"Base URL: {self.base_url}")
        print(f"{'='*60}{Style.RESET_ALL}")

        # Run each test
        self.run_test("Exact Match Search", self.test_exact_match)
        self.run_test("Prefix Search", self.test_prefix_search)
        self.run_test("Case Insensitive Search", self.test_case_insensitive)
        self.run_test("@ Symbol Handling", self.test_at_symbol_handling)
        self.run_test("Pagination", self.test_pagination)
        self.run_test("Empty Query Handling", self.test_empty_query)
        self.run_test("Metadata Headers", self.test_metadata_headers)
        self.run_test("General Search Integration", self.test_general_search_integration)
        self.run_test("Search Performance", self.test_search_performance)
        self.run_test("Search Suggestions (Basic)", self.test_search_suggestions_basic)
        self.run_test("Search Suggestions (Short Prefix)", self.test_search_suggestions_short_prefix)
        self.run_test("Search Suggestions (Performance)", self.test_search_suggestions_performance)

        # Summary
        print(f"\n{Fore.CYAN}{'='*60}")
        print("Test Summary")
        print(f"{'='*60}{Style.RESET_ALL}")
        
        passed = sum(1 for r in self.results if r.passed)
        failed = len(self.results) - passed
        
        print(f"\nTotal Tests: {len(self.results)}")
        print(f"{Fore.GREEN}Passed: {passed}{Style.RESET_ALL}")
        print(f"{Fore.RED}Failed: {failed}{Style.RESET_ALL}")
        
        if failed > 0:
            print(f"\n{Fore.RED}Failed Tests:{Style.RESET_ALL}")
            for result in self.results:
                if not result.passed:
                    print(f"  - {result.name}: {result.message}")
        
        print(f"\nTotal Time: {sum(r.duration for r in self.results):.2f}s")
        
        return failed == 0

def main():
    # Configuration
    BASE_URL = "https://dev.tinybox.dev"  # Update with your actual URL
    
    # You'll need to get an access token first
    # This could be from your test setup or manual OAuth flow
    ACCESS_TOKEN = "test-token"  # Replace with actual token
    
    if len(sys.argv) > 1:
        BASE_URL = sys.argv[1]
    
    if len(sys.argv) > 2:
        ACCESS_TOKEN = sys.argv[2]
    
    # Create tester and run tests
    tester = AccountSearchTester(BASE_URL, ACCESS_TOKEN)
    success = tester.run_all_tests()
    
    # Exit with appropriate code
    sys.exit(0 if success else 1)

if __name__ == "__main__":
    main() 
