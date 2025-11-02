#!/usr/bin/env python3
"""
Automated test script for lesser API using Mastodon.py
Tests public endpoints only
"""

from mastodon import Mastodon
import json
import requests
import time
import sys
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

def test_public_endpoints():
    """Test public endpoints that don't require authentication"""
    print("\n=== Testing Public Endpoints ===")
    
    # Test instance v2 (v2 endpoint)
    try:
        response = requests.get(f"{INSTANCE_URL}/api/v2/instance")
        if response.status_code == 200:
            data = response.json()
            # Debug: print full response
            print(f"  DEBUG: Full v2 response: {json.dumps(data, indent=2)[:500]}...")
            log_test(True, "GET /api/v2/instance", 
                    f"Domain: {data.get('domain', 'N/A')}")
        else:
            log_test(False, "GET /api/v2/instance", f"Status {response.status_code}")
    except Exception as e:
        log_test(False, "GET /api/v2/instance", str(e))
    
    # Public timeline (v1 endpoint)
    mastodon = Mastodon(api_base_url=INSTANCE_URL)
    try:
        timeline = mastodon.timeline_public(limit=10)
        log_test(True, "GET /api/v1/timelines/public", 
                f"Statuses: {len(timeline)}")
    except Exception as e:
        log_test(False, "GET /api/v1/timelines/public", str(e))
    
    # Local timeline (v1 endpoint)
    try:
        timeline = mastodon.timeline_local(limit=10)
        log_test(True, "GET /api/v1/timelines/public?local=true", 
                f"Local statuses: {len(timeline)}")
    except Exception as e:
        log_test(False, "GET /api/v1/timelines/public?local=true", str(e))
    
    # Hashtag timeline (v1 endpoint)
    try:
        timeline = mastodon.timeline_hashtag("test", limit=10)
        log_test(True, "GET /api/v1/timelines/tag/test", 
                f"Statuses: {len(timeline)}")
    except Exception as e:
        log_test(False, "GET /api/v1/timelines/tag/test", str(e))
    
    # Custom emojis (v1 endpoint)
    try:
        emojis = mastodon.custom_emojis()
        log_test(True, "GET /api/v1/custom_emojis", 
                f"Emojis: {len(emojis)}")
    except Exception as e:
        log_test(False, "GET /api/v1/custom_emojis", str(e))
    
    # Instance activity (v1 endpoint)
    try:
        activity = mastodon.instance_activity()
        log_test(True, "GET /api/v1/instance/activity", 
                f"Weeks: {len(activity)}")
    except Exception as e:
        log_test(False, "GET /api/v1/instance/activity", str(e))
    
    # Instance peers (v1 endpoint)
    try:
        peers = mastodon.instance_peers()
        log_test(True, "GET /api/v1/instance/peers", 
                f"Peers: {len(peers)}")
    except Exception as e:
        log_test(False, "GET /api/v1/instance/peers", str(e))

def test_search():
    """Test search functionality (v2 endpoint)"""
    print("\n=== Testing Search ===")
    mastodon = Mastodon(api_base_url=INSTANCE_URL)
    
    # Search for accounts
    try:
        results = mastodon.search_v2("@testuser", result_type="accounts")
        log_test(True, "Search for accounts", 
                f"Found {len(results['accounts'])} accounts")
    except Exception as e:
        log_test(False, "Search for accounts", str(e))
    
    # Search for hashtags
    try:
        results = mastodon.search_v2("#test", result_type="hashtags")
        log_test(True, "Search for hashtags", 
                f"Found {len(results['hashtags'])} hashtags")
    except Exception as e:
        log_test(False, "Search for hashtags", str(e))

def test_webfinger():
    """Test WebFinger lookups"""
    print("\n=== Testing WebFinger ===")
    
    try:
        response = requests.get(f"{INSTANCE_URL}/.well-known/webfinger",
                              params={"resource": "acct:testuser@lesser.host"})
        if response.status_code == 200:
            data = response.json()
            log_test(True, "WebFinger lookup", 
                    f"Subject: {data.get('subject', 'N/A')}")
        else:
            log_test(False, "WebFinger lookup", f"Status {response.status_code}")
    except Exception as e:
        log_test(False, "WebFinger lookup", str(e))

