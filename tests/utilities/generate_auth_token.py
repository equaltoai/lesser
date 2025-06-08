#!/usr/bin/env python3
"""
Utility to generate OAuth tokens for testing Lesser's authenticated endpoints.

This script supports multiple authentication methods:
1. Password Grant - Exchange username/password for token (simplest for testing)
2. Authorization Code Flow - Full OAuth flow with PKCE
3. Client Credentials - App-only authentication
"""

import requests
import json
import base64
import secrets
import hashlib
import urllib.parse
import sys
import os
from typing import Optional, Dict, Tuple

# Configuration - can be overridden with environment variables
BASE_URL = os.getenv('LESSER_URL', 'http://localhost:8080')
DEFAULT_SCOPES = 'read write follow push'


class LesserAuthTokenGenerator:
    def __init__(self, base_url: str = BASE_URL):
        self.base_url = base_url.rstrip('/')
        self.client_id = None
        self.client_secret = None
        
    def create_oauth_app(self, app_name: str = "Test Auth App") -> Tuple[str, str]:
        """Create an OAuth application and return client credentials."""
        app_data = {
            "client_name": app_name,
            "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",  # Out-of-band for testing
            "scopes": DEFAULT_SCOPES,
            "website": "https://github.com/lesser/test"
        }
        
        response = requests.post(f"{self.base_url}/api/v1/apps", json=app_data)
        if response.status_code != 200:
            raise Exception(f"Failed to create OAuth app: {response.status_code} - {response.text}")
        
        app = response.json()
        self.client_id = app['client_id']
        self.client_secret = app['client_secret']
        
        print(f"✅ Created OAuth app: {app_name}")
        print(f"   Client ID: {self.client_id}")
        print(f"   Client Secret: {self.client_secret[:20]}...")
        
        return self.client_id, self.client_secret
    
    def register_user(self, username: str, email: str, password: str) -> bool:
        """Register a new user account."""
        user_data = {
            "username": username,
            "email": email,
            "password": password,
            "agreement": True,
            "locale": "en"
        }
        
        response = requests.post(f"{self.base_url}/api/v1/accounts", json=user_data)
        if response.status_code == 201:
            print(f"✅ User '{username}' registered successfully")
            return True
        elif response.status_code == 422 and "already taken" in response.text:
            print(f"ℹ️  User '{username}' already exists")
            return True
        else:
            print(f"❌ Failed to register user: {response.status_code} - {response.text}")
            return False
    
    def password_grant_token(self, username: str, password: str, 
                           client_id: Optional[str] = None, 
                           client_secret: Optional[str] = None,
                           scopes: str = DEFAULT_SCOPES) -> Optional[Dict]:
        """Get token using password grant type (simplest for testing)."""
        if not client_id:
            client_id = self.client_id
        if not client_secret:
            client_secret = self.client_secret
            
        if not client_id or not client_secret:
            print("❌ Client credentials not set. Create an OAuth app first.")
            return None
        
        token_data = {
            "grant_type": "password",
            "client_id": client_id,
            "client_secret": client_secret,
            "username": username,
            "password": password,
            "scope": scopes
        }
        
        response = requests.post(
            f"{self.base_url}/oauth/token",
            data=token_data,
            headers={"Content-Type": "application/x-www-form-urlencoded"}
        )
        
        if response.status_code == 200:
            tokens = response.json()
            print(f"✅ Access token obtained for user '{username}'")
            print(f"   Token: {tokens['access_token'][:50]}...")
            print(f"   Type: {tokens['token_type']}")
            print(f"   Expires in: {tokens.get('expires_in', 'N/A')} seconds")
            if 'refresh_token' in tokens:
                print(f"   Refresh token: {tokens['refresh_token'][:30]}...")
            return tokens
        else:
            print(f"❌ Failed to get token: {response.status_code} - {response.text}")
            return None
    
    def authorization_code_flow(self, username: str, password: str) -> Optional[Dict]:
        """
        Simulate the full authorization code flow with PKCE.
        This is what real Mastodon clients use.
        """
        if not self.client_id or not self.client_secret:
            print("❌ Client credentials not set. Create an OAuth app first.")
            return None
        
        # Generate PKCE challenge
        code_verifier = base64.urlsafe_b64encode(secrets.token_bytes(32)).decode('utf-8').rstrip('=')
        code_challenge = base64.urlsafe_b64encode(
            hashlib.sha256(code_verifier.encode('utf-8')).digest()
        ).decode('utf-8').rstrip('=')
        
        # Step 1: Build authorization URL
        state = secrets.token_urlsafe(16)
        auth_params = {
            'response_type': 'code',
            'client_id': self.client_id,
            'redirect_uri': 'urn:ietf:wg:oauth:2.0:oob',
            'scope': DEFAULT_SCOPES,
            'state': state,
            'code_challenge': code_challenge,
            'code_challenge_method': 'S256'
        }
        
        auth_url = f"{self.base_url}/oauth/authorize?{urllib.parse.urlencode(auth_params)}"
        print(f"🔐 Authorization URL: {auth_url}")
        
        # Step 2: Login to get a session token
        print("📝 Logging in to authorize the app...")
        login_response = requests.post(f"{self.base_url}/auth/login", json={
            'username': username,
            'password': password
        })
        
        if login_response.status_code != 200:
            print(f"❌ Login failed: {login_response.text}")
            return None
        
        auth_token = login_response.json()['access_token']
        
        # Step 3: Authorize the app
        auth_response = requests.get(auth_url, headers={
            'Authorization': f'Bearer {auth_token}'
        }, allow_redirects=False)
        
        if auth_response.status_code != 302:
            print(f"❌ Authorization failed: {auth_response.status_code}")
            return None
        
        # Extract authorization code from redirect
        redirect_url = auth_response.headers.get('Location', '')
        parsed_url = urllib.parse.urlparse(redirect_url)
        query_params = urllib.parse.parse_qs(parsed_url.query)
        
        if 'code' not in query_params:
            print(f"❌ No authorization code in redirect: {redirect_url}")
            return None
        
        auth_code = query_params['code'][0]
        print(f"✅ Got authorization code: {auth_code[:20]}...")
        
        # Step 4: Exchange code for token
        token_data = {
            'grant_type': 'authorization_code',
            'client_id': self.client_id,
            'client_secret': self.client_secret,
            'code': auth_code,
            'redirect_uri': 'urn:ietf:wg:oauth:2.0:oob',
            'code_verifier': code_verifier
        }
        
        token_response = requests.post(
            f"{self.base_url}/oauth/token",
            data=token_data,
            headers={"Content-Type": "application/x-www-form-urlencoded"}
        )
        
        if token_response.status_code == 200:
            tokens = token_response.json()
            print(f"✅ Tokens obtained via authorization code flow")
            print(f"   Access token: {tokens['access_token'][:50]}...")
            print(f"   Refresh token: {tokens['refresh_token'][:30]}...")
            return tokens
        else:
            print(f"❌ Token exchange failed: {token_response.text}")
            return None
    
    def client_credentials_token(self, client_id: Optional[str] = None,
                               client_secret: Optional[str] = None,
                               scopes: str = 'read') -> Optional[Dict]:
        """Get app-only token using client credentials grant."""
        if not client_id:
            client_id = self.client_id
        if not client_secret:
            client_secret = self.client_secret
            
        if not client_id or not client_secret:
            print("❌ Client credentials not set. Create an OAuth app first.")
            return None
        
        token_data = {
            "grant_type": "client_credentials",
            "client_id": client_id,
            "client_secret": client_secret,
            "scope": scopes
        }
        
        response = requests.post(
            f"{self.base_url}/oauth/token",
            data=token_data,
            headers={"Content-Type": "application/x-www-form-urlencoded"}
        )
        
        if response.status_code == 200:
            tokens = response.json()
            print(f"✅ App-only access token obtained")
            print(f"   Token: {tokens['access_token'][:50]}...")
            print(f"   Scope: {tokens.get('scope', 'N/A')}")
            return tokens
        else:
            print(f"❌ Failed to get app token: {response.status_code} - {response.text}")
            return None
    
    def refresh_access_token(self, refresh_token: str,
                           client_id: Optional[str] = None,
                           client_secret: Optional[str] = None) -> Optional[Dict]:
        """Use a refresh token to get a new access token."""
        if not client_id:
            client_id = self.client_id
        if not client_secret:
            client_secret = self.client_secret
            
        token_data = {
            "grant_type": "refresh_token",
            "refresh_token": refresh_token,
            "client_id": client_id,
            "client_secret": client_secret
        }
        
        response = requests.post(
            f"{self.base_url}/oauth/token",
            data=token_data,
            headers={"Content-Type": "application/x-www-form-urlencoded"}
        )
        
        if response.status_code == 200:
            tokens = response.json()
            print(f"✅ Access token refreshed")
            print(f"   New token: {tokens['access_token'][:50]}...")
            return tokens
        else:
            print(f"❌ Failed to refresh token: {response.status_code} - {response.text}")
            return None
    
    def verify_token(self, access_token: str) -> bool:
        """Verify that a token works by calling the verify_credentials endpoint."""
        headers = {"Authorization": f"Bearer {access_token}"}
        response = requests.get(f"{self.base_url}/api/v1/accounts/verify_credentials", headers=headers)
        
        if response.status_code == 200:
            user = response.json()
            print(f"✅ Token verified - authenticated as @{user['username']}")
            return True
        else:
            print(f"❌ Token verification failed: {response.status_code}")
            return False


