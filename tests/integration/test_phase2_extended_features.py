#!/usr/bin/env python3
"""Test Phase 2 extended features for Lesser - Status source, history, and scheduled statuses"""

import requests
import time
import json
import sys
from datetime import datetime, timedelta

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
        "client_name": "Phase2 Extended Test Client",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow push",
        "website": "https://example.com"
    }
    
    response = requests.post(f"{API_BASE}/apps", json=app_data)
    if response.status_code != 200:
        print(f"Failed to register app: {response.status_code} - {response.text}")
        return None
    
    app_creds = response.json()
    
    # Get token
    token_data = {
        "grant_type": "password",
        "client_id": app_creds["client_id"],
        "client_secret": app_creds["client_secret"],
        "username": username,
        "password": password,
        "scope": "read write follow push"
    }
    
    response = requests.post(f"{BASE_URL}/oauth/token", data=token_data)
    if response.status_code != 200:
        print(f"Failed to get token: {response.status_code} - {response.text}")
        return None
    
    return response.json()["access_token"]

def test_status_source(token):
    """Test status source endpoint"""
    print("\n=== Testing Status Source ===")
    
    # Create a status
    status_data = {
        "status": "This is a test status with **markdown**",
        "spoiler_text": "Test Warning",
        "sensitive": True
    }
    
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.post(f"{API_BASE}/statuses", json=status_data, headers=headers)
    if response.status_code != 200:
        print(f"Failed to create status: {response.status_code} - {response.text}")
        return False
    
    status = response.json()
    status_id = status["id"]
    print(f"Created status: {status_id}")
    
    # Get status source
    response = requests.get(f"{API_BASE}/statuses/{status_id}/source", headers=headers)
    if response.status_code != 200:
        print(f"Failed to get status source: {response.status_code} - {response.text}")
        return False
    
    source = response.json()
    print(f"Status source: {json.dumps(source, indent=2)}")
    
    # Verify source contains raw text
    if source.get("text") != status_data["status"]:
        print(f"ERROR: Source text mismatch. Expected: {status_data['status']}, Got: {source.get('text')}")
        return False
    
    if source.get("spoiler_text") != status_data["spoiler_text"]:
        print(f"ERROR: Source spoiler_text mismatch")
        return False
    
    print("✓ Status source test passed")
    return True

def test_status_history(token):
    """Test status history endpoint"""
    print("\n=== Testing Status History ===")
    
    # Create a status
    status_data = {
        "status": "Original content",
        "sensitive": False
    }
    
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.post(f"{API_BASE}/statuses", json=status_data, headers=headers)
    if response.status_code != 200:
        print(f"Failed to create status: {response.status_code} - {response.text}")
        return False
    
    status = response.json()
    status_id = status["id"]
    print(f"Created status: {status_id}")
    
    # Edit the status
    time.sleep(1)  # Ensure different timestamp
    update_data = {
        "status": "Updated content",
        "spoiler_text": "Now with warning",
        "sensitive": True
    }
    
    response = requests.put(f"{API_BASE}/statuses/{status_id}", json=update_data, headers=headers)
    if response.status_code != 200:
        print(f"Failed to update status: {response.status_code} - {response.text}")
        return False
    
    print("Updated status")
    
    # Get status history
    response = requests.get(f"{API_BASE}/statuses/{status_id}/history", headers=headers)
    if response.status_code != 200:
        print(f"Failed to get status history: {response.status_code} - {response.text}")
        return False
    
    history = response.json()
    print(f"Status history: {json.dumps(history, indent=2)}")
    
    # Verify history contains both versions
    if len(history) < 2:
        print(f"ERROR: Expected at least 2 history entries, got {len(history)}")
        return False
    
    # Most recent should be the updated version
    if history[0]["content"] != update_data["status"]:
        print(f"ERROR: Most recent history entry doesn't match update")
        return False
    
    # Older entry should be original
    if history[-1]["content"] != status_data["status"]:
        print(f"ERROR: Original history entry doesn't match creation")
        return False
    
    print("✓ Status history test passed")
    return True

