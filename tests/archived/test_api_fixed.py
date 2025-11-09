#!/usr/bin/env python3
"""
Fixed version of the test script with better error handling
"""

from mastodon import Mastodon
import time
import sys
import requests
from datetime import datetime

# Your lesser instance
INSTANCE_URL = 'https://lesser.host'

# Global counters for test results
tests_passed = 0
tests_failed = 0

def log_test(success, test_name, details=""):
    """Log test result with consistent formatting"""
    global tests_passed, tests_failed
    
    if success:
        tests_passed += 1
        print(f"✓ {test_name}")
        if details:
            print(f"  {details}")
    else:
        tests_failed += 1
        print(f"✗ {test_name}")
        if details:
            print(f"  ERROR: {details}")

def test_search(mastodon):
    """Test search functionality with better error handling"""
    print("\n\n=== Testing Search ===\n")
    
    try:
        # V2 search - try without limit first
        try:
            results = mastodon.search_v2('test')
        except TypeError:
            # If that fails, try with q parameter
            results = mastodon.search(q='test', version=2)
            
        log_test(True, "GET /api/v2/search", 
                f"Accounts: {len(results.get('accounts', []))}, "
                f"Statuses: {len(results.get('statuses', []))}, "
                f"Hashtags: {len(results.get('hashtags', []))}")
    except Exception as e:
        log_test(False, "GET /api/v2/search", str(e))
    
    try:
        # Search with resolve - also handle different API versions
        try:
            results = mastodon.search_v2('test', resolve=True)
        except TypeError:
            try:
                results = mastodon.search(q='test', resolve=True, version=2)
            except:
                # Fall back to raw request
                response = requests.get(
                    f"{INSTANCE_URL}/api/v2/search",
                    params={'q': 'test', 'resolve': 'true'},
                    headers={'Authorization': f'Bearer {mastodon.access_token}'}
                )
                if response.status_code == 200:
                    results = response.json()
                else:
                    raise Exception(f"Status {response.status_code}")
                    
        log_test(True, "GET /api/v2/search?resolve=true", "Search with resolve")
    except Exception as e:
        log_test(False, "GET /api/v2/search?resolve=true", str(e))

# Example of using the fixed search function
if __name__ == "__main__":
    # This would be called from the main test script
    print("Use this fixed search function in your test script") 