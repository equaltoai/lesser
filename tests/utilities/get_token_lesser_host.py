#!/usr/bin/env python3
"""
Get auth token from lesser.host for testing
Since lesser.host doesn't support password grant, this script helps you get a token manually
"""

import requests
import webbrowser
import urllib.parse


def redact(value: str, visible: int = 4) -> str:
    """Mask sensitive values before displaying them."""
    if not value:
        return ""
    value = str(value)
    if len(value) <= visible * 2:
        return "*" * len(value)
    return f"{value[:visible]}...{value[-visible:]}"

def create_app():
    """Create an OAuth app on lesser.host"""
    
    print("Creating OAuth app on lesser.host...")
    
    response = requests.post("https://lesser.host/api/v1/apps", json={
        "client_name": "Lesser Test Suite",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write follow push admin",
        "website": "https://github.com/lesser"
    })
    
    if response.status_code == 200:
        app = response.json()
        print("✅ OAuth app created!")
        print(f"Client ID: {redact(app['client_id'])}")
        print(f"Client Secret: {redact(app['client_secret'])}")
        return app['client_id'], app['client_secret']
    else:
        print(f"❌ Failed to create app: {response.status_code} - {response.text}")
        return None, None

def main():
    # Step 1: Create OAuth app
    client_id, client_secret = create_app()
    if not client_id:
        return
    
    # Step 2: Generate authorization URL
    auth_params = {
        'response_type': 'code',
        'client_id': client_id,
        'redirect_uri': 'urn:ietf:wg:oauth:2.0:oob',
        'scope': 'read write follow push admin'
    }
    
    auth_url = f"https://lesser.host/oauth/authorize?{urllib.parse.urlencode(auth_params)}"
    
    print("\n" + "="*60)
    print("MANUAL STEPS REQUIRED:")
    print("="*60)
    print("\n1. Open this URL in your browser:")
    print(f"\n   {redact(auth_url, visible=12)}\n")
    print("2. Log in with your Lesser account")
    print("3. Authorize the app")
    print("4. Copy the authorization code shown\n")
    
    # Try to open browser
    try:
        webbrowser.open(auth_url)
        print("(Browser should open automatically)")
    except:
        pass
    
    # Step 3: Get authorization code from user
    auth_code = input("\nPaste the authorization code here: ").strip()
    
    # URL decode the code if needed
    auth_code = urllib.parse.unquote(auth_code)
    print(f"Using code: {redact(auth_code)}")
    
    # Step 4: Exchange code for token
    print("\nExchanging code for token...")
    
    token_response = requests.post("https://lesser.host/oauth/token", data={
        'grant_type': 'authorization_code',
        'client_id': client_id,
        'client_secret': client_secret,
        'code': auth_code,
        'redirect_uri': 'urn:ietf:wg:oauth:2.0:oob'
    }, headers={'Content-Type': 'application/x-www-form-urlencoded'})
    
    if token_response.status_code == 200:
        tokens = token_response.json()
        print("\n✅ Success! Tokens retrieved (values redacted for safety):\n")
        print(f"Access Token: {redact(tokens['access_token'])}")
        refresh_token = tokens.get('refresh_token')
        print(f"\nRefresh Token: {redact(refresh_token) if refresh_token else 'N/A'}")

        print("\n" + "="*60)
        print("To use these tokens in tests:")
        print("="*60)
        print("\nexport LESSER_AUTH_TOKEN='<access token>'")
        print("export LESSER_URL='https://lesser.host'")
        print("export LESSER_CLIENT_ID='<client id>'")
        print("export LESSER_CLIENT_SECRET='<client secret>'")
        
        # Verify the token works
        print("\nVerifying token...")
        verify_response = requests.get(
            "https://lesser.host/api/v1/accounts/verify_credentials",
            headers={'Authorization': f"Bearer {tokens['access_token']}"}
        )
        
        if verify_response.status_code == 200:
            user = verify_response.json()
            print(f"✅ Token verified! Authenticated as @{user['username']}")
        else:
            print(f"⚠️  Token verification failed: {verify_response.status_code}")
            
    else:
        print(f"\n❌ Failed to exchange code: {token_response.status_code}")
        print(f"Response: {token_response.text}")

if __name__ == "__main__":
    main() 
