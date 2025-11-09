#!/usr/bin/env python3
"""
Test WebAuthn/Passkey functionality
"""

import requests
import base64
import os

# Configuration
BASE_URL = os.getenv('LESSER_API_URL', 'https://api.lesser.app/lab')
AUTH_URL = os.getenv('LESSER_AUTH_URL', 'https://api.lesser.app/lab')

# Test user credentials
TEST_USER = "testuser"
TEST_PASSWORD = "testpassword123"

def login_with_password():
    """Login with password to get access token"""
    print("Logging in with password...")
    response = requests.post(f"{AUTH_URL}/auth/login", json={
        "username": TEST_USER,
        "password": TEST_PASSWORD,
        "device_name": "Test Script"
    })
    
    if response.status_code != 200:
        print(f"Login failed: {response.status_code}")
        print(response.text)
        return None
    
    data = response.json()
    print(f"Login successful! Access token: {data['access_token'][:20]}...")
    return data['access_token']

def test_webauthn_registration_begin(access_token):
    """Start WebAuthn registration"""
    print("\nStarting WebAuthn registration...")
    
    headers = {"Authorization": f"Bearer {access_token}"}
    response = requests.post(f"{AUTH_URL}/auth/webauthn/register/begin", headers=headers)
    
    if response.status_code != 200:
        print(f"Registration begin failed: {response.status_code}")
        print(response.text)
        return None
    
    data = response.json()
    print("Registration challenge received:")
    print(f"- Challenge: {data['challenge']}")
    print(f"- RP ID: {data['publicKey']['rp']['id']}")
    print(f"- User ID: {base64.b64encode(data['publicKey']['user']['id'].encode()).decode()}")
    
    return data

def test_webauthn_login_begin():
    """Start WebAuthn login (no auth required)"""
    print("\nStarting WebAuthn login...")
    
    response = requests.post(f"{AUTH_URL}/auth/webauthn/login/begin", json={
        "username": TEST_USER
    })
    
    if response.status_code != 200:
        print(f"Login begin failed: {response.status_code}")
        print(response.text)
        return None
    
    data = response.json()
    print("Login challenge received:")
    print(f"- Challenge: {data['challenge']}")
    print(f"- Allowed credentials: {len(data['publicKey'].get('allowCredentials', []))}")
    
    return data

def list_webauthn_credentials(access_token):
    """List user's WebAuthn credentials"""
    print("\nListing WebAuthn credentials...")
    
    headers = {"Authorization": f"Bearer {access_token}"}
    response = requests.get(f"{AUTH_URL}/auth/webauthn/credentials", headers=headers)
    
    if response.status_code != 200:
        print(f"List credentials failed: {response.status_code}")
        print(response.text)
        return None
    
    data = response.json()
    credentials = data.get('credentials', [])
    
    if not credentials:
        print("No WebAuthn credentials registered")
    else:
        print(f"Found {len(credentials)} credential(s):")
        for cred in credentials:
            print(f"  - {cred['name']} (ID: {cred['id'][:20]}...)")
            print(f"    Created: {cred['created_at']}")
            print(f"    Last used: {cred['last_used_at']}")
    
    return credentials

def main():
    print("=== WebAuthn Test Suite ===")
    print(f"Auth URL: {AUTH_URL}")
    print(f"Test user: {TEST_USER}")
    print()
    
    # Step 1: Login with password
    access_token = login_with_password()
    if not access_token:
        print("Failed to login, exiting")
        return
    
    # Step 2: List existing credentials
    credentials = list_webauthn_credentials(access_token)
    
    # Step 3: Start registration
    reg_data = test_webauthn_registration_begin(access_token)
    if reg_data:
        print("\nRegistration challenge created successfully!")
        print("Note: To complete registration, you need a WebAuthn-capable browser/client")
        print("The challenge expires in 5 minutes")
    
    # Step 4: Test login begin (doesn't require auth)
    if credentials:  # Only test login if user has credentials
        login_data = test_webauthn_login_begin()
        if login_data:
            print("\nLogin challenge created successfully!")
            print("Note: To complete login, you need a WebAuthn-capable browser/client")
    
    print("\n=== Test Complete ===")
    print("\nNext steps:")
    print("1. Use a WebAuthn-capable client to complete registration/login")
    print("2. The client should call /auth/webauthn/register/finish or /auth/webauthn/login/finish")
    print("3. Include the challenge and credential response from the browser's WebAuthn API")

if __name__ == "__main__":
    main() 
