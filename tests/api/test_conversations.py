#!/usr/bin/env python3
"""Test script for conversation threading functionality"""

import requests
import time
import sys

# Configuration
BASE_URL = "https://aron.lesser.id"
ACCESS_TOKEN = None  # Will be set after login

def login():
    """Login and get access token"""
    global ACCESS_TOKEN
    
    # First, create an OAuth app
    app_response = requests.post(f"{BASE_URL}/api/v1/apps", json={
        "client_name": "Conversation Test",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow"
    })
    
    if app_response.status_code != 200:
        print(f"Failed to create app: {app_response.status_code}")
        print(app_response.text)
        sys.exit(1)
    
    app_data = app_response.json()
    client_id = app_data["client_id"]
    client_secret = app_data["client_secret"]
    
    # Get access token
    token_response = requests.post(f"{BASE_URL}/oauth/token", json={
        "grant_type": "password",
        "client_id": client_id,
        "client_secret": client_secret,
        "username": "aron",  # Update with your username
        "password": "test123",  # Update with your password
        "scope": "read write follow"
    })
    
    if token_response.status_code != 200:
        print(f"Failed to get token: {token_response.status_code}")
        print(token_response.text)
        sys.exit(1)
    
    ACCESS_TOKEN = token_response.json()["access_token"]
    print("✅ Logged in successfully")

def create_post(content, in_reply_to_id=None):
    """Create a new post"""
    headers = {"Authorization": f"Bearer {ACCESS_TOKEN}"}
    data = {
        "status": content,
        "visibility": "public"
    }
    
    if in_reply_to_id:
        data["in_reply_to_id"] = in_reply_to_id
    
    response = requests.post(f"{BASE_URL}/api/v1/statuses", 
                           headers=headers, 
                           json=data)
    
    if response.status_code != 201:
        print(f"Failed to create post: {response.status_code}")
        print(response.text)
        return None
    
    return response.json()

def get_conversations():
    """Get all conversations"""
    headers = {"Authorization": f"Bearer {ACCESS_TOKEN}"}
    response = requests.get(f"{BASE_URL}/api/v1/conversations", headers=headers)
    
    if response.status_code != 200:
        print(f"Failed to get conversations: {response.status_code}")
        print(response.text)
        return []
    
    return response.json()

def delete_conversation(conversation_id):
    """Delete a conversation"""
    headers = {"Authorization": f"Bearer {ACCESS_TOKEN}"}
    response = requests.delete(f"{BASE_URL}/api/v1/conversations/{conversation_id}", 
                             headers=headers)
    
    return response.status_code == 200

def mark_conversation_read(conversation_id):
    """Mark a conversation as read"""
    headers = {"Authorization": f"Bearer {ACCESS_TOKEN}"}
    response = requests.post(f"{BASE_URL}/api/v1/conversations/{conversation_id}/read", 
                           headers=headers)
    
    return response.status_code == 200

def test_conversation_threading():
    """Test conversation threading functionality"""
    print("\n🧪 Testing Conversation Threading")
    print("=" * 50)
    
    # Step 1: Create a root post
    print("\n1️⃣  Creating root post...")
    root_post = create_post("This is the start of a conversation thread!")
    if not root_post:
        print("❌ Failed to create root post")
        return
    
    root_id = root_post["id"]
    print(f"✅ Created root post: {root_id}")
    
    # Give the system a moment to process
    time.sleep(1)
    
    # Step 2: Create replies
    print("\n2️⃣  Creating replies...")
    reply1 = create_post("This is the first reply!", root_id)
    if not reply1:
        print("❌ Failed to create first reply")
        return
    print(f"✅ Created reply 1: {reply1['id']}")
    
    time.sleep(0.5)
    
    reply2 = create_post("This is the second reply!", root_id)
    if not reply2:
        print("❌ Failed to create second reply")
        return
    print(f"✅ Created reply 2: {reply2['id']}")
    
    time.sleep(0.5)
    
    # Create a nested reply
    nested_reply = create_post("This is a reply to the first reply!", reply1['id'])
    if not nested_reply:
        print("❌ Failed to create nested reply")
        return
    print(f"✅ Created nested reply: {nested_reply['id']}")
    
    # Give the system time to process
    time.sleep(2)
    
    # Step 3: Check conversations
    print("\n3️⃣  Checking conversations...")
    conversations = get_conversations()
    
    if not conversations:
        print("❌ No conversations found")
        return
    
    print(f"✅ Found {len(conversations)} conversation(s)")
    
    # Find our conversation
    our_conversation = None
    for conv in conversations:
        if conv.get("last_status", {}).get("id") == nested_reply["id"]:
            our_conversation = conv
            break
    
    if not our_conversation:
        print("❌ Could not find our conversation")
        print("Available conversations:")
        for conv in conversations:
            print(f"  - ID: {conv['id']}, Last status: {conv.get('last_status', {}).get('content', 'N/A')}")
        return
    
    print(f"✅ Found our conversation: {our_conversation['id']}")
    print(f"   Unread: {our_conversation['unread']}")
    print(f"   Participants: {len(our_conversation['accounts'])}")
    print(f"   Last status: {our_conversation['last_status']['content']}")
    
    # Step 4: Mark as read
    print("\n4️⃣  Marking conversation as read...")
    if mark_conversation_read(our_conversation['id']):
        print("✅ Marked conversation as read")
    else:
        print("❌ Failed to mark conversation as read")
    
    # Step 5: Delete conversation
    print("\n5️⃣  Testing conversation deletion...")
    if delete_conversation(our_conversation['id']):
        print("✅ Deleted conversation")
    else:
        print("❌ Failed to delete conversation")
    
    # Verify deletion
    time.sleep(1)
    conversations_after = get_conversations()
    found = any(c['id'] == our_conversation['id'] for c in conversations_after)
    
    if not found:
        print("✅ Conversation successfully removed from list")
    else:
        print("❌ Conversation still appears in list after deletion")
    
    print("\n✅ Conversation threading test complete!")

def main():
    """Main function"""
    print("🔧 Conversation Threading Test Suite")
    print("=" * 50)
    
    # Login
    login()
    
    # Run tests
    test_conversation_threading()
    
    print("\n✨ All tests completed!")

if __name__ == "__main__":
    main() 
