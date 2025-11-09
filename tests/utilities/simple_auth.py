#!/usr/bin/env python3
"""
Simple auth token generator for Lesser that uses the direct login endpoint
"""

import requests
import sys

def get_auth_token(username, password, base_url="http://localhost:8080"):
    """Get auth token using Lesser's direct login endpoint"""
    
    # Login directly
    login_response = requests.post(f"{base_url}/auth/login", json={
        'username': username,
        'password': password
    })
    
    if login_response.status_code == 200:
        data = login_response.json()
        print(f"✅ Login successful!")
        print(f"Access Token: {data['access_token']}")
        if 'refresh_token' in data:
            print(f"Refresh Token: {data['refresh_token']}")
        return data['access_token']
    else:
        print(f"❌ Login failed: {login_response.status_code} - {login_response.text}")
        return None

def create_user(username, email, password, base_url="http://localhost:8080"):
    """Create a new user account"""
    
    response = requests.post(f"{base_url}/api/v1/accounts", json={
        "username": username,
        "email": email,
        "password": password,
        "agreement": True,
        "locale": "en"
    })
    
    if response.status_code == 201:
        print(f"✅ User '{username}' created successfully")
        return True
    elif response.status_code == 422:
        error_data = response.json()
        if "already taken" in response.text:
            print(f"ℹ️  User '{username}' already exists")
            return True
        else:
            print(f"❌ Failed to create user: {error_data}")
            return False
    else:
        print(f"❌ Failed to create user: {response.status_code} - {response.text}")
        return False

def main():
    if len(sys.argv) < 3:
        print("Usage: simple_auth.py <username> <password> [base_url]")
        print("\nInteractive mode:")
        
        username = input("Username: ")
        password = input("Password: ")
        base_url = input("Base URL (default: http://localhost:8080): ").strip() or "http://localhost:8080"
        
        # Try to login first
        token = get_auth_token(username, password, base_url)
        
        if not token:
            create = input("\nUser doesn't exist. Create new user? (y/N): ").lower() == 'y'
            if create:
                email = input("Email: ")
                if create_user(username, email, password, base_url):
                    # Try login again
                    token = get_auth_token(username, password, base_url)
        
        if token:
            print(f"\n🎉 Success! Use this token for authenticated requests:")
            print(f"export LESSER_AUTH_TOKEN='{token}'")
    else:
        username = sys.argv[1]
        password = sys.argv[2]
        base_url = sys.argv[3] if len(sys.argv) > 3 else "http://localhost:8080"
        
        token = get_auth_token(username, password, base_url)
        if token:
            print(f"\nexport LESSER_AUTH_TOKEN='{token}'")

if __name__ == "__main__":
    main() 