def interactive_mode():
    """Interactive mode to generate tokens."""
    generator = LesserAuthTokenGenerator()
    
    print("🚀 Lesser OAuth Token Generator")
    print("=" * 50)
    
    # Step 1: Create OAuth app
    app_name = input("Enter OAuth app name (default: Test Auth App): ").strip() or "Test Auth App"
    generator.create_oauth_app(app_name)
    print()
    
    # Step 2: Choose authentication method
    print("Select authentication method:")
    print("1. Password Grant (simplest for testing)")
    print("2. Authorization Code Flow (what real clients use)")
    print("3. Client Credentials (app-only, no user)")
    print("4. Use existing user token")
    
    choice = input("\nChoice (1-4): ").strip()
    
    if choice in ['1', '2']:
        # Need user credentials
        print("\nUser credentials:")
        username = input("Username: ").strip()
        
        create_new = input("Create new user? (y/N): ").strip().lower() == 'y'
        if create_new:
            email = input("Email: ").strip()
            password = input("Password: ").strip()
            generator.register_user(username, email, password)
        else:
            password = input("Password: ").strip()
        
        print()
        
        if choice == '1':
            tokens = generator.password_grant_token(username, password)
        else:
            tokens = generator.authorization_code_flow(username, password)
            
        if tokens:
            print("\n" + "=" * 50)
            print("🎉 SUCCESS! Your tokens:")
            print(f"Access Token: {tokens['access_token']}")
            if 'refresh_token' in tokens:
                print(f"Refresh Token: {tokens['refresh_token']}")
            print("=" * 50)
            
            # Save to file option
            save = input("\nSave tokens to file? (y/N): ").strip().lower() == 'y'
            if save:
                filename = input("Filename (default: .env.lesser): ").strip() or ".env.lesser"
                with open(filename, 'w') as f:
                    f.write(f"# Generated by generate_auth_token.py\n")
                    f.write(f"LESSER_URL={generator.base_url}\n")
                    f.write(f"LESSER_CLIENT_ID={generator.client_id}\n")
                    f.write(f"LESSER_CLIENT_SECRET={generator.client_secret}\n")
                    f.write(f"LESSER_ACCESS_TOKEN={tokens['access_token']}\n")
                    if 'refresh_token' in tokens:
                        f.write(f"LESSER_REFRESH_TOKEN={tokens['refresh_token']}\n")
                print(f"✅ Tokens saved to {filename}")
    
    elif choice == '3':
        tokens = generator.client_credentials_token()
        if tokens:
            print(f"\n🎉 App-only token: {tokens['access_token']}")
    
    elif choice == '4':
        token = input("\nEnter existing access token: ").strip()
        if generator.verify_token(token):
            print("✅ Token is valid!")
        else:
            print("❌ Token is invalid or expired")


def quick_token(username: str, password: str, base_url: str = BASE_URL) -> Optional[str]:
    """Quick function to get a token for testing - returns just the access token string."""
    generator = LesserAuthTokenGenerator(base_url)
    generator.create_oauth_app("Quick Test App")
    tokens = generator.password_grant_token(username, password)
    return tokens['access_token'] if tokens else None


if __name__ == "__main__":
    if len(sys.argv) > 1:
        # Command line mode
        if len(sys.argv) < 3:
            print("Usage: generate_auth_token.py <username> <password> [base_url]")
            sys.exit(1)
        
        username = sys.argv[1]
        password = sys.argv[2]
        base_url = sys.argv[3] if len(sys.argv) > 3 else BASE_URL
        
        token = quick_token(username, password, base_url)
        if token:
            print(f"\nAccess Token: {token}")
        else:
            sys.exit(1)
    else:
        # Interactive mode
        interactive_mode() 