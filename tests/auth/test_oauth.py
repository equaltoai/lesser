#!/usr/bin/env python3
"""Test OAuth2 implementation in Lesser."""

import base64
import hashlib
import secrets
from urllib.parse import urlencode, urlparse, parse_qs, parse_qsl

import requests

# Configuration
BASE_URL = "http://localhost:8080/api/v1"  # Update with your API URL
CLIENT_ID = None  # Will be set after app registration
CLIENT_SECRET = None
REDIRECT_URI = "http://localhost:3000/callback"


def report_secret(label: str) -> None:
    """Consistently acknowledge secret handling without exposing values."""
    print(f"{label}: <redacted>")

def register_oauth_app():
    """Register an OAuth application."""
    print("\n=== Registering OAuth App ===")
    
    data = {
        "client_name": "Test OAuth App",
        "redirect_uris": f"{REDIRECT_URI} urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow push",
        "website": "https://example.com"
    }
    
    response = requests.post(f"{BASE_URL}/apps", json=data)
    print(f"Status: {response.status_code}")
    
    if response.status_code == 200:
        app = response.json()
        print("App registered successfully!")
        print("OAuth client credentials retrieved.")
        return app['client_id'], app['client_secret']
    else:
        print(f"Error: status={response.status_code}")
        return None, None

def test_authorization_flow(username, password):
    """Test the OAuth authorization code flow."""
    print("\n=== Testing Authorization Code Flow ===")
    
    # Generate PKCE challenge
    code_verifier = base64.urlsafe_b64encode(secrets.token_bytes(32)).decode('utf-8').rstrip('=')
    code_challenge = base64.urlsafe_b64encode(
        hashlib.sha256(code_verifier.encode('utf-8')).digest()
    ).decode('utf-8').rstrip('=')
    
    # Step 1: Get authorization URL
    state = secrets.token_urlsafe(16)
    auth_params = {
        'response_type': 'code',
        'client_id': CLIENT_ID,
        'redirect_uri': REDIRECT_URI,
        'scope': 'read write',
        'state': state,
        'code_challenge': code_challenge,
        'code_challenge_method': 'S256'
    }
    
    auth_url = f"{BASE_URL}/oauth/authorize?{urlencode(auth_params)}"
    print("Authorization URL generated for OAuth flow.")
    
    # Simulate user login and authorization
    # In a real app, the user would be redirected to login
    print("\nSimulating user authorization...")
    
    # First, login to get a session
    login_response = requests.post(f"{BASE_URL}/auth/login", json={
        'username': username,
        'password': password
    })
    
    if login_response.status_code != 200:
        print(f"Login failed: status={login_response.status_code}")
        return None
    
    auth_token = login_response.json()['access_token']
    
    # Now authorize the app (with authentication)
    auth_response = requests.get(auth_url, headers={
        'Authorization': f'Bearer {auth_token}'
    }, allow_redirects=False)
    
    if auth_response.status_code == 302:
        # Parse authorization code from redirect
        redirect_url = auth_response.headers.get('Location')
        print("Received redirect from authorization endpoint.")
        
        parsed = urlparse(redirect_url)
        params = parse_qs(parsed.query)
        
        if 'code' in params:
            auth_code = params['code'][0]
            returned_state = params.get('state', [None])[0]
            
            print(f"State matches: {returned_state == state}")
            
            # Step 2: Exchange code for tokens
            print("\nExchanging code for tokens...")
            token_data = {
                'grant_type': 'authorization_code',
                'code': auth_code,
                'redirect_uri': REDIRECT_URI,
                'client_id': CLIENT_ID,
                'client_secret': CLIENT_SECRET,
                'code_verifier': code_verifier
            }
            
            token_response = requests.post(f"{BASE_URL}/oauth/token", data=token_data)
            print(f"Token response status: {token_response.status_code}")
            
            if token_response.status_code == 200:
                tokens = token_response.json()
                report_secret("Access token")
                if 'refresh_token' in tokens:
                    report_secret("Refresh token")
                print(f"Token type: {tokens['token_type']}")
                print(f"Expires in: {tokens['expires_in']} seconds")
                return tokens
            else:
                print(f"Token exchange failed with status {token_response.status_code}.")
        else:
            print(f"No authorization code in redirect: {params}")
    else:
        print(f"Authorization failed with status {auth_response.status_code}.")
    
    return None

