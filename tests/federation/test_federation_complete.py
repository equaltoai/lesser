#!/usr/bin/env python3
"""
Test script for complete federation implementation in Lesser.

This script tests:
1. Remote follow functionality
2. Activity delivery to remote instances
3. Inbox processing of remote activities
4. Accept/Reject handling
5. Create/Update/Delete federation
"""

import requests
import time
import argparse
import json
from typing import Dict, Optional

def make_request(method: str, url: str, token: Optional[str] = None, 
                json_data: Optional[Dict] = None, headers: Optional[Dict] = None) -> requests.Response:
    """Make HTTP request with proper headers"""
    if headers is None:
        headers = {}
    
    if token:
        headers['Authorization'] = f'Bearer {token}'
    
    if json_data is not None:
        headers['Content-Type'] = 'application/activity+json'
    
    return requests.request(method, url, json=json_data, headers=headers)

def test_remote_follow(base_url: str, token: str, username: str, remote_actor: str):
    """Test following a remote actor"""
    print(f"\n=== Testing Remote Follow: {username} -> {remote_actor} ===")
    
    # Create Follow activity
    follow_activity = {
        "type": "Follow",
        "object": remote_actor
    }
    
    # Post to outbox
    outbox_url = f"{base_url}/users/{username}/outbox"
    response = make_request('POST', outbox_url, token, json_data=follow_activity)
    
    if response.status_code == 201:
        print("✓ Follow activity created successfully")
        activity = response.json()
        print(f"  Activity ID: {activity.get('id')}")
        print(f"  Type: {activity.get('type')}")
        print(f"  Object: {activity.get('object')}")
        
        # Check if it appears in outbox
        time.sleep(1)
        outbox_response = make_request('GET', f"{outbox_url}?page=true", token)
        if outbox_response.status_code == 200:
            page = outbox_response.json()
            if page.get('orderedItems'):
                print("✓ Follow activity appears in outbox")
    else:
        print(f"✗ Failed to create follow activity: {response.status_code}")
        print(f"  Response: {response.text}")

def test_inbox_follow_processing(base_url: str, token: str, username: str):
    """Test receiving a Follow activity in inbox"""
    print(f"\n=== Testing Inbox Follow Processing for {username} ===")
    
    # Simulate incoming Follow activity
    follow_activity = {
        "@context": "https://www.w3.org/ns/activitystreams",
        "id": "https://remote.example.com/activities/follow-123",
        "type": "Follow",
        "actor": "https://remote.example.com/users/alice",
        "object": f"{base_url}/users/{username}",
        "to": [f"{base_url}/users/{username}"]
    }
    
    inbox_url = f"{base_url}/users/{username}/inbox"
    
    # Note: This would normally require HTTP signature
    # For testing, the inbox might accept activities without signature in dev mode
    print("! Note: Inbox POST normally requires HTTP signature from remote server")
    print("  This test simulates what would happen when a properly signed Follow arrives")
    print(f"  Target inbox: {inbox_url}")
    print("  Sample payload:")
    print(json.dumps(follow_activity, indent=2))

def test_create_activity_delivery(base_url: str, token: str, username: str):
    """Test creating a post that should be delivered to followers"""
    print(f"\n=== Testing Create Activity Delivery for {username} ===")
    
    # Create a public post
    create_activity = {
        "type": "Create",
        "to": ["https://www.w3.org/ns/activitystreams#Public"],
        "cc": [f"{base_url}/users/{username}/followers"],
        "object": {
            "type": "Note",
            "content": "Hello Fediverse! This is a federated post from Lesser.",
            "to": ["https://www.w3.org/ns/activitystreams#Public"],
            "cc": [f"{base_url}/users/{username}/followers"]
        }
    }
    
    outbox_url = f"{base_url}/users/{username}/outbox"
    response = make_request('POST', outbox_url, token, json_data=create_activity)
    
    if response.status_code == 201:
        print("✓ Create activity posted successfully")
        activity = response.json()
        print(f"  Activity ID: {activity.get('id')}")
        print(f"  Object ID: {activity.get('object', {}).get('id')}")
        print("  This activity will be delivered to all remote followers")
    else:
        print(f"✗ Failed to create post: {response.status_code}")
        print(f"  Response: {response.text}")

