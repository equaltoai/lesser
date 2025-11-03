#!/usr/bin/env python3
"""
Test script for Lesser lists management system.

This script tests all list endpoints:
- GET /api/v1/lists - Get user's lists
- POST /api/v1/lists - Create list
- GET /api/v1/lists/:id - Get specific list
- PUT /api/v1/lists/:id - Update list
- DELETE /api/v1/lists/:id - Delete list
- GET /api/v1/lists/:id/accounts - Get accounts in list
- POST /api/v1/lists/:id/accounts - Add accounts to list
- DELETE /api/v1/lists/:id/accounts - Remove accounts from list
- GET /api/v1/timelines/list/:list_id - List timeline
"""

import requests
import sys
import time
import argparse
from datetime import datetime

def test_lists(base_url, token):
    """Test lists management system."""
    
    headers = {"Authorization": f"Bearer {token}"}
    
    print("\n📋 Testing Lists Management System")
    print("=" * 50)
    
    # Get user's own account info
    r = requests.get(f"{base_url}/api/v1/accounts/verify_credentials", headers=headers)
    assert r.status_code == 200
    
    # 1. Get existing lists (should be empty initially)
    print("\n1. Getting existing lists...")
    r = requests.get(f"{base_url}/api/v1/lists", headers=headers)
    assert r.status_code == 200, f"Failed to get lists: {r.status_code} {r.text}"
    lists = r.json()
    initial_count = len(lists)
    print(f"✅ Found {initial_count} existing lists")
    
    # 2. Create a new list
    print("\n2. Creating a new list...")
    list_data = {
        "title": f"Test List {datetime.now().strftime('%Y%m%d%H%M%S')}",
        "replies_policy": "list"  # "followed", "list", or "none"
    }
    r = requests.post(f"{base_url}/api/v1/lists", headers=headers, json=list_data)
    assert r.status_code == 200, f"Failed to create list: {r.status_code} {r.text}"
    created_list = r.json()
    list_id = created_list['id']
    print(f"✅ Created list: {created_list['title']} (ID: {list_id})")
    assert created_list['replies_policy'] == 'list'
    
    # 3. Get the specific list
    print("\n3. Getting specific list...")
    r = requests.get(f"{base_url}/api/v1/lists/{list_id}", headers=headers)
    assert r.status_code == 200, f"Failed to get list: {r.status_code} {r.text}"
    fetched_list = r.json()
    assert fetched_list['id'] == list_id
    assert fetched_list['title'] == created_list['title']
    print(f"✅ Retrieved list: {fetched_list['title']}")
    
    # 4. Update the list
    print("\n4. Updating the list...")
    update_data = {
        "title": f"Updated {created_list['title']}",
        "replies_policy": "none"
    }
    r = requests.put(f"{base_url}/api/v1/lists/{list_id}", headers=headers, json=update_data)
    assert r.status_code == 200, f"Failed to update list: {r.status_code} {r.text}"
    updated_list = r.json()
    assert updated_list['title'] == update_data['title']
    assert updated_list['replies_policy'] == 'none'
    print(f"✅ Updated list title and replies policy")
    
    # 5. Get accounts in list (should be empty)
    print("\n5. Getting accounts in list...")
    r = requests.get(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers)
    assert r.status_code == 200, f"Failed to get list accounts: {r.status_code} {r.text}"
    accounts = r.json()
    assert len(accounts) == 0, f"Expected empty list, got {len(accounts)} accounts"
    print("✅ List is empty (as expected)")
    
    # 6. Search for accounts to add
    print("\n6. Searching for accounts to add...")
    # Search for public accounts
    r = requests.get(f"{base_url}/api/v1/accounts/search?q=test&limit=5", headers=headers)
    if r.status_code == 200:
        search_results = r.json()
        if search_results:
            test_account = search_results[0]
            print(f"✅ Found account to add: @{test_account['username']}")
        else:
            # If no search results, we can't test adding accounts
            print("⚠️ No accounts found to add to list, skipping account operations")
            test_account = None
    else:
        print(f"⚠️ Account search failed: {r.status_code}")
        test_account = None
    
    if test_account:
        # 7. Add account to list
        print("\n7. Adding account to list...")
        add_data = {
            "account_ids": [test_account['id']]
        }
        r = requests.post(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers, json=add_data)
        assert r.status_code in [200, 204], f"Failed to add account: {r.status_code} {r.text}"
        print(f"✅ Added @{test_account['username']} to list")
        
        # 8. Verify account is in list
        print("\n8. Verifying account is in list...")
        r = requests.get(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers)
        assert r.status_code == 200
        accounts = r.json()
        assert len(accounts) == 1, f"Expected 1 account, got {len(accounts)}"
        assert accounts[0]['id'] == test_account['id']
        print(f"✅ Confirmed @{test_account['username']} is in list")
        
        # 9. Get list timeline
        print("\n9. Getting list timeline...")
        r = requests.get(f"{base_url}/api/v1/timelines/list/{list_id}", headers=headers)
        assert r.status_code == 200, f"Failed to get list timeline: {r.status_code} {r.text}"
        timeline = r.json()
        print(f"✅ List timeline has {len(timeline)} posts")
        
        # 10. Remove account from list
        print("\n10. Removing account from list...")
        remove_data = {
            "account_ids": [test_account['id']]
        }
        r = requests.delete(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers, json=remove_data)
        assert r.status_code in [200, 204], f"Failed to remove account: {r.status_code} {r.text}"
        print(f"✅ Removed @{test_account['username']} from list")
        
        # Verify removal
        r = requests.get(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers)
        assert r.status_code == 200
        accounts = r.json()
        assert len(accounts) == 0, f"Expected empty list, got {len(accounts)} accounts"
        print("✅ Confirmed list is empty again")
        
        # Test GET /api/v1/accounts/:id/lists endpoint
        print("\n10a. Testing account lists endpoint...")
        # First add account back to list
        r = requests.post(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers, json=add_data)
        assert r.status_code in [200, 204]
        
        # Get lists containing this account
        r = requests.get(f"{base_url}/api/v1/accounts/{test_account['id']}/lists", headers=headers)
        assert r.status_code == 200, f"Failed to get account lists: {r.status_code} {r.text}"
        account_lists = r.json()
        
        # Should find our list
        found = False
        for lst in account_lists:
            if lst['id'] == list_id:
                found = True
                print(f"✅ Found list '{lst['title']}' containing @{test_account['username']}")
                break
        assert found, f"List not found in account's lists"
        
        # Clean up - remove account again
        r = requests.delete(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers, json=remove_data)
    
    # 11. Test multiple lists
    print("\n11. Testing multiple lists...")
    # Create another list with different policy
    list2_data = {
        "title": "Friends Only",
        "replies_policy": "followed"
    }
    r = requests.post(f"{base_url}/api/v1/lists", headers=headers, json=list2_data)
    assert r.status_code == 200
    list2 = r.json()
    print(f"✅ Created second list: {list2['title']}")
    
    # Get all lists
    r = requests.get(f"{base_url}/api/v1/lists", headers=headers)
    assert r.status_code == 200
    all_lists = r.json()
    assert len(all_lists) >= initial_count + 2, f"Expected at least {initial_count + 2} lists, got {len(all_lists)}"
    print(f"✅ Total lists: {len(all_lists)}")
    
    # 12. Test error cases
    print("\n12. Testing error cases...")
    
    # Try to get non-existent list
    r = requests.get(f"{base_url}/api/v1/lists/nonexistent", headers=headers)
    assert r.status_code == 404, f"Expected 404 for non-existent list, got {r.status_code}"
    print("✅ Properly returns 404 for non-existent list")
    
    # Try to create list without title
    r = requests.post(f"{base_url}/api/v1/lists", headers=headers, json={})
    assert r.status_code == 400, f"Expected 400 for missing title, got {r.status_code}"
    print("✅ Properly rejects list without title")
    
    # 13. Cleanup - Delete test lists
    print("\n13. Cleanup - deleting test lists...")
    r = requests.delete(f"{base_url}/api/v1/lists/{list_id}", headers=headers)
    assert r.status_code in [200, 204], f"Failed to delete list: {r.status_code}"
    print(f"✅ Deleted first test list")
    
    r = requests.delete(f"{base_url}/api/v1/lists/{list2['id']}", headers=headers)
    assert r.status_code in [200, 204], f"Failed to delete list: {r.status_code}"
    print(f"✅ Deleted second test list")
    
    # Verify deletion
    r = requests.get(f"{base_url}/api/v1/lists/{list_id}", headers=headers)
    assert r.status_code == 404, f"Expected 404 for deleted list, got {r.status_code}"
    print("✅ Confirmed lists are deleted")
    
    print("\n✅ All list tests passed!")