def test_scheduled_statuses(token):
    """Test scheduled status functionality"""
    print("\n=== Testing Scheduled Statuses ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    
    # Schedule a status for 10 minutes from now
    scheduled_time = datetime.utcnow() + timedelta(minutes=10)
    scheduled_data = {
        "status": "This is a scheduled status",
        "visibility": "public",
        "scheduled_at": scheduled_time.isoformat() + "Z"
    }
    
    response = requests.post(f"{API_BASE}/statuses", json=scheduled_data, headers=headers)
    if response.status_code != 200:
        print(f"Failed to schedule status: {response.status_code} - {response.text}")
        return False
    
    scheduled = response.json()
    print(f"Scheduled status: {json.dumps(scheduled, indent=2)}")
    
    # Verify it's a scheduled status response
    if "scheduled_at" not in scheduled:
        print("ERROR: Response doesn't look like a scheduled status")
        return False
    
    scheduled_id = scheduled["id"]
    
    # List scheduled statuses
    response = requests.get(f"{API_BASE}/scheduled_statuses", headers=headers)
    if response.status_code != 200:
        print(f"Failed to list scheduled statuses: {response.status_code} - {response.text}")
        return False
    
    scheduled_list = response.json()
    print(f"Found {len(scheduled_list)} scheduled statuses")
    
    # Find our scheduled status
    found = False
    for s in scheduled_list:
        if s["id"] == scheduled_id:
            found = True
            break
    
    if not found:
        print(f"ERROR: Scheduled status {scheduled_id} not found in list")
        return False
    
    # Get single scheduled status
    response = requests.get(f"{API_BASE}/scheduled_statuses/{scheduled_id}", headers=headers)
    if response.status_code != 200:
        print(f"Failed to get scheduled status: {response.status_code} - {response.text}")
        return False
    
    single_scheduled = response.json()
    print(f"Retrieved scheduled status: {single_scheduled['id']}")
    
    # Update scheduled time to 15 minutes from now
    new_scheduled_time = datetime.utcnow() + timedelta(minutes=15)
    update_data = {
        "scheduled_at": new_scheduled_time.isoformat() + "Z"
    }
    
    response = requests.put(f"{API_BASE}/scheduled_statuses/{scheduled_id}", 
                          json=update_data, headers=headers)
    if response.status_code != 200:
        print(f"Failed to update scheduled status: {response.status_code} - {response.text}")
        return False
    
    updated = response.json()
    print(f"Updated scheduled time to: {updated['scheduled_at']}")
    
    # Delete scheduled status
    response = requests.delete(f"{API_BASE}/scheduled_statuses/{scheduled_id}", headers=headers)
    if response.status_code != 200:
        print(f"Failed to delete scheduled status: {response.status_code} - {response.text}")
        return False
    
    print("Deleted scheduled status")
    
    # Verify it's gone
    response = requests.get(f"{API_BASE}/scheduled_statuses/{scheduled_id}", headers=headers)
    if response.status_code != 404:
        print(f"ERROR: Expected 404 for deleted scheduled status, got {response.status_code}")
        return False
    
    print("✓ Scheduled status test passed")
    return True

def test_scheduled_status_validation(token):
    """Test scheduled status validation"""
    print("\n=== Testing Scheduled Status Validation ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    
    # Try to schedule in the past
    past_time = datetime.utcnow() - timedelta(minutes=10)
    invalid_data = {
        "status": "This should fail",
        "scheduled_at": past_time.isoformat() + "Z"
    }
    
    response = requests.post(f"{API_BASE}/statuses", json=invalid_data, headers=headers)
    if response.status_code == 200:
        print("ERROR: Scheduling in the past should have failed")
        return False
    
    print("✓ Correctly rejected past scheduled time")
    
    # Try to schedule less than 5 minutes in the future
    near_future = datetime.utcnow() + timedelta(minutes=3)
    invalid_data["scheduled_at"] = near_future.isoformat() + "Z"
    
    response = requests.post(f"{API_BASE}/statuses", json=invalid_data, headers=headers)
    if response.status_code == 200:
        print("ERROR: Scheduling less than 5 minutes ahead should have failed")
        return False
    
    print("✓ Correctly rejected near-future scheduled time")
    
    # Try invalid time format
    invalid_data["scheduled_at"] = "not-a-date"
    
    response = requests.post(f"{API_BASE}/statuses", json=invalid_data, headers=headers)
    if response.status_code == 200:
        print("ERROR: Invalid date format should have failed")
        return False
    
    print("✓ Correctly rejected invalid date format")
    
    print("✓ Scheduled status validation test passed")
    return True

def main():
    """Run all Phase 2 extended feature tests"""
    print("Testing Phase 2 Extended Features for Lesser")
    print("=" * 50)
    
    # Get access token
    token = get_access_token(TEST_USER, TEST_PASSWORD)
    if not token:
        print("Failed to get access token")
        sys.exit(1)
    
    print(f"Got access token for {TEST_USER}")
    
    # Run tests
    tests = [
        ("Status Source", test_status_source),
        ("Status History", test_status_history),
        ("Scheduled Statuses", test_scheduled_statuses),
        ("Scheduled Status Validation", test_scheduled_status_validation)
    ]
    
    passed = 0
    failed = 0
    
    for test_name, test_func in tests:
        try:
            if test_func(token):
                passed += 1
            else:
                failed += 1
                print(f"✗ {test_name} test failed")
        except Exception as e:
            failed += 1
            print(f"✗ {test_name} test failed with exception: {e}")
    
    print("\n" + "=" * 50)
    print(f"Tests passed: {passed}")
    print(f"Tests failed: {failed}")
    
    if failed == 0:
        print("\n✓ All Phase 2 extended feature tests passed!")
        sys.exit(0)
    else:
        print(f"\n✗ {failed} tests failed")
        sys.exit(1)

if __name__ == "__main__":
    main() 