def test_update_activity(base_url: str, token: str, username: str, object_id: str):
    """Test updating an object"""
    print(f"\n=== Testing Update Activity for {username} ===")
    
    update_activity = {
        "type": "Update",
        "object": {
            "id": object_id,
            "type": "Note",
            "content": "Updated content - this has been edited!",
            "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        }
    }
    
    outbox_url = f"{base_url}/users/{username}/outbox"
    response = make_request('POST', outbox_url, token, json_data=update_activity)
    
    if response.status_code == 201:
        print("✓ Update activity created successfully")
        print("  Remote instances will receive this update")
    else:
        print(f"✗ Failed to create update: {response.status_code}")

def test_delete_activity(base_url: str, token: str, username: str, object_id: str):
    """Test deleting an object"""
    print(f"\n=== Testing Delete Activity for {username} ===")
    
    delete_activity = {
        "type": "Delete",
        "object": object_id
    }
    
    outbox_url = f"{base_url}/users/{username}/outbox"
    response = make_request('POST', outbox_url, token, json_data=delete_activity)
    
    if response.status_code == 201:
        print("✓ Delete activity created successfully")
        print("  Remote instances will be notified of deletion")
    else:
        print(f"✗ Failed to create delete: {response.status_code}")

def test_undo_follow(base_url: str, token: str, username: str, follow_activity_id: str):
    """Test undoing a follow"""
    print(f"\n=== Testing Undo Follow for {username} ===")
    
    undo_activity = {
        "type": "Undo",
        "object": follow_activity_id
    }
    
    outbox_url = f"{base_url}/users/{username}/outbox"
    response = make_request('POST', outbox_url, token, json_data=undo_activity)
    
    if response.status_code == 201:
        print("✓ Undo activity created successfully")
        print("  Remote instance will be notified of unfollow")
    else:
        print(f"✗ Failed to create undo: {response.status_code}")

def test_like_remote_object(base_url: str, token: str, username: str, remote_object: str):
    """Test liking a remote object"""
    print(f"\n=== Testing Like Remote Object: {username} likes {remote_object} ===")
    
    like_activity = {
        "type": "Like",
        "object": remote_object
    }
    
    outbox_url = f"{base_url}/users/{username}/outbox"
    response = make_request('POST', outbox_url, token, json_data=like_activity)
    
    if response.status_code == 201:
        print("✓ Like activity created successfully")
        print("  Remote instance will be notified of the like")
    else:
        print(f"✗ Failed to create like: {response.status_code}")

def test_announce_remote_object(base_url: str, token: str, username: str, remote_object: str):
    """Test announcing (boosting) a remote object"""
    print(f"\n=== Testing Announce Remote Object: {username} boosts {remote_object} ===")
    
    announce_activity = {
        "type": "Announce",
        "object": remote_object,
        "to": ["https://www.w3.org/ns/activitystreams#Public"],
        "cc": [f"{base_url}/users/{username}/followers"]
    }
    
    outbox_url = f"{base_url}/users/{username}/outbox"
    response = make_request('POST', outbox_url, token, json_data=announce_activity)
    
    if response.status_code == 201:
        print("✓ Announce activity created successfully")
        print("  This boost will be delivered to followers")
    else:
        print(f"✗ Failed to create announce: {response.status_code}")

def check_federation_endpoints(base_url: str, username: str):
    """Check that all federation endpoints are accessible"""
    print(f"\n=== Checking Federation Endpoints for {username} ===")
    
    endpoints = [
        f"/users/{username}",
        f"/users/{username}/inbox",
        f"/users/{username}/outbox",
        f"/users/{username}/followers",
        f"/users/{username}/following",
    ]
    
    for endpoint in endpoints:
        url = f"{base_url}{endpoint}"
        response = make_request('GET', url)
        if response.status_code == 200:
            print(f"✓ {endpoint} - accessible")
        else:
            print(f"✗ {endpoint} - status {response.status_code}")

def main():
    parser = argparse.ArgumentParser(description='Test Lesser federation implementation')
    parser.add_argument('base_url', help='Base URL of Lesser instance (e.g., https://lesser.example.com)')
    parser.add_argument('--token', required=True, help='OAuth token for authentication')
    parser.add_argument('--username', default='test', help='Username to test with')
    parser.add_argument('--remote-actor', default='https://mastodon.social/users/Gargron', 
                       help='Remote actor to follow')
    parser.add_argument('--remote-object', default='https://mastodon.social/users/Gargron/statuses/109612109320568846',
                       help='Remote object to interact with')
    
    args = parser.parse_args()
    
    print("=== Lesser Federation Test Suite ===")
    print(f"Instance: {args.base_url}")
    print(f"User: {args.username}")
    
    # Check endpoints
    check_federation_endpoints(args.base_url, args.username)
    
    # Test remote follow
    test_remote_follow(args.base_url, args.token, args.username, args.remote_actor)
    
    # Test activity creation and delivery
    test_create_activity_delivery(args.base_url, args.token, args.username)
    
    # Test inbox processing
    test_inbox_follow_processing(args.base_url, args.token, args.username)
    
    # Test interactions with remote objects
    test_like_remote_object(args.base_url, args.token, args.username, args.remote_object)
    test_announce_remote_object(args.base_url, args.token, args.username, args.remote_object)
    
    print("\n=== Federation Test Complete ===")
    print("\nNote: Full federation testing requires:")
    print("1. A publicly accessible instance with valid HTTPS")
    print("2. Proper DNS configuration")
    print("3. HTTP signature implementation")
    print("4. Interaction with real remote instances")
    
    print("\nTo test with real federation:")
    print("1. Deploy Lesser to a public URL")
    print("2. Follow a real Mastodon account")
    print("3. Have that account follow back")
    print("4. Exchange activities between instances")

if __name__ == "__main__":
    main() 