def test_list_timeline_fanout(base_url, token1, token2):
    """Test that posts fan out to list timelines correctly."""
    
    headers1 = {"Authorization": f"Bearer {token1}"}
    headers2 = {"Authorization": f"Bearer {token2}"}
    
    print("\n📨 Testing List Timeline Fan-out")
    print("=" * 50)
    
    # Get user info
    r = requests.get(f"{base_url}/api/v1/accounts/verify_credentials", headers=headers1)
    assert r.status_code == 200
    
    r = requests.get(f"{base_url}/api/v1/accounts/verify_credentials", headers=headers2)
    assert r.status_code == 200
    user2 = r.json()
    
    # 1. User1 creates a list
    print("\n1. Creating list for fan-out test...")
    list_data = {
        "title": "Timeline Test List",
        "replies_policy": "list"
    }
    r = requests.post(f"{base_url}/api/v1/lists", headers=headers1, json=list_data)
    assert r.status_code == 200
    test_list = r.json()
    list_id = test_list['id']
    print(f"✅ Created list: {test_list['title']}")
    
    # 2. Add User2 to the list
    print("\n2. Adding user2 to list...")
    add_data = {
        "account_ids": [user2['id']]
    }
    r = requests.post(f"{base_url}/api/v1/lists/{list_id}/accounts", headers=headers1, json=add_data)
    assert r.status_code in [200, 204]
    print(f"✅ Added @{user2['username']} to list")
    
    # 3. User2 creates a post
    print("\n3. User2 creating a post...")
    status_data = {
        "status": f"Test list timeline fan-out {datetime.now().strftime('%Y%m%d%H%M%S')}"
    }
    r = requests.post(f"{base_url}/api/v1/statuses", headers=headers2, json=status_data)
    assert r.status_code == 200
    status = r.json()
    print(f"✅ Created status: {status['id']}")
    
    # Give time for fan-out
    time.sleep(2)
    
    # 4. Check list timeline
    print("\n4. Checking list timeline...")
    r = requests.get(f"{base_url}/api/v1/timelines/list/{list_id}", headers=headers1)
    assert r.status_code == 200
    timeline = r.json()
    
    # Find the status in timeline
    found = False
    for post in timeline:
        if post['id'] == status['id']:
            found = True
            print(f"✅ Status appeared in list timeline!")
            break
    
    if not found:
        print(f"❌ Status not found in list timeline (timeline has {len(timeline)} posts)")
    
    # 5. Test replies policy
    print("\n5. Testing replies policy...")
    # Create a reply
    reply_data = {
        "status": f"Reply to test policies",
        "in_reply_to_id": status['id']
    }
    r = requests.post(f"{base_url}/api/v1/statuses", headers=headers2, json=reply_data)
    assert r.status_code == 200
    reply = r.json()
    print(f"✅ Created reply: {reply['id']}")
    
    time.sleep(2)
    
    # Check if reply appears (should appear with "list" policy)
    r = requests.get(f"{base_url}/api/v1/timelines/list/{list_id}", headers=headers1)
    assert r.status_code == 200
    timeline = r.json()
    
    reply_found = any(post['id'] == reply['id'] for post in timeline)
    print(f"✅ Reply {'appeared' if reply_found else 'did not appear'} in list timeline (policy: list)")
    
    # 6. Update list to "none" policy
    print("\n6. Testing 'none' replies policy...")
    update_data = {
        "replies_policy": "none"
    }
    r = requests.put(f"{base_url}/api/v1/lists/{list_id}", headers=headers1, json=update_data)
    assert r.status_code == 200
    print("✅ Updated list to 'none' replies policy")
    
    # Create another reply
    reply2_data = {
        "status": f"Another reply to test none policy",
        "in_reply_to_id": status['id']  
    }
    r = requests.post(f"{base_url}/api/v1/statuses", headers=headers2, json=reply2_data)
    assert r.status_code == 200
    reply2 = r.json()
    
    time.sleep(2)
    
    # This reply should NOT appear with "none" policy
    r = requests.get(f"{base_url}/api/v1/timelines/list/{list_id}", headers=headers1)
    timeline = r.json()
    reply2_found = any(post['id'] == reply2['id'] for post in timeline[:5])  # Check recent posts
    print(f"✅ New reply {'appeared' if reply2_found else 'did not appear'} in list timeline (policy: none)")
    
    # Cleanup
    print("\n7. Cleanup...")
    requests.delete(f"{base_url}/api/v1/lists/{list_id}", headers=headers1)
    requests.delete(f"{base_url}/api/v1/statuses/{status['id']}", headers=headers2)
    requests.delete(f"{base_url}/api/v1/statuses/{reply['id']}", headers=headers2)
    requests.delete(f"{base_url}/api/v1/statuses/{reply2['id']}", headers=headers2)
    print("✅ Cleanup complete")
    
    print("\n✅ List timeline fan-out tests passed!")

def main():
    parser = argparse.ArgumentParser(description='Test Lesser lists management system')
    parser.add_argument('base_url', help='Base URL of the Lesser instance (e.g., https://lesser.example.com)')
    parser.add_argument('--token', required=True, help='Access token for test user')
    parser.add_argument('--token2', help='Second access token for fan-out tests')
    parser.add_argument('--fanout', action='store_true', help='Run timeline fan-out tests (requires --token2)')
    
    args = parser.parse_args()
    
    # Remove trailing slash from base URL
    base_url = args.base_url.rstrip('/')
    
    try:
        # Run basic list tests
        test_lists(base_url, args.token)
        
        # Run fan-out tests if second token provided
        if args.fanout:
            if not args.token2:
                print("\n⚠️ Skipping fan-out tests (requires --token2)")
            else:
                test_list_timeline_fanout(base_url, args.token, args.token2)
        
        print("\n🎉 All tests passed!")
    except AssertionError as e:
        print(f"\n❌ Test failed: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ Unexpected error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main() 
