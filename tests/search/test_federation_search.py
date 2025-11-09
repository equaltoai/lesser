#!/usr/bin/env python3
"""
Test script for Federation & Remote Search (Track 2)

This script tests:
1. WebFinger lookups for remote actors
2. Remote actor discovery and caching
3. Cross-instance search with @user@domain handles
"""

import requests
import json
import sys
import time
import argparse
from urllib.parse import quote

# ANSI color codes
GREEN = '\033[92m'
RED = '\033[91m'
YELLOW = '\033[93m'
BLUE = '\033[94m'
RESET = '\033[0m'

def print_test_header(test_name):
    print(f"\n{BLUE}{'='*60}{RESET}")
    print(f"{BLUE}Testing: {test_name}{RESET}")
    print(f"{BLUE}{'='*60}{RESET}")

def print_result(success, message):
    if success:
        print(f"{GREEN}✓ {message}{RESET}")
    else:
        print(f"{RED}✗ {message}{RESET}")

def test_webfinger_local(base_url, username):
    """Test WebFinger for local actors"""
    print_test_header(f"WebFinger for local actor: {username}")
    
    # Extract domain from base URL
    domain = base_url.replace("https://", "").replace("http://", "")
    resource = f"acct:{username}@{domain}"
    
    url = f"{base_url}/.well-known/webfinger?resource={quote(resource)}"
    
    try:
        response = requests.get(url, headers={"Accept": "application/jrd+json"})
        
        if response.status_code == 200:
            data = response.json()
            print_result(True, f"WebFinger resolved successfully")
            print(f"  Subject: {data.get('subject')}")
            
            # Check for ActivityPub link
            ap_link = None
            for link in data.get('links', []):
                if link.get('rel') == 'self' and link.get('type') == 'application/activity+json':
                    ap_link = link.get('href')
                    break
            
            if ap_link:
                print_result(True, f"Found ActivityPub link: {ap_link}")
            else:
                print_result(False, "No ActivityPub link found")
            
            return True
        else:
            print_result(False, f"WebFinger failed: {response.status_code}")
            if response.text:
                print(f"  Response: {response.text}")
            return False
            
    except Exception as e:
        print_result(False, f"WebFinger error: {str(e)}")
        return False

def test_remote_actor_search(base_url, token, remote_handle):
    """Test searching for a remote actor"""
    print_test_header(f"Remote actor search: {remote_handle}")
    
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    params = {
        "q": remote_handle,
        "resolve": "true",
        "limit": "5"
    }
    
    url = f"{base_url}/api/v1/accounts/search"
    
    try:
        response = requests.get(url, headers=headers, params=params)
        
        if response.status_code == 200:
            accounts = response.json()
            
            if len(accounts) > 0:
                print_result(True, f"Found {len(accounts)} result(s)")
                
                for account in accounts:
                    print(f"\n  Account: @{account['username']}@{account.get('domain', 'local')}")
                    print(f"  Display name: {account.get('display_name', 'N/A')}")
                    print(f"  ID: {account['id']}")
                    print(f"  URL: {account.get('url', account.get('uri'))}")
                    
                    # Check if it's a remote actor
                    if account.get('domain'):
                        print_result(True, f"Remote actor from domain: {account['domain']}")
                    
                return True
            else:
                print_result(False, "No results found")
                print("  Note: The remote instance might be down or the actor doesn't exist")
                return False
        else:
            print_result(False, f"Search failed: {response.status_code}")
            if response.text:
                print(f"  Response: {response.text}")
            return False
            
    except Exception as e:
        print_result(False, f"Search error: {str(e)}")
        return False

def test_cached_remote_search(base_url, token, remote_handle):
    """Test that remote actors are cached properly"""
    print_test_header("Testing remote actor caching")
    
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    params = {
        "q": remote_handle,
        "resolve": "true",
        "limit": "5"
    }
    
    url = f"{base_url}/api/v1/accounts/search"
    
    # First search (should fetch from remote)
    print("\n1. Initial search (fetching from remote)...")
    start_time = time.time()
    
    try:
        response = requests.get(url, headers=headers, params=params)
        first_time = time.time() - start_time
        
        if response.status_code == 200 and len(response.json()) > 0:
            print_result(True, f"First search completed in {first_time:.2f}s")
        else:
            print_result(False, "First search failed")
            return False
            
    except Exception as e:
        print_result(False, f"First search error: {str(e)}")
        return False
    
    # Second search (should be cached)
    print("\n2. Second search (should use cache)...")
    start_time = time.time()
    
    try:
        response = requests.get(url, headers=headers, params=params)
        second_time = time.time() - start_time
        
        if response.status_code == 200 and len(response.json()) > 0:
            print_result(True, f"Second search completed in {second_time:.2f}s")
            
            # Cached search should be faster
            if second_time < first_time:
                print_result(True, f"Cache is working! Second search was {(first_time/second_time):.1f}x faster")
            else:
                print_result(False, "Second search wasn't faster (cache might not be working)")
                
            return True
        else:
            print_result(False, "Second search failed")
            return False
            
    except Exception as e:
        print_result(False, f"Second search error: {str(e)}")
        return False

