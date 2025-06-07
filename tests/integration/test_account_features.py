#!/usr/bin/env python3
"""Test script for account pinning and notes functionality."""

import requests
import json
import sys
from datetime import datetime

# Configuration
BASE_URL = "http://localhost:8080"
USERNAME = "testuser"
PASSWORD = "testpass123!"
TARGET_ACCOUNT = "testuser2"  # Account to pin/note

def get_auth_token():
    """Get an OAuth token for authentication."""
    # First register the user if needed
    register_data = {
        "username": USERNAME,
        "email": f"{USERNAME}@example.com",
        "password": PASSWORD,
        "agreement": True,
        "locale": "en"
    }
    
    # Try to register (may fail if user exists)
    requests.post(f"{BASE_URL}/api/v1/accounts", json=register_data)
    
    # Create OAuth app
    app_data = {
        "client_name": "Test Account Features App",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow"
    }
    
    app_response = requests.post(f"{BASE_URL}/api/v1/apps", json=app_data)
    if app_response.status_code != 200:
        print(f"Failed to create OAuth app: {app_response.text}")
        return None
        
    app = app_response.json()
    client_id = app["client_id"]
    client_secret = app["client_secret"]
    
    # Get token
    token_data = {
        "client_id": client_id,
        "client_secret": client_secret,
        "grant_type": "password",
        "username": USERNAME,
        "password": PASSWORD,
        "scope": "read write follow"
    }
    
    token_response = requests.post(f"{BASE_URL}/oauth/token", data=token_data)
    if token_response.status_code != 200:
        print(f"Failed to get token: {token_response.text}")
        return None
        
    return token_response.json()["access_token"]

def test_account_pin(token):
    """Test account pinning functionality."""
    headers = {"Authorization": f"Bearer {token}"}
    
    print("\n=== Testing Account Pinning ===")
    
    # Pin an account
    print(f"\n1. Pinning account {TARGET_ACCOUNT}...")
    pin_response = requests.post(
        f"{BASE_URL}/api/v1/accounts/{TARGET_ACCOUNT}/pin",
        headers=headers
    )
    
    if pin_response.status_code == 200:
        relationship = pin_response.json()
        print(f"✓ Account pinned successfully")
        print(f"  Endorsed: {relationship.get('endorsed', False)}")
    else:
        print(f"✗ Failed to pin account: {pin_response.status_code} - {pin_response.text}")
        return False
    
    # Try to pin again (should fail)
    print("\n2. Trying to pin the same account again...")
    pin_again_response = requests.post(
        f"{BASE_URL}/api/v1/accounts/{TARGET_ACCOUNT}/pin",
        headers=headers
    )
    
    if pin_again_response.status_code == 422:
        print("✓ Correctly rejected duplicate pin")
    else:
        print(f"✗ Unexpected response: {pin_again_response.status_code}")
    
    # Unpin the account
    print(f"\n3. Unpinning account {TARGET_ACCOUNT}...")
    unpin_response = requests.post(
        f"{BASE_URL}/api/v1/accounts/{TARGET_ACCOUNT}/unpin",
        headers=headers
    )
    
    if unpin_response.status_code == 200:
        relationship = unpin_response.json()
        print(f"✓ Account unpinned successfully")
        print(f"  Endorsed: {relationship.get('endorsed', False)}")
    else:
        print(f"✗ Failed to unpin account: {unpin_response.status_code} - {unpin_response.text}")
        return False
    
    return True

def main():
    """Run all tests."""
    print("Starting Account Features Tests")
    print("==============================")
    
    # Get auth token
    print("\nGetting authentication token...")
    token = get_auth_token()
    if not token:
        print("Failed to get auth token")
        sys.exit(1)
    
    print("✓ Got auth token")
    
    # Run test
    if test_account_pin(token):
        print("\n✓ All tests passed!")
    else:
        print("\n✗ Tests failed")
        sys.exit(1)

if __name__ == "__main__":
    main() 