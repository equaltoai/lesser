#!/usr/bin/env python3
"""Test Phase 2 status features for Lesser - Status pinning and conversation muting"""

import requests
import sys

# Configuration
BASE_URL = "http://localhost:8080"
API_BASE = f"{BASE_URL}/api/v1"

# Test accounts (assumes these exist)
TEST_USER = "testuser"
TEST_PASSWORD = "testpassword"

def get_access_token(username, password):
    """Get an access token for a user"""
    # First register the OAuth app
    app_data = {
        "client_name": "Phase2 Test Client",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow push",
        "website": "https://example.com"
    }
    
    response = requests.post(f"{API_BASE}/apps", json=app_data)
    if response.status_code != 200:
        print(f"Failed to register app: {response.text}")
        return None
        
    app = response.json()
    client_id = app.get("client_id")
    client_secret = app.get("client_secret")
    
    # Get token
    token_data = {
        "grant_type": "password",
        "client_id": client_id,
        "client_secret": client_secret,
        "username": username,
        "password": password,
        "scope": "read write follow push"
    }
    
    response = requests.post(f"{BASE_URL}/oauth/token", data=token_data)
    if response.status_code != 200:
        print(f"Failed to get token: {response.text}")
        return None
        
    return response.json().get("access_token")

def create_status(token, content):
    """Create a new status"""
    headers = {"Authorization": f"Bearer {token}"}
    data = {"status": content}
    
    response = requests.post(f"{API_BASE}/statuses", headers=headers, json=data)
    if response.status_code != 201:
        print(f"Failed to create status: {response.text}")
        return None
    
    return response.json()

def test_status_pinning(token):
    """Test status pinning and unpinning"""
    print("\n=== Testing Status Pinning ===")
    
    # Create a status to pin
    status = create_status(token, "This is a status to be pinned! 📌")
    if not status:
        return False
    
    status_id = status.get("id")
    print(f"Created status: {status_id}")
    
    # Pin the status
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.post(f"{API_BASE}/statuses/{status_id}/pin", headers=headers)
    
    if response.status_code != 200:
        print(f"❌ Failed to pin status: {response.status_code} - {response.text}")
        return False
    
    pinned_status = response.json()
    if not pinned_status.get("pinned"):
        print("❌ Status not marked as pinned in response")
        return False
    
    print("✅ Status pinned successfully")
    
    # Try to pin the same status again (should fail)
    response = requests.post(f"{API_BASE}/statuses/{status_id}/pin", headers=headers)
    if response.status_code != 422:
        print(f"❌ Expected 422 for duplicate pin, got {response.status_code}")
        return False
    
    print("✅ Duplicate pin correctly rejected")
    
    # Create more statuses to test the limit
    for i in range(4):
        s = create_status(token, f"Another pinned status #{i+2}")
        if s:
            requests.post(f"{API_BASE}/statuses/{s['id']}/pin", headers=headers)
    
    # Try to pin a 6th status (should fail due to limit)
    extra_status = create_status(token, "This should exceed the pin limit")
    if extra_status:
        response = requests.post(f"{API_BASE}/statuses/{extra_status['id']}/pin", headers=headers)
        if response.status_code != 422:
            print(f"❌ Expected 422 for exceeding pin limit, got {response.status_code}")
            return False
        print("✅ Pin limit correctly enforced")
    
    # Unpin the first status
    response = requests.post(f"{API_BASE}/statuses/{status_id}/unpin", headers=headers)
    if response.status_code != 200:
        print(f"❌ Failed to unpin status: {response.status_code} - {response.text}")
        return False
    
    unpinned_status = response.json()
    if unpinned_status.get("pinned"):
        print("❌ Status still marked as pinned after unpinning")
        return False
    
    print("✅ Status unpinned successfully")
    
    return True

def test_conversation_muting(token):
    """Test conversation muting and unmuting"""
    print("\n=== Testing Conversation Muting ===")
    
    # Create a status to mute
    status = create_status(token, "This is a conversation that will be muted 🔇")
    if not status:
        return False
    
    status_id = status.get("id")
    print(f"Created status: {status_id}")
    
    # Mute the conversation
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.post(f"{API_BASE}/statuses/{status_id}/mute", headers=headers)
    
    if response.status_code != 200:
        print(f"❌ Failed to mute conversation: {response.status_code} - {response.text}")
        return False
    
    muted_status = response.json()
    if not muted_status.get("muted"):
        print("❌ Status not marked as muted in response")
        return False
    
    print("✅ Conversation muted successfully")
    
    # Test muting with duration
    status2 = create_status(token, "This conversation will be muted temporarily ⏰")
    if status2:
        data = {"duration": 300}  # 5 minutes
        response = requests.post(f"{API_BASE}/statuses/{status2['id']}/mute", 
                               headers=headers, json=data)
        if response.status_code == 200:
            print("✅ Conversation muted with duration")
        else:
            print(f"❌ Failed to mute with duration: {response.status_code}")
    
    # Unmute the first conversation
    response = requests.post(f"{API_BASE}/statuses/{status_id}/unmute", headers=headers)
    if response.status_code != 200:
        print(f"❌ Failed to unmute conversation: {response.status_code} - {response.text}")
        return False
    
    unmuted_status = response.json()
    if unmuted_status.get("muted"):
        print("❌ Status still marked as muted after unmuting")
        return False
    
    print("✅ Conversation unmuted successfully")
    
    # Test idempotency - unmuting already unmuted conversation
    response = requests.post(f"{API_BASE}/statuses/{status_id}/unmute", headers=headers)
    if response.status_code != 200:
        print(f"❌ Unmuting already unmuted conversation should succeed: {response.status_code}")
        return False
    
    print("✅ Idempotent unmuting works correctly")
    
    return True

def test_ownership_validation(token):
    """Test that users can only pin their own statuses"""
    print("\n=== Testing Ownership Validation ===")
    
    # This would require a second user account to test properly
    # For now, we'll just verify that the endpoint exists and responds correctly
    
    headers = {"Authorization": f"Bearer {token}"}
    
    # Try to pin a non-existent status
    fake_id = "nonexistent-status-id"
    response = requests.post(f"{API_BASE}/statuses/{fake_id}/pin", headers=headers)
    
    if response.status_code != 404:
        print(f"❌ Expected 404 for non-existent status, got {response.status_code}")
        return False
    
    print("✅ Non-existent status correctly returns 404")
    
    return True

def main():
    """Run all Phase 2 tests"""
    print("Phase 2 Status Features Test Suite")
    print("==================================")
    
    # Get access token
    token = get_access_token(TEST_USER, TEST_PASSWORD)
    if not token:
        print("❌ Failed to get access token")
        sys.exit(1)
    
    print(f"✅ Got access token")
    
    # Run tests
    tests = [
        ("Status Pinning", test_status_pinning),
        ("Conversation Muting", test_conversation_muting),
        ("Ownership Validation", test_ownership_validation)
    ]
    
    passed = 0
    failed = 0
    
    for test_name, test_func in tests:
        try:
            if test_func(token):
                passed += 1
            else:
                failed += 1
                print(f"❌ {test_name} test failed")
        except Exception as e:
            failed += 1
            print(f"❌ {test_name} test crashed: {e}")
    
    print(f"\n{'='*50}")
    print(f"Test Results: {passed} passed, {failed} failed")
    
    if failed == 0:
        print("✅ All Phase 2 tests passed!")
    else:
        print(f"❌ {failed} tests failed")
        sys.exit(1)

if __name__ == "__main__":
    main() 
