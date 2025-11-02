#!/usr/bin/env python3
"""Test hashtag search implementation."""

import requests
import time
from typing import Dict, Any

# Test configuration
BASE_URL = "https://lab.lesser.aronprice.com/api/v1"

def get_auth_headers(username: str = "testuser1", password: str = "testpass1") -> Dict[str, str]:
    """Get authentication headers for API requests."""
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

def test_hashtag_indexing():
    """Test that hashtags are indexed when creating a status."""
    headers = get_auth_headers()
    
    print("Test 1: Create status with hashtags")
    # Create a status with hashtags
    unique_tag = f"test{int(time.time())}"
    status_data = {
        "status": f"Testing hashtag indexing #lesser #activitypub #{unique_tag}",
        "visibility": "public"
    }
    
    response = requests.post(
        f"{BASE_URL}/statuses",
        json=status_data,
        headers=headers
    )
    
    if response.status_code == 200:
        status = response.json()
        print(f"✅ Status created with ID: {status['id']}")
        print(f"   Content: {status['content']}")
        return status['id'], unique_tag
    else:
        print(f"❌ Failed to create status: {response.status_code} - {response.text}")
        return None, None

def test_hashtag_search(unique_tag: str):
    """Test hashtag search functionality."""
    headers = get_auth_headers()
    
    print("\nTest 2: Search for hashtags")
    
    # Test 1: Search by hashtag name
    print(f"  2.1: Search for '#{unique_tag}'")
    response = requests.get(
        f"{BASE_URL}/search",
        params={
            "q": f"#{unique_tag}",
            "type": "hashtags"
        },
        headers=headers
    )
    
    if response.status_code == 200:
        data = response.json()
        hashtags = data.get("hashtags", [])
        if hashtags:
            print(f"  ✅ Found {len(hashtags)} hashtag(s)")
            for tag in hashtags:
                print(f"     - #{tag['name']} ({tag['url']})")
                if tag.get('history'):
                    print(f"       History: {tag['history'][0] if tag['history'] else 'No history'}")
        else:
            print(f"  ⚠️  No hashtags found for #{unique_tag}")
    else:
        print(f"  ❌ Search failed: {response.status_code}")
    
    # Test 2: Search without # prefix
    print(f"\n  2.2: Search for '{unique_tag}' (without #)")
    response = requests.get(
        f"{BASE_URL}/search",
        params={
            "q": unique_tag,
            "type": "hashtags"
        },
        headers=headers
    )
    
    if response.status_code == 200:
        data = response.json()
        hashtags = data.get("hashtags", [])
        if hashtags:
            print(f"  ✅ Found {len(hashtags)} hashtag(s)")
        else:
            print(f"  ⚠️  No hashtags found")
    
    # Test 3: Partial search
    print(f"\n  2.3: Partial search for '{unique_tag[:4]}'")
    response = requests.get(
        f"{BASE_URL}/search",
        params={
            "q": unique_tag[:4],
            "type": "hashtags"
        },
        headers=headers
    )
    
    if response.status_code == 200:
        data = response.json()
        hashtags = data.get("hashtags", [])
        print(f"  ✅ Found {len(hashtags)} hashtag(s) with partial match")

def test_hashtag_timeline(hashtag: str):
    """Test hashtag timeline functionality."""
    headers = get_auth_headers()
    
    print(f"\nTest 3: Hashtag timeline for #{hashtag}")
    response = requests.get(
        f"{BASE_URL}/timelines/tag/{hashtag}",
        headers=headers
    )
    
    if response.status_code == 200:
        statuses = response.json()
        print(f"✅ Timeline returned {len(statuses)} status(es)")
        for status in statuses[:3]:  # Show first 3
            print(f"   - {status['id']}: {status['content'][:50]}...")
    else:
        print(f"❌ Failed to get timeline: {response.status_code}")

def test_search_v2():
    """Test that v2 search also returns hashtags."""
    headers = get_auth_headers()
    
    print("\nTest 4: Search v2 endpoint")
    response = requests.get(
        f"{BASE_URL.replace('v1', 'v2')}/search",
        params={"q": "#activitypub"},
        headers=headers
    )
    
    if response.status_code == 200:
        data = response.json()
        print(f"✅ v2 search successful")
        print(f"   Response has keys: {list(data.keys())}")
        print(f"   Hashtags found: {len(data.get('hashtags', []))}")
    else:
        print(f"❌ v2 search failed: {response.status_code}")

def test_common_hashtags():
    """Test searching for common hashtags."""
    headers = get_auth_headers()
    
    print("\nTest 5: Search for common hashtags")
    common_tags = ["test", "lesser", "activitypub", "mastodon"]
    
    for tag in common_tags:
        response = requests.get(
            f"{BASE_URL}/search",
            params={
                "q": f"#{tag}",
                "type": "hashtags",
                "limit": "5"
            },
            headers=headers
        )
        
        if response.status_code == 200:
            data = response.json()
            hashtags = data.get("hashtags", [])
            print(f"  #{tag}: Found {len(hashtags)} result(s)")
        else:
            print(f"  #{tag}: ❌ Search failed")

if __name__ == "__main__":
    print("Testing Hashtag Search Implementation\n")
    
    # Create a status with hashtags
    status_id, unique_tag = test_hashtag_indexing()
    
    if unique_tag:
        # Wait a moment for indexing
        print("\nWaiting 2 seconds for indexing...")
        time.sleep(2)
        
        # Test hashtag search
        test_hashtag_search(unique_tag)
        
        # Test hashtag timeline
        test_hashtag_timeline(unique_tag)
    
    # Test v2 search
    test_search_v2()
    
    # Test common hashtags
    test_common_hashtags()
    
    print("\nTest completed!") 