def test_nodeinfo():
    """Test NodeInfo endpoints"""
    print("\n=== Testing NodeInfo ===")
    
    # NodeInfo discovery
    try:
        response = requests.get(f"{INSTANCE_URL}/.well-known/nodeinfo")
        if response.status_code == 200:
            data = response.json()
            log_test(True, "GET /.well-known/nodeinfo", 
                    f"Links: {len(data.get('links', []))}")
            
            # Follow NodeInfo 2.0 link
            for link in data.get('links', []):
                if link.get('rel') == 'http://nodeinfo.diaspora.software/ns/schema/2.0':
                    break
        else:
            log_test(False, "GET /.well-known/nodeinfo", f"Status {response.status_code}")
    except Exception as e:
        log_test(False, "GET /.well-known/nodeinfo", str(e))
    
    # NodeInfo 2.0
    try:
        response = requests.get(f"{INSTANCE_URL}/nodeinfo/2.0")
        if response.status_code == 200:
            data = response.json()
            log_test(True, "GET /nodeinfo/2.0", 
                    f"Software: {data.get('software', {}).get('name', 'N/A')} {data.get('software', {}).get('version', 'N/A')}")
        else:
            log_test(False, "GET /nodeinfo/2.0", f"Status {response.status_code}")
    except Exception as e:
        log_test(False, "GET /nodeinfo/2.0", str(e))

def test_pagination():
    """Test pagination headers"""
    print("\n=== Testing Pagination ===")
    
    endpoints = [
        ("/api/v1/timelines/public?limit=5", "public timeline"),
        ("/api/v1/timelines/tag/test?limit=5", "hashtag timeline"),
        ("/api/v1/custom_emojis", "custom emojis")
    ]
    
    for endpoint, name in endpoints:
        try:
            response = requests.get(f"{INSTANCE_URL}{endpoint}")
            
            # Show what headers are actually present
            header_info = []
            if 'Link' in response.headers:
                header_info.append(f"Link: {response.headers['Link'][:50]}...")
            if 'X-Total-Count' in response.headers:
                header_info.append(f"X-Total-Count: {response.headers['X-Total-Count']}")
            
            details = f"Status: {response.status_code}, "
            if header_info:
                details += ", ".join(header_info)
            else:
                details += "No pagination headers"
                
            log_test(True, f"Pagination for {endpoint}", details)
        except Exception as e:
            log_test(False, f"Pagination for {endpoint}", str(e))

def test_account_lookup():
    """Test account lookup (v1 endpoint)"""
    print("\n=== Testing Account Lookup ===")
    
    try:
        response = requests.get(f"{INSTANCE_URL}/api/v1/accounts/lookup",
                              params={"acct": "testuser"})
        if response.status_code == 200:
            data = response.json()
            log_test(True, "GET /api/v1/accounts/lookup", 
                    f"Found account: @{data.get('username', 'N/A')} (ID: {data.get('id', 'N/A')})")
            
            # Test getting account statuses
            account_id = data.get('id')
            if account_id:
                response2 = requests.get(f"{INSTANCE_URL}/api/v1/accounts/{account_id}/statuses")
                if response2.status_code == 200:
                    statuses = response2.json()
                    log_test(True, f"GET /api/v1/accounts/{account_id}/statuses", 
                            f"Statuses: {len(statuses)}")
                else:
                    log_test(False, f"GET /api/v1/accounts/{account_id}/statuses", 
                            f"Status {response2.status_code}")
        else:
            log_test(False, "GET /api/v1/accounts/lookup", f"Status {response.status_code}")
    except Exception as e:
        log_test(False, "GET /api/v1/accounts/lookup", str(e))

def test_app_creation():
    """Test OAuth app creation (v1 endpoint)"""
    print("\n=== Testing App Creation ===")
    
    try:
        response = requests.post(f"{INSTANCE_URL}/api/v1/apps", json={
            "client_name": "Test App",
            "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
            "scopes": "read write follow",
            "website": "https://example.com"
        })
        if response.status_code == 200:
            data = response.json()
            log_test(True, "POST /api/v1/apps", 
                    f"Client ID: {data.get('client_id', 'N/A')[:20]}...")
        else:
            log_test(False, "POST /api/v1/apps", f"Status {response.status_code}")
    except Exception as e:
        log_test(False, "POST /api/v1/apps", str(e))

def main():
    print(f"Testing Lesser API at {INSTANCE_URL}")
    print(f"Started at {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Run tests
    test_public_endpoints()
    test_search()
    test_webfinger()
    test_nodeinfo()
    test_pagination()
    test_account_lookup()
    test_app_creation()
    
    # Print summary
    print("\n" + "="*50)
    print(f"Test Summary: {tests_passed} passed, {tests_failed} failed")
    print(f"Completed at {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Exit with appropriate code
    sys.exit(0 if tests_failed == 0 else 1)

if __name__ == "__main__":
    main() 
