#!/usr/bin/env python3
"""Test script for the favorites timeline endpoint."""

import requests
import sys
import json

def test_favorites_timeline(base_url, token):
    """Test the GET /api/v1/favourites endpoint."""
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    print("Testing favorites timeline...")
    
    # Get the favorites timeline
    response = requests.get(
        f"{base_url}/api/v1/favourites",
        headers=headers,
        params={"limit": 20}
    )
    
    print(f"Status Code: {response.status_code}")
    
    if response.status_code == 200:
        favorites = response.json()
        print(f"Found {len(favorites)} favorited statuses")
        
        # Check pagination header
        link_header = response.headers.get('Link')
        if link_header:
            print(f"Pagination Link: {link_header}")
        
        # Display first few favorites
        for i, status in enumerate(favorites[:3]):
            print(f"\nFavorite {i+1}:")
            print(f"  ID: {status['id']}")
            print(f"  Author: @{status['account']['username']}")
            print(f"  Content: {status['content'][:100]}...")
            print(f"  Favorited: {status['favourited']}")
            print(f"  Created: {status['created_at']}")
            
        # Test pagination if we have a cursor
        if link_header and len(favorites) == 20:
            print("\n\nTesting pagination...")
            # Extract max_id from Link header
            import re
            match = re.search(r'max_id=([^&>]+)', link_header)
            if match:
                max_id = match.group(1)
                response = requests.get(
                    f"{base_url}/api/v1/favourites",
                    headers=headers,
                    params={"limit": 20, "max_id": max_id}
                )
                if response.status_code == 200:
                    page2 = response.json()
                    print(f"Page 2 has {len(page2)} statuses")
                else:
                    print(f"Pagination failed: {response.status_code}")
        
        print("\n✅ Favorites timeline endpoint is working!")
        return True
    else:
        print(f"Error: {response.status_code}")
        print(f"Response: {response.text}")
        return False

def main():
    if len(sys.argv) < 3:
        print("Usage: python test_favorites.py <base_url> <access_token>")
        print("Example: python test_favorites.py https://your-instance.com your-token-here")
        sys.exit(1)
    
    base_url = sys.argv[1].rstrip('/')
    token = sys.argv[2]
    
    success = test_favorites_timeline(base_url, token)
    
    if success:
        print("\n🎉 All tests passed!")
    else:
        print("\n❌ Some tests failed!")
        sys.exit(1)

if __name__ == "__main__":
    main() 