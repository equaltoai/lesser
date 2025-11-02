#!/usr/bin/env python3
"""
Add test data to Lesser for testing pagination
"""

import requests
import time

INSTANCE_URL = 'https://lesser.host'
USERNAME = 'testuser'
PASSWORD = 'test123'

def get_oauth_token():
    """Get OAuth token for testuser"""
    # First create an app
    app_response = requests.post(f"{INSTANCE_URL}/api/v1/apps", json={
        "client_name": "Test Data Script",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow"
    })
    
    if app_response.status_code != 200:
        print(f"Failed to create app: {app_response.status_code}")
        return None, None
    
    app = app_response.json()
    client_id = app['client_id']
    client_secret = app['client_secret']
    
    # Get authorization code (simplified - in real scenario would need browser)
    # For testing, we'll use the password grant if available
    token_response = requests.post(f"{INSTANCE_URL}/oauth/token", data={
        "grant_type": "password",
        "client_id": client_id,
        "client_secret": client_secret,
        "username": USERNAME,
        "password": PASSWORD,
        "scope": "read write follow"
    })
    
    if token_response.status_code == 200:
        token_data = token_response.json()
        return token_data['access_token']
    else:
        print(f"OAuth failed: {token_response.status_code} - {token_response.text}")
        print("Password grant not supported. Manual authorization needed.")
        return None

def create_posts(token, count=10):
    """Create test posts"""
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    for i in range(count):
        post_data = {
            "status": f"Test post #{i+1} for pagination testing #test #pagination",
            "visibility": "public"
        }
        
        response = requests.post(
            f"{INSTANCE_URL}/api/v1/statuses",
            headers=headers,
            json=post_data
        )
        
        if response.status_code == 200:
            print(f"✓ Created post #{i+1}")
        else:
            print(f"✗ Failed to create post #{i+1}: {response.status_code}")
        
        # Small delay to avoid rate limiting
        time.sleep(0.5)

def main():
    print("Adding test data to Lesser...")
    
    # Get OAuth token
    token = get_oauth_token()
    if not token:
        print("Failed to get OAuth token. You may need to:")
        print("1. Ensure testuser exists with password 'test123'")
        print("2. Or manually get an access token")
        return
    
    # Create test posts
    create_posts(token, 10)
    
    print("\nTest data added successfully!")
    print("Now run test_api_automated.py to see pagination headers.")

if __name__ == "__main__":
    main() 
