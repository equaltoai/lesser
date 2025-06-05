#!/usr/bin/env python3
"""
Test script for lesser API using Mastodon.py
First install: pip install Mastodon.py
"""

from mastodon import Mastodon
import time
import sys

# Your lesser instance
INSTANCE_URL = 'https://lesser.host'

def test_instance_info():
    """Test instance information endpoints"""
    print("Testing instance info...")
    
    # Create anonymous client
    mastodon = Mastodon(api_base_url=INSTANCE_URL)
    
    try:
        # Test v1 instance endpoint
        instance = mastodon.instance()
        print(f"✓ Instance: {instance['uri']}")
        print(f"  Title: {instance['title']}")
        print(f"  Version: {instance['version']}")
    except Exception as e:
        print(f"✗ Failed to get instance info: {e}")
        
    try:
        # Test v2 instance endpoint  
        instance_v2 = mastodon.instance_v2()
        print(f"✓ Instance v2: {instance_v2['domain']}")
    except Exception as e:
        print(f"✗ Failed to get instance v2 info: {e}")

def test_app_registration():
    """Test OAuth app registration"""
    print("\nTesting app registration...")
    
    try:
        # Register app
        client_id, client_secret = Mastodon.create_app(
            'lesser-test-app',
            api_base_url=INSTANCE_URL,
            scopes=['read', 'write', 'follow', 'push']
        )
        print(f"✓ App registered successfully")
        print(f"  Client ID: {client_id}")
        return client_id, client_secret
    except Exception as e:
        print(f"✗ Failed to register app: {e}")
        return None, None

def test_oauth_flow(client_id, client_secret):
    """Test OAuth authentication flow"""
    print("\nTesting OAuth flow...")
    
    try:
        # Create client
        mastodon = Mastodon(
            client_id=client_id,
            client_secret=client_secret,
            api_base_url=INSTANCE_URL
        )
        
        # Get auth URL
        auth_url = mastodon.auth_request_url(
            scopes=['read', 'write', 'follow', 'push']
        )
        print(f"✓ Auth URL generated: {auth_url}")
        print("\nPlease visit the URL above and authorize the app.")
        print("Then paste the authorization code here:")
        
        # Get code from user
        auth_code = input("Authorization code: ").strip()
        
        # Log in with code
        access_token = mastodon.log_in(
            code=auth_code,
            scopes=['read', 'write', 'follow', 'push']
        )
        print(f"✓ Successfully logged in")
        
        return mastodon
        
    except Exception as e:
        print(f"✗ OAuth flow failed: {e}")
        return None

def test_authenticated_endpoints(mastodon):
    """Test endpoints that require authentication"""
    print("\nTesting authenticated endpoints...")
    
    # Test verify credentials
    try:
        account = mastodon.account_verify_credentials()
        print(f"✓ Verify credentials: @{account['username']}")
        print(f"  ID: {account['id']}")
        print(f"  Display name: {account['display_name']}")
    except Exception as e:
        print(f"✗ Failed to verify credentials: {e}")
        
    # Test posting a status
    try:
        status = mastodon.status_post(
            f"Test post from Mastodon.py at {time.strftime('%Y-%m-%d %H:%M:%S')}",
            visibility='public'
        )
        print(f"✓ Posted status: {status['id']}")
        print(f"  Content: {status['content']}")
        
        # Test deleting the status
        time.sleep(2)
        mastodon.status_delete(status['id'])
        print(f"✓ Deleted status")
        
    except Exception as e:
        print(f"✗ Failed to post/delete status: {e}")
    
    # Test timelines
    try:
        # Home timeline
        home_tl = mastodon.timeline_home(limit=5)
        print(f"✓ Home timeline: {len(home_tl)} statuses")
        
        # Public timeline
        public_tl = mastodon.timeline_public(limit=5)
        print(f"✓ Public timeline: {len(public_tl)} statuses")
        
    except Exception as e:
        print(f"✗ Failed to get timelines: {e}")
        
    # Test notifications
    try:
        notifications = mastodon.notifications(limit=5)
        print(f"✓ Notifications: {len(notifications)} items")
    except Exception as e:
        print(f"✗ Failed to get notifications: {e}")
        
    # Test lists
    try:
        lists = mastodon.lists()
        print(f"✓ Lists: {len(lists)} lists")
    except Exception as e:
        print(f"✗ Failed to get lists: {e}")
        
    # Test preferences
    try:
        prefs = mastodon.preferences()
        print(f"✓ Preferences retrieved")
    except Exception as e:
        print(f"✗ Failed to get preferences: {e}")

def test_search(mastodon):
    """Test search functionality"""
    print("\nTesting search...")
    
    try:
        results = mastodon.search_v2('test', limit=5)
        print(f"✓ Search results:")
        print(f"  Accounts: {len(results.get('accounts', []))}")
        print(f"  Statuses: {len(results.get('statuses', []))}")
        print(f"  Hashtags: {len(results.get('hashtags', []))}")
    except Exception as e:
        print(f"✗ Search failed: {e}")

def test_trends(mastodon):
    """Test trends endpoints"""
    print("\nTesting trends...")
    
    try:
        # Trending tags
        tags = mastodon.trending_tags()
        print(f"✓ Trending tags: {len(tags)} tags")
        
        # Trending statuses
        statuses = mastodon.trending_statuses()
        print(f"✓ Trending statuses: {len(statuses)} statuses")
        
        # Trending links
        links = mastodon.trending_links()
        print(f"✓ Trending links: {len(links)} links")
        
    except Exception as e:
        print(f"✗ Trends failed: {e}")

def main():
    """Run all tests"""
    print(f"Testing lesser API at {INSTANCE_URL}\n")
    
    # Test public endpoints
    test_instance_info()
    
    # Test OAuth flow
    client_id, client_secret = test_app_registration()
    if not client_id:
        print("\nApp registration failed, cannot continue with authenticated tests")
        return
        
    # Get authenticated client
    mastodon = test_oauth_flow(client_id, client_secret)
    if not mastodon:
        print("\nOAuth flow failed, cannot continue with authenticated tests")
        return
        
    # Test authenticated endpoints
    test_authenticated_endpoints(mastodon)
    test_search(mastodon)
    test_trends(mastodon)
    
    print("\n✅ Testing complete!")

if __name__ == "__main__":
    main() 