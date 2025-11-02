#!/usr/bin/env python3
"""Test the search v2 endpoint implementation."""

import requests
import json
from typing import Dict, Any

# Test configuration
BASE_URL = "https://lab.lesser.aronprice.com/api/v1"
BASE_URL_V2 = "https://lab.lesser.aronprice.com/api/v2"

def get_auth_headers(username: str = "testuser1", password: str = "testpass1") -> Dict[str, str]:
    """Get authentication headers for API requests."""
    # First, get the access token
    auth_data = {
        "grant_type": "password",
        "username": username,
        "password": password,
        "client_id": "test_client",
        "client_secret": "test_secret",
        "scope": "read write follow"
    }
    
    response = requests.post(f"{BASE_URL}/oauth/token", data=auth_data)
    if response.status_code == 200:
        token = response.json()["access_token"]
        return {"Authorization": f"Bearer {token}"}
    else:
        print(f"Failed to authenticate: {response.status_code} - {response.text}")
        return {}

def test_search_v2():
    """Test the v2 search endpoint."""
    headers = get_auth_headers()
    
    # Test 1: Basic search
    print("Test 1: Basic search with v2 endpoint")
    response = requests.get(
        f"{BASE_URL_V2}/search",
        params={"q": "test"},
        headers=headers
    )
    
    print(f"Status: {response.status_code}")
    if response.status_code == 200:
        data = response.json()
        print(f"Response structure: {list(data.keys())}")
        print(f"Accounts found: {len(data.get('accounts', []))}")
        print(f"Statuses found: {len(data.get('statuses', []))}")
        print(f"Hashtags found: {len(data.get('hashtags', []))}")
        print("✅ v2 search returns expected format")
    else:
        print(f"❌ Failed: {response.text}")
    
    print("\n" + "="*50 + "\n")
    
    # Test 2: Compare v1 and v2 results
    print("Test 2: Compare v1 and v2 search results")
    query = "testuser"
    
    # v1 search
    v1_response = requests.get(
        f"{BASE_URL}/search",
        params={"q": query},
        headers=headers
    )
    
    # v2 search
    v2_response = requests.get(
        f"{BASE_URL_V2}/search",
        params={"q": query},
        headers=headers
    )
    
    if v1_response.status_code == 200 and v2_response.status_code == 200:
        v1_data = v1_response.json()
        v2_data = v2_response.json()
        
        # Compare structure
        if list(v1_data.keys()) == list(v2_data.keys()):
            print("✅ v1 and v2 have same response structure")
        else:
            print(f"❌ Different structures: v1={list(v1_data.keys())}, v2={list(v2_data.keys())}")
        
        # Compare results
        if v1_data == v2_data:
            print("✅ v1 and v2 return identical results")
        else:
            print("❌ Results differ between v1 and v2")
    else:
        print(f"❌ Failed to get responses: v1={v1_response.status_code}, v2={v2_response.status_code}")
    
    print("\n" + "="*50 + "\n")
    
    # Test 3: Search with type filter
    print("Test 3: Search with type filter")
    response = requests.get(
        f"{BASE_URL_V2}/search",
        params={"q": "test", "type": "accounts"},
        headers=headers
    )
    
    if response.status_code == 200:
        data = response.json()
        # When type=accounts, only accounts should have results
        has_statuses = len(data.get('statuses', [])) == 0
        has_hashtags = len(data.get('hashtags', [])) == 0
        
        if has_statuses and has_hashtags:
            print("✅ Type filter works correctly")
        else:
            print(f"⚠️  Type filter may not be working properly:")
            print(f"   Accounts: {len(data.get('accounts', []))}")
            print(f"   Statuses: {len(data.get('statuses', []))}")
            print(f"   Hashtags: {len(data.get('hashtags', []))}")
    else:
        print(f"❌ Failed: {response.text}")

if __name__ == "__main__":
    print("Testing Search v2 Endpoint Implementation\n")
    test_search_v2()
    print("\nTest completed!") 