def test_multiple_remote_handles(base_url, token, handles):
    """Test searching for multiple remote handles"""
    print_test_header("Testing multiple remote actor searches")
    
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    
    success_count = 0
    for handle in handles:
        print(f"\n{YELLOW}Searching for: {handle}{RESET}")
        
        params = {
            "q": handle,
            "resolve": "true",
            "limit": "1"
        }
        
        url = f"{base_url}/api/v1/accounts/search"
        
        try:
            response = requests.get(url, headers=headers, params=params)
            
            if response.status_code == 200:
                accounts = response.json()
                if len(accounts) > 0:
                    account = accounts[0]
                    print_result(True, f"Found: @{account['username']}@{account.get('domain', 'local')}")
                    success_count += 1
                else:
                    print_result(False, "No results")
            else:
                print_result(False, f"Error: {response.status_code}")
                
        except Exception as e:
            print_result(False, f"Error: {str(e)}")
    
    print(f"\n{BLUE}Summary: {success_count}/{len(handles)} searches successful{RESET}")
    return success_count > 0

def test_invalid_handles(base_url, token):
    """Test handling of invalid handle formats"""
    print_test_header("Testing invalid handle formats")
    
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    
    invalid_handles = [
        "notahandle",
        "@@@invalid",
        "user@@domain.com",
        "@user@domain@extra",
        "",
        "@"
    ]
    
    for handle in invalid_handles:
        print(f"\n{YELLOW}Testing: '{handle}'{RESET}")
        
        params = {
            "q": handle,
            "resolve": "true",
            "limit": "1"
        }
        
        url = f"{base_url}/api/v1/accounts/search"
        
        try:
            response = requests.get(url, headers=headers, params=params)
            
            # For invalid handles, we expect either no results or only local results
            if response.status_code == 200:
                accounts = response.json()
                if len(accounts) == 0:
                    print_result(True, "No results (expected for invalid handle)")
                else:
                    # Check if any results are remote
                    has_remote = any(acc.get('domain') for acc in accounts)
                    if not has_remote:
                        print_result(True, "Only local results (no remote lookup attempted)")
                    else:
                        print_result(False, "Got remote results for invalid handle!")
            else:
                print_result(True, f"Request failed as expected: {response.status_code}")
                
        except Exception as e:
            print_result(False, f"Unexpected error: {str(e)}")
    
    return True

def main():
    parser = argparse.ArgumentParser(description='Test Federation & Remote Search')
    parser.add_argument('base_url', help='Base URL of the instance (e.g., https://your-instance.com)')
    parser.add_argument('--token', help='OAuth token for authenticated requests')
    parser.add_argument('--local-user', default='test', help='Local username to test WebFinger')
    parser.add_argument('--remote-handle', default='@gargron@mastodon.social', 
                       help='Remote handle to search for')
    parser.add_argument('--test', choices=['all', 'webfinger', 'search', 'cache', 'multiple', 'invalid'],
                       default='all', help='Which test to run')
    
    args = parser.parse_args()
    
    # Remove trailing slash from base URL
    base_url = args.base_url.rstrip('/')
    
    print(f"{BLUE}Federation & Remote Search Test Suite{RESET}")
    print(f"{BLUE}Instance: {base_url}{RESET}")
    
    # Track test results
    all_passed = True
    
    # Run tests based on selection
    if args.test in ['all', 'webfinger']:
        if not test_webfinger_local(base_url, args.local_user):
            all_passed = False
    
    if args.test in ['all', 'search']:
        if not test_remote_actor_search(base_url, args.token, args.remote_handle):
            all_passed = False
    
    if args.test in ['all', 'cache']:
        if args.token:
            if not test_cached_remote_search(base_url, args.token, args.remote_handle):
                all_passed = False
        else:
            print(f"\n{YELLOW}Skipping cache test (requires --token){RESET}")
    
    if args.test in ['all', 'multiple']:
        # Test multiple well-known ActivityPub instances
        test_handles = [
            "@gargron@mastodon.social",
            "@Mastodon@mastodon.social",
            "@peertube@framapiaf.org"
        ]
        if not test_multiple_remote_handles(base_url, args.token, test_handles):
            all_passed = False
    
    if args.test in ['all', 'invalid']:
        if not test_invalid_handles(base_url, args.token):
            all_passed = False
    
    # Summary
    print(f"\n{BLUE}{'='*60}{RESET}")
    if all_passed:
        print(f"{GREEN}✓ All tests passed!{RESET}")
        print(f"\n{GREEN}Federation & Remote Search is working correctly!{RESET}")
    else:
        print(f"{RED}✗ Some tests failed{RESET}")
        print(f"\n{YELLOW}Check the output above for details{RESET}")
    
    sys.exit(0 if all_passed else 1)

if __name__ == "__main__":
    main() 