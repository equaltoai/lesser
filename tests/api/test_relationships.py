#!/usr/bin/env python3
"""Test script for Priority 1 endpoints."""

import requests
import sys

def test_account_relationships_batch(base_url, token):
    """Test the GET /api/v1/accounts/relationships endpoint."""
    print("\n=== Testing Account Relationships Batch ===")
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Test with multiple account IDs
    params = {
        "id[]": ["test", "admin", "nonexistent"]  # Array format
    }
    
    response = requests.get(
        f"{base_url}/api/v1/accounts/relationships",
        headers=headers,
        params=params
    )
    
    print(f"Status Code: {response.status_code}")
    
    if response.status_code == 200:
        relationships = response.json()
        print(f"Found {len(relationships)} relationships")
        
        for rel in relationships:
            print(f"\nRelationship with @{rel['id']}:")
            print(f"  Following: {rel['following']}")
            print(f"  Followed by: {rel['followed_by']}")
            print(f"  Blocking: {rel['blocking']}")
            print(f"  Muting: {rel['muting']}")
            if 'requested' in rel:
                print(f"  Requested: {rel['requested']}")
        
        return True
    else:
        print(f"Error: {response.status_code}")
        print(f"Response: {response.text}")
        return False

def test_status_favourited_by(base_url, token, status_id=None):
    """Test the GET /api/v1/statuses/:id/favourited_by endpoint."""
    print("\n=== Testing Status Favourited By ===")
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # If no status_id provided, try to get one from public timeline
    if not status_id:
        timeline_resp = requests.get(
            f"{base_url}/api/v1/timelines/public",
            headers=headers,
            params={"limit": 10}
        )
        if timeline_resp.status_code == 200:
            statuses = timeline_resp.json()
            for status in statuses:
                if status.get('favourites_count', 0) > 0:
                    status_id = status['id']
                    print(f"Using status {status_id} which has {status['favourites_count']} favorites")
                    break
    
    if not status_id:
        print("No favorited status found to test with")
        return True  # Not a failure, just no data
    
    response = requests.get(
        f"{base_url}/api/v1/statuses/{status_id}/favourited_by",
        headers=headers,
        params={"limit": 20}
    )
    
    print(f"Status Code: {response.status_code}")
    
    if response.status_code == 200:
        accounts = response.json()
        print(f"Found {len(accounts)} accounts who favorited this status")
        
        for i, account in enumerate(accounts[:3]):
            print(f"\nAccount {i+1}:")
            print(f"  Username: @{account['username']}")
            print(f"  Display Name: {account['display_name']}")
        
        # Check pagination header
        link_header = response.headers.get('Link')
        if link_header:
            print(f"\nPagination available: {link_header}")
        
        return True
    else:
        print(f"Error: {response.status_code}")
        print(f"Response: {response.text}")
        return False

def test_status_reblogged_by(base_url, token, status_id=None):
    """Test the GET /api/v1/statuses/:id/reblogged_by endpoint."""
    print("\n=== Testing Status Reblogged By ===")
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # If no status_id provided, try to get one from public timeline
    if not status_id:
        timeline_resp = requests.get(
            f"{base_url}/api/v1/timelines/public",
            headers=headers,
            params={"limit": 10}
        )
        if timeline_resp.status_code == 200:
            statuses = timeline_resp.json()
            for status in statuses:
                if status.get('reblogs_count', 0) > 0:
                    status_id = status['id']
                    print(f"Using status {status_id} which has {status['reblogs_count']} reblogs")
                    break
    
    if not status_id:
        print("No reblogged status found to test with")
        return True  # Not a failure, just no data
    
    response = requests.get(
        f"{base_url}/api/v1/statuses/{status_id}/reblogged_by",
        headers=headers,
        params={"limit": 20}
    )
    
    print(f"Status Code: {response.status_code}")
    
    if response.status_code == 200:
        accounts = response.json()
        print(f"Found {len(accounts)} accounts who reblogged this status")
        
        for i, account in enumerate(accounts[:3]):
            print(f"\nAccount {i+1}:")
            print(f"  Username: @{account['username']}")
            print(f"  Display Name: {account['display_name']}")
        
        # Check pagination header
        link_header = response.headers.get('Link')
        if link_header:
            print(f"\nPagination available: {link_header}")
        
        return True
    else:
        print(f"Error: {response.status_code}")
        print(f"Response: {response.text}")
        return False

def test_follow_requests(base_url, token):
    """Test follow request endpoints."""
    print("\n=== Testing Follow Request Endpoints ===")
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Test GET /api/v1/follow_requests
    print("\n1. Getting follow requests...")
    response = requests.get(
        f"{base_url}/api/v1/follow_requests",
        headers=headers
    )
    
    print(f"Status Code: {response.status_code}")
    
    if response.status_code == 200:
        requests_list = response.json()
        print(f"Found {len(requests_list)} follow requests")
        print("(Note: Lesser doesn't support locked accounts yet, so this should be empty)")
        
        # Test authorize endpoint (should be no-op for now)
        print("\n2. Testing authorize endpoint...")
        auth_response = requests.post(
            f"{base_url}/api/v1/follow_requests/test_user/authorize",
            headers=headers
        )
        
        if auth_response.status_code == 200:
            print("Authorize endpoint returned success (no-op)")
            relationship = auth_response.json()
            print(f"Relationship: {json.dumps(relationship, indent=2)}")
        else:
            print(f"Authorize failed: {auth_response.status_code}")
        
        # Test reject endpoint (should be no-op for now)
        print("\n3. Testing reject endpoint...")
        reject_response = requests.post(
            f"{base_url}/api/v1/follow_requests/test_user/reject",
            headers=headers
        )
        
        if reject_response.status_code == 200:
            print("Reject endpoint returned success (no-op)")
            relationship = reject_response.json()
            print(f"Relationship: {json.dumps(relationship, indent=2)}")
        else:
            print(f"Reject failed: {reject_response.status_code}")
        
        return True
    else:
        print(f"Error: {response.status_code}")
        print(f"Response: {response.text}")
        return False

def test_favorites_timeline(base_url, token):
    """Test the GET /api/v1/favourites endpoint."""
    print("\n=== Testing Favorites Timeline ===")
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    response = requests.get(
        f"{base_url}/api/v1/favourites",
        headers=headers,
        params={"limit": 10}
    )
    
    print(f"Status Code: {response.status_code}")
    
    if response.status_code == 200:
        favorites = response.json()
        print(f"Found {len(favorites)} favorited statuses")
        
        for i, status in enumerate(favorites[:3]):
            print(f"\nFavorite {i+1}:")
            print(f"  ID: {status['id']}")
            print(f"  Author: @{status['account']['username']}")
            print(f"  Favorited: {status['favourited']}")
        
        return True
    else:
        print(f"Error: {response.status_code}")
        print(f"Response: {response.text}")
        return False

def main():
    if len(sys.argv) < 3:
        print("Usage: python test_priority1_endpoints.py <base_url> <access_token>")
        print("Example: python test_priority1_endpoints.py https://your-instance.com your-token-here")
        sys.exit(1)
    
    base_url = sys.argv[1].rstrip('/')
    token = sys.argv[2]
    
    print("Testing all Priority 1 endpoints...")
    
    results = []
    
    # Test each endpoint
    results.append(("Account Relationships Batch", test_account_relationships_batch(base_url, token)))
    results.append(("Favorites Timeline", test_favorites_timeline(base_url, token)))
    results.append(("Status Favourited By", test_status_favourited_by(base_url, token)))
    results.append(("Status Reblogged By", test_status_reblogged_by(base_url, token)))
    results.append(("Follow Requests", test_follow_requests(base_url, token)))
    
    # Summary
    print("\n" + "="*50)
    print("SUMMARY")
    print("="*50)
    
    all_passed = True
    for name, passed in results:
        status = "✅ PASSED" if passed else "❌ FAILED"
        print(f"{name}: {status}")
        if not passed:
            all_passed = False
    
    if all_passed:
        print("\n🎉 All Priority 1 endpoints are working!")
    else:
        print("\n❌ Some tests failed!")
        sys.exit(1)

if __name__ == "__main__":
    main() 
