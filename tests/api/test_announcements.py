#!/usr/bin/env python3
"""
Test script for announcements functionality in Lesser
"""

import requests
import json
import time
from datetime import datetime, timedelta, timezone

# Configuration
BASE_URL = "http://localhost:8080"
ADMIN_TOKEN = None  # Will be set after login
USER_TOKEN = None   # Will be set after login

def create_test_users():
    """Create test admin and regular user accounts"""
    global ADMIN_TOKEN, USER_TOKEN
    
    # Create admin user
    admin_data = {
        "username": "testadmin",
        "email": "admin@test.com", 
        "password": "testpass123"
    }
    
    # Register admin (would need to be promoted to admin role separately)
    resp = requests.post(f"{BASE_URL}/api/v1/accounts", json=admin_data)
    print(f"Admin registration: {resp.status_code}")
    
    # Create regular user
    user_data = {
        "username": "testuser",
        "email": "user@test.com",
        "password": "testpass123" 
    }
    
    resp = requests.post(f"{BASE_URL}/api/v1/accounts", json=user_data)
    print(f"User registration: {resp.status_code}")
    
    # Login as admin (simplified - you'd need proper OAuth flow)
    # For testing, we'll assume tokens are available
    ADMIN_TOKEN = "admin-test-token"
    USER_TOKEN = "user-test-token"

def test_get_announcements_unauthenticated():
    """Test getting announcements without authentication"""
    print("\n=== Testing GET /api/v1/announcements (unauthenticated) ===")
    
    resp = requests.get(f"{BASE_URL}/api/v1/announcements")
    print(f"Status: {resp.status_code}")
    print(f"Response: {json.dumps(resp.json(), indent=2)}")
    
    assert resp.status_code == 200
    assert isinstance(resp.json(), list)

def test_get_announcements_authenticated():
    """Test getting announcements with authentication"""
    print("\n=== Testing GET /api/v1/announcements (authenticated) ===")
    
    headers = {"Authorization": f"Bearer {USER_TOKEN}"}
    resp = requests.get(f"{BASE_URL}/api/v1/announcements", headers=headers)
    print(f"Status: {resp.status_code}")
    print(f"Response: {json.dumps(resp.json(), indent=2)}")
    
    assert resp.status_code == 200
    assert isinstance(resp.json(), list)

