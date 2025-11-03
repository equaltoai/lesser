#!/usr/bin/env python3
"""
Test script for Lesser notifications system.

This script tests all notification endpoints:
- GET /api/v1/notifications - List notifications
- GET /api/v1/notifications/:id - Get single notification 
- POST /api/v1/notifications/clear - Clear all notifications
- POST /api/v1/notifications/:id/dismiss - Dismiss single notification

It also tests notification generation for:
- Follow notifications
- Favourite notifications  
- Reblog notifications
- Mention notifications
"""

import requests
import json
import sys
import time
import argparse
from datetime import datetime

def test_notifications(base_url, token1, token2):
    """Test notifications system with two users."""
    
    headers1 = {"Authorization": f"Bearer {token1}"}
    headers2 = {"Authorization": f"Bearer {token2}"}
    
    print("\n🔔 Testing Notifications System")
    print("=" * 50)
    
    # 1. Clear any existing notifications
    print("\n1. Clearing existing notifications...")
    r = requests.post(f"{base_url}/api/v1/notifications/clear", headers=headers1)
    assert r.status_code in [200, 204], f"Failed to clear notifications: {r.status_code} {r.text}"
    print("✅ Cleared user1 notifications")
    
    r = requests.post(f"{base_url}/api/v1/notifications/clear", headers=headers2)
    assert r.status_code in [200, 204], f"Failed to clear notifications: {r.status_code} {r.text}"
    print("✅ Cleared user2 notifications")
    
    # 2. User2 follows User1 (should create follow notification)
    print("\n2. Testing follow notification...")
    r = requests.get(f"{base_url}/api/v1/accounts/verify_credentials", headers=headers1)
    assert r.status_code == 200
    user1 = r.json()
    
    r = requests.get(f"{base_url}/api/v1/accounts/verify_credentials", headers=headers2)
    assert r.status_code == 200
    user2 = r.json()
    
    r = requests.post(f"{base_url}/api/v1/accounts/{user1['id']}/follow", headers=headers2)
    if r.status_code != 200:
        print(f"Follow failed: {r.status_code} {r.text}")
    else:
        print(f"✅ User2 ({user2['username']}) followed User1 ({user1['username']})")
    
    # Give time for notification to be created
    time.sleep(2)
    
    # Check User1's notifications
    r = requests.get(f"{base_url}/api/v1/notifications", headers=headers1)
    assert r.status_code == 200, f"Failed to get notifications: {r.status_code}"
    notifications = r.json()
    
    follow_notif = None
    for n in notifications:
        if n['type'] == 'follow':
            follow_notif = n
            break
    
    if follow_notif:
        print(f"✅ Follow notification created: {follow_notif['account']['username']} followed you")
    else:
        print("❌ No follow notification found")
    
    # 3. User1 creates a status
    print("\n3. Creating status for interaction tests...")
    status_data = {
        "status": f"Test status for notifications @{user2['username']} #{datetime.now().strftime('%Y%m%d%H%M%S')}"
    }
    r = requests.post(f"{base_url}/api/v1/statuses", headers=headers1, json=status_data)
    assert r.status_code == 200, f"Failed to create status: {r.status_code} {r.text}"
    status = r.json()
    print(f"✅ Created status: {status['id']}")
    
    # Give time for mention notification
    time.sleep(2)
    
    # 4. Check User2's notifications for mention
    print("\n4. Testing mention notification...")
    r = requests.get(f"{base_url}/api/v1/notifications", headers=headers2)
    assert r.status_code == 200
    notifications = r.json()
    
    mention_notif = None
    for n in notifications:
        if n['type'] == 'mention':
            mention_notif = n
            break
    
    if mention_notif:
        print(f"✅ Mention notification created: {mention_notif['account']['username']} mentioned you")
        print(f"   Status preview: {mention_notif.get('status', {}).get('content', '')[:50]}...")
    else:
        print("❌ No mention notification found")
    
    # 5. User2 favorites the status
    print("\n5. Testing favourite notification...")
    r = requests.post(f"{base_url}/api/v1/statuses/{status['id']}/favourite", headers=headers2)
    assert r.status_code == 200, f"Failed to favourite: {r.status_code}"
    print(f"✅ User2 favourited the status")
    
    time.sleep(2)
    
    # Check User1's notifications for favourite
    r = requests.get(f"{base_url}/api/v1/notifications", headers=headers1)
    assert r.status_code == 200
    notifications = r.json()
    
    fav_notif = None
    for n in notifications:
        if n['type'] == 'favourite':
            fav_notif = n
            break
    
    if fav_notif:
        print(f"✅ Favourite notification created: {fav_notif['account']['username']} favourited your status")
    else:
        print("❌ No favourite notification found")
    
    # 6. User2 reblogs the status
    print("\n6. Testing reblog notification...")
    r = requests.post(f"{base_url}/api/v1/statuses/{status['id']}/reblog", headers=headers2)
    assert r.status_code == 200, f"Failed to reblog: {r.status_code}"
    print(f"✅ User2 reblogged the status")
    
    time.sleep(2)
    
    # Check User1's notifications for reblog
    r = requests.get(f"{base_url}/api/v1/notifications", headers=headers1)
    assert r.status_code == 200
    notifications = r.json()
    
    reblog_notif = None
    for n in notifications:
        if n['type'] == 'reblog':
            reblog_notif = n
            break
    
    if reblog_notif:
        print(f"✅ Reblog notification created: {reblog_notif['account']['username']} reblogged your status")
    else:
        print("❌ No reblog notification found")
    
    # 7. Test notification filters
    print("\n7. Testing notification filters...")
    
    # Filter by type
    r = requests.get(f"{base_url}/api/v1/notifications?types[]=follow", headers=headers1)
    assert r.status_code == 200
    filtered = r.json()
    assert all(n['type'] == 'follow' for n in filtered), "Filter by type failed"
    print(f"✅ Filter by type working: {len(filtered)} follow notifications")
    
    # Exclude types
    r = requests.get(f"{base_url}/api/v1/notifications?exclude_types[]=follow", headers=headers1)
    assert r.status_code == 200
    filtered = r.json()
    assert all(n['type'] != 'follow' for n in filtered), "Exclude types failed"
    print(f"✅ Exclude types working: {len(filtered)} non-follow notifications")
    
    # 8. Test getting single notification
    print("\n8. Testing single notification retrieval...")
    if notifications:
        test_notif = notifications[0]
        r = requests.get(f"{base_url}/api/v1/notifications/{test_notif['id']}", headers=headers1)
        assert r.status_code == 200, f"Failed to get notification: {r.status_code}"
        single = r.json()
        assert single['id'] == test_notif['id'], "Retrieved wrong notification"
        print(f"✅ Retrieved single notification: {single['type']} from {single['account']['username']}")
    
    # 9. Test dismissing single notification
    print("\n9. Testing dismiss single notification...")
    if notifications:
        dismiss_id = notifications[0]['id']
        r = requests.post(f"{base_url}/api/v1/notifications/{dismiss_id}/dismiss", headers=headers1)
        assert r.status_code in [200, 204], f"Failed to dismiss: {r.status_code}"
        print(f"✅ Dismissed notification {dismiss_id}")
        
        # Verify it's gone
        r = requests.get(f"{base_url}/api/v1/notifications/{dismiss_id}", headers=headers1)
        assert r.status_code == 404, "Notification still exists after dismiss"
        print("✅ Verified notification was deleted")
    
    # 10. Test pagination
    print("\n10. Testing pagination...")
    r = requests.get(f"{base_url}/api/v1/notifications?limit=2", headers=headers1)
    assert r.status_code == 200
    limited = r.json()
    assert len(limited) <= 2, f"Limit not respected: got {len(limited)} notifications"
    print(f"✅ Pagination limit working: {len(limited)} notifications")
    
    # Check for Link header
    if 'link' in r.headers:
        print(f"✅ Link header present: {r.headers['link'][:50]}...")
    
    # 11. Test clearing all notifications
    print("\n11. Testing clear all notifications...")
    r = requests.post(f"{base_url}/api/v1/notifications/clear", headers=headers1)
    assert r.status_code in [200, 204], f"Failed to clear: {r.status_code}"
    print("✅ Cleared all notifications")
    
    # Verify they're gone
    r = requests.get(f"{base_url}/api/v1/notifications", headers=headers1)
    assert r.status_code == 200
    remaining = r.json()
    assert len(remaining) == 0, f"Still have {len(remaining)} notifications after clear"
    print("✅ Verified all notifications cleared")
    
    print("\n✅ All notification tests passed!")
    
    # Cleanup
    print("\n12. Cleanup...")
    # Unfollow
    requests.post(f"{base_url}/api/v1/accounts/{user1['id']}/unfollow", headers=headers2)
    # Delete status
    requests.delete(f"{base_url}/api/v1/statuses/{status['id']}", headers=headers1)
    print("✅ Cleanup complete")

def main():
    parser = argparse.ArgumentParser(description='Test Lesser notifications system')
    parser.add_argument('base_url', help='Base URL of the Lesser instance (e.g., https://lesser.example.com)')
    parser.add_argument('--token1', required=True, help='Access token for first test user')
    parser.add_argument('--token2', required=True, help='Access token for second test user')
    
    args = parser.parse_args()
    
    # Remove trailing slash from base URL
    base_url = args.base_url.rstrip('/')
    
    try:
        test_notifications(base_url, args.token1, args.token2)
        print("\n🎉 All tests passed!")
    except AssertionError as e:
        print(f"\n❌ Test failed: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ Unexpected error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main() 