def test_refresh_token(refresh_token):
    """Test refreshing an access token."""
    print("\n=== Testing Refresh Token ===")
    
    data = {
        'grant_type': 'refresh_token',
        'refresh_token': refresh_token,
        'client_id': CLIENT_ID,
        'client_secret': CLIENT_SECRET
    }
    
    response = requests.post(f"{BASE_URL}/oauth/token", data=data)
    print(f"Status: {response.status_code}")
    
    if response.status_code == 200:
        tokens = response.json()
        report_secret("Refreshed access token")
        print(f"Token refreshed successfully!")
        return tokens['access_token']
    else:
        print(f"Error: status={response.status_code}")
        return None

def test_token_revocation(token):
    """Test revoking a token."""
    print("\n=== Testing Token Revocation ===")
    
    data = {
        'token': token,
        'token_type_hint': 'refresh_token',
        'client_id': CLIENT_ID,
        'client_secret': CLIENT_SECRET
    }
    
    response = requests.post(f"{BASE_URL}/oauth/revoke", data=data)
    print(f"Status: {response.status_code}")
    
    if response.status_code == 200:
        print("Token revoked successfully!")
    else:
        print(f"Error: status={response.status_code}")

def test_api_with_oauth_token(access_token):
    """Test API access with OAuth token."""
    print("\n=== Testing API Access with OAuth Token ===")
    
    headers = {
        'Authorization': f'Bearer {access_token}'
    }
    
    # Test getting user info
    response = requests.get(f"{BASE_URL}/accounts/verify_credentials", headers=headers)
    print(f"Status: {response.status_code}")
    
    if response.status_code == 200:
        user = response.json()
        print(f"Authenticated as: @{user['username']}")
        print(f"Display name: {user.get('display_name', 'N/A')}")
        return True
    else:
        print(f"Error: status={response.status_code}")
        return False

def test_external_oauth():
    """Test external OAuth provider flow."""
    print("\n=== Testing External OAuth (GitHub) ===")
    
    # Get GitHub authorization URL
    response = requests.get(f"{BASE_URL}/oauth/github/authorize", 
                           params={'redirect_uri': REDIRECT_URI},
                           headers={'Accept': 'application/json'})
    
    if response.status_code == 200:
        data = response.json()
        auth_url = data['auth_url']
        parsed_url = urlparse(auth_url)
        masked_query = []
        for key, value in parse_qsl(parsed_url.query):
            if key.lower() in {"client_id", "state"}:
                masked_query.append(f"{key}=<redacted>")
            else:
                masked_query.append(f"{key}={value}")
        safe_url = parsed_url._replace(query="&".join(masked_query)).geturl()
        print(f"GitHub Auth URL: {safe_url}")
        report_secret("OAuth state parameter")
        print("\nTo test GitHub OAuth:")
        print("1. Visit the auth URL in your browser")
        print("2. Authorize the app on GitHub")
        print("3. You'll be redirected back with a code")
        print("4. The callback handler will exchange it for tokens")
    else:
        print(f"Error: status={response.status_code}")

def main():
    """Run OAuth tests."""
    print("=== Lesser OAuth2 Test Suite ===")
    
    # Register OAuth app
    global CLIENT_ID, CLIENT_SECRET
    CLIENT_ID, CLIENT_SECRET = register_oauth_app()
    
    if not CLIENT_ID:
        print("Failed to register OAuth app. Exiting.")
        return
    
    # Test user credentials
    username = input("\nEnter test username: ")
    password = input("Enter test password: ")
    
    # Test authorization code flow
    tokens = test_authorization_flow(username, password)
    
    if tokens:
        # Test API access
        test_api_with_oauth_token(tokens['access_token'])
        
        # Test refresh token
        new_access_token = test_refresh_token(tokens['refresh_token'])
        
        if new_access_token:
            # Test with refreshed token
            test_api_with_oauth_token(new_access_token)
        
        # Test token revocation
        test_token_revocation(tokens['refresh_token'])
        
        # Try to use revoked token (should fail)
        print("\n=== Testing Revoked Token ===")
        test_api_with_oauth_token(tokens['access_token'])
    
    # Test external OAuth
    test_external_oauth()
    
    print("\n=== OAuth2 Tests Complete ===")

if __name__ == "__main__":
    main() 