def test_create_announcement():
    """Test creating an announcement as admin"""
    print("\n=== Testing POST /api/v1/admin/announcements ===")
    
    # Create a basic announcement
    announcement_data = {
        "content": "<p>Welcome to Lesser! This is our first announcement.</p>",
        "text": "Welcome to Lesser! This is our first announcement.",
        "all_day": True
    }
    
    headers = {"Authorization": f"Bearer {ADMIN_TOKEN}"}
    resp = requests.post(
        f"{BASE_URL}/api/v1/admin/announcements",
        json=announcement_data,
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    if resp.status_code == 200:
        announcement = resp.json()
        print(f"Created announcement: {json.dumps(announcement, indent=2)}")
        return announcement["id"]
    else:
        print(f"Error: {resp.text}")
        return None

def test_create_timed_announcement():
    """Test creating an announcement with start/end times"""
    print("\n=== Testing timed announcement ===")
    
    # Create announcement that starts in 1 hour and ends in 2 hours
    start_time = (datetime.now(timezone.utc) + timedelta(hours=1)).isoformat()
    end_time = (datetime.now(timezone.utc) + timedelta(hours=2)).isoformat()
    
    announcement_data = {
        "content": "<p>Limited time announcement!</p>",
        "text": "Limited time announcement!",
        "all_day": False,
        "starts_at": start_time,
        "ends_at": end_time
    }
    
    headers = {"Authorization": f"Bearer {ADMIN_TOKEN}"}
    resp = requests.post(
        f"{BASE_URL}/api/v1/admin/announcements",
        json=announcement_data,
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    if resp.status_code == 200:
        print(f"Created timed announcement: {json.dumps(resp.json(), indent=2)}")

def test_dismiss_announcement(announcement_id):
    """Test dismissing an announcement"""
    print(f"\n=== Testing POST /api/v1/announcements/{announcement_id}/dismiss ===")
    
    headers = {"Authorization": f"Bearer {USER_TOKEN}"}
    resp = requests.post(
        f"{BASE_URL}/api/v1/announcements/{announcement_id}/dismiss",
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    assert resp.status_code == 200
    
    # Verify it's not shown anymore
    resp = requests.get(f"{BASE_URL}/api/v1/announcements", headers=headers)
    announcements = resp.json()
    
    # Should not contain the dismissed announcement
    dismissed_ids = [a["id"] for a in announcements]
    assert announcement_id not in dismissed_ids
    print("✓ Announcement successfully dismissed")

def test_add_reaction(announcement_id):
    """Test adding a reaction to an announcement"""
    print(f"\n=== Testing PUT /api/v1/announcements/{announcement_id}/reactions/👍 ===")
    
    headers = {"Authorization": f"Bearer {USER_TOKEN}"}
    resp = requests.put(
        f"{BASE_URL}/api/v1/announcements/{announcement_id}/reactions/👍",
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    assert resp.status_code == 200
    
    # Verify reaction was added
    resp = requests.get(f"{BASE_URL}/api/v1/announcements", headers=headers)
    announcements = resp.json()
    
    for announcement in announcements:
        if announcement["id"] == announcement_id:
            # Find the thumbs up reaction
            for reaction in announcement["reactions"]:
                if reaction["name"] == "👍":
                    assert reaction["count"] >= 1
                    assert reaction["me"] == True
                    print("✓ Reaction successfully added")
                    return
    
    print("✗ Announcement not found after adding reaction")

def test_remove_reaction(announcement_id):
    """Test removing a reaction from an announcement"""
    print(f"\n=== Testing DELETE /api/v1/announcements/{announcement_id}/reactions/👍 ===")
    
    headers = {"Authorization": f"Bearer {USER_TOKEN}"}
    resp = requests.delete(
        f"{BASE_URL}/api/v1/announcements/{announcement_id}/reactions/👍",
        headers=headers
    )
    
    print(f"Status: {resp.status_code}")
    assert resp.status_code == 200
    
    # Verify reaction was removed
    resp = requests.get(f"{BASE_URL}/api/v1/announcements", headers=headers)
    announcements = resp.json()
    
    for announcement in announcements:
        if announcement["id"] == announcement_id:
            # Find the thumbs up reaction
            for reaction in announcement["reactions"]:
                if reaction["name"] == "👍":
                    assert reaction["me"] == False
                    print("✓ Reaction successfully removed")
                    return

def test_announcement_errors():
    """Test error cases"""
    print("\n=== Testing error cases ===")
    
    # Try to dismiss non-existent announcement
    headers = {"Authorization": f"Bearer {USER_TOKEN}"}
    resp = requests.post(
        f"{BASE_URL}/api/v1/announcements/nonexistent/dismiss",
        headers=headers
    )
    print(f"Dismiss non-existent: {resp.status_code} (should be 404)")
    assert resp.status_code == 404
    
    # Try to add reaction without auth
    resp = requests.put(
        f"{BASE_URL}/api/v1/announcements/test/reactions/👍"
    )
    print(f"React without auth: {resp.status_code} (should be 401)")
    assert resp.status_code == 401
    
    # Try to create announcement without admin
    headers = {"Authorization": f"Bearer {USER_TOKEN}"}
    resp = requests.post(
        f"{BASE_URL}/api/v1/admin/announcements",
        json={"content": "test"},
        headers=headers
    )
    print(f"Create without admin: {resp.status_code} (should be 403)")
    assert resp.status_code == 403

def main():
    """Run all announcement tests"""
    print("=== Lesser Announcements Test Suite ===")
    
    # Set up test users
    # create_test_users()
    
    # Test unauthenticated access
    test_get_announcements_unauthenticated()
    
    # For authenticated tests, you'd need proper OAuth tokens
    # These are placeholder tests showing the expected flow
    
    print("\n✓ All announcement tests completed!")
    print("\nNote: For full testing, you need:")
    print("1. A running Lesser instance")
    print("2. Admin user with proper role")
    print("3. OAuth tokens for authentication")
    print("4. DynamoDB table with proper configuration")

if __name__ == "__main__":
    main() 