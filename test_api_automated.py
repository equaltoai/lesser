#!/usr/bin/env python3
"""
Automated test script for lesser API using Mastodon.py
Tests endpoints that don't require manual OAuth flow
"""

from mastodon import Mastodon
import json
import requests
import base64
import time

# Your lesser instance
INSTANCE_URL = 'https://lesser.host'

# Test credentials (from your existing test user)
TEST_USERNAME = 'testuser'
TEST_PASSWORD = 'testpassword'

def test_public_endpoints():
    """Test endpoints that don't require authentication"""
    print("=== Testing Public Endpoints ===\n")
    
    # Create anonymous client
    mastodon = Mastodon(api_base_url=INSTANCE_URL)
    
    # Test instance info
    try:
        instance = mastodon.instance()
        print(f"✓ GET /api/v1/instance")
        print(f"  URI: {instance.get('uri', 'N/A')}")
        print(f"  Title: {instance.get('title', 'N/A')}")
        print(f"  Version: {instance.get('version', 'N/A')}")
    except Exception as e:
        print(f"✗ GET /api/v1/instance failed: {e}")
    
    # Test public timeline
    try:
        public_tl = mastodon.timeline_public(limit=5)
        print(f"\n✓ GET /api/v1/timelines/public")
        print(f"  Statuses: {len(public_tl)}")
    except Exception as e:
        print(f"\n✗ GET /api/v1/timelines/public failed: {e}")
    
    # Test local timeline
    try:
        local_tl = mastodon.timeline_local(limit=5)
        print(f"\n✓ GET /api/v1/timelines/public?local=true")
        print(f"  Statuses: {len(local_tl)}")
    except Exception as e:
        print(f"\n✗ GET /api/v1/timelines/public?local=true failed: {e}")
    
    # Test custom emojis
    try:
        emojis = mastodon.custom_emojis()
        print(f"\n✓ GET /api/v1/custom_emojis")
        print(f"  Emojis: {len(emojis)}")
    except Exception as e:
        print(f"\n✗ GET /api/v1/custom_emojis failed: {e}")
    
    # Test instance activity
    try:
        activity = mastodon.instance_activity()
        print(f"\n✓ GET /api/v1/instance/activity")
        print(f"  Activity entries: {len(activity)}")
    except Exception as e:
        print(f"\n✗ GET /api/v1/instance/activity failed: {e}")
    
    # Test instance peers
    try:
        peers = mastodon.instance_peers()
        print(f"\n✓ GET /api/v1/instance/peers")
        print(f"  Peers: {len(peers)}")
    except Exception as e:
        print(f"\n✗ GET /api/v1/instance/peers failed: {e}")

def test_webfinger():
    """Test WebFinger endpoint"""
    print("\n\n=== Testing WebFinger ===\n")
    
    webfinger_url = f"{INSTANCE_URL}/.well-known/webfinger"
    resource = f"acct:{TEST_USERNAME}@{INSTANCE_URL.replace('https://', '')}"
    
    try:
        response = requests.get(webfinger_url, params={'resource': resource})
        if response.status_code == 200:
            data = response.json()
            print(f"✓ WebFinger lookup for {resource}")
            print(f"  Subject: {data.get('subject', 'N/A')}")
            print(f"  Links: {len(data.get('links', []))}")
        else:
            print(f"✗ WebFinger failed with status {response.status_code}")
    except Exception as e:
        print(f"✗ WebFinger failed: {e}")

def test_nodeinfo():
    """Test NodeInfo endpoints"""
    print("\n\n=== Testing NodeInfo ===\n")
    
    # Test .well-known/nodeinfo
    try:
        response = requests.get(f"{INSTANCE_URL}/.well-known/nodeinfo")
        if response.status_code == 200:
            data = response.json()
            print(f"✓ GET /.well-known/nodeinfo")
            print(f"  Links: {len(data.get('links', []))}")
            
            # Test actual nodeinfo endpoints
            for link in data.get('links', []):
                href = link.get('href')
                if href:
                    try:
                        ni_response = requests.get(href)
                        if ni_response.status_code == 200:
                            ni_data = ni_response.json()
                            print(f"\n✓ GET {href}")
                            print(f"  Software: {ni_data.get('software', {}).get('name', 'N/A')}")
                            print(f"  Version: {ni_data.get('software', {}).get('version', 'N/A')}")
                    except:
                        pass
        else:
            print(f"✗ GET /.well-known/nodeinfo failed with status {response.status_code}")
    except Exception as e:
        print(f"✗ NodeInfo failed: {e}")

def test_oauth_endpoints():
    """Test OAuth endpoints directly"""
    print("\n\n=== Testing OAuth Endpoints ===\n")
    
    # Test app creation
    try:
        app_data = {
            'client_name': 'lesser-test-automated',
            'redirect_uris': 'urn:ietf:wg:oauth:2.0:oob',
            'scopes': 'read write follow push',
            'website': 'https://github.com/aron23/lesser'
        }
        
        response = requests.post(f"{INSTANCE_URL}/api/v1/apps", json=app_data)
        if response.status_code == 200:
            app = response.json()
            print(f"✓ POST /api/v1/apps")
            print(f"  Client ID: {app.get('client_id', 'N/A')[:20]}...")
            print(f"  Redirect URI: {app.get('redirect_uri', 'N/A')}")
            return app
        else:
            print(f"✗ POST /api/v1/apps failed with status {response.status_code}")
            return None
    except Exception as e:
        print(f"✗ App creation failed: {e}")
        return None

def test_api_response_headers():
    """Test API response headers"""
    print("\n\n=== Testing Response Headers ===\n")
    
    endpoints = [
        '/api/v1/instance',
        '/api/v1/timelines/public',
        '/api/v1/custom_emojis',
    ]
    
    for endpoint in endpoints:
        try:
            response = requests.get(f"{INSTANCE_URL}{endpoint}")
            print(f"\n{endpoint}:")
            print(f"  Status: {response.status_code}")
            print(f"  Content-Type: {response.headers.get('Content-Type', 'N/A')}")
            print(f"  CORS: {response.headers.get('Access-Control-Allow-Origin', 'N/A')}")
            
            # Test OPTIONS for CORS
            options_response = requests.options(f"{INSTANCE_URL}{endpoint}")
            print(f"  OPTIONS Status: {options_response.status_code}")
            print(f"  Allow Methods: {options_response.headers.get('Access-Control-Allow-Methods', 'N/A')}")
        except Exception as e:
            print(f"\n{endpoint}: Failed - {e}")

def test_missing_endpoints():
    """Test for commonly expected but possibly missing endpoints"""
    print("\n\n=== Testing Additional Endpoints ===\n")
    
    endpoints = [
        ('/api/v1/instance/extended_description', 'Extended description'),
        ('/api/v1/instance/rules', 'Instance rules'),
        ('/api/v2/instance', 'Instance v2'),
        ('/api/v1/trends', 'Trends'),
        ('/api/v1/trends/statuses', 'Trending statuses'),
        ('/api/v1/trends/tags', 'Trending tags'),
        ('/api/v1/trends/links', 'Trending links'),
        ('/api/v1/directory', 'Profile directory'),
        ('/api/v1/announcements', 'Announcements'),
    ]
    
    for endpoint, description in endpoints:
        try:
            response = requests.get(f"{INSTANCE_URL}{endpoint}")
            if response.status_code == 200:
                data = response.json()
                if isinstance(data, list):
                    print(f"✓ {endpoint} - {description}: {len(data)} items")
                else:
                    print(f"✓ {endpoint} - {description}: OK")
            else:
                print(f"✗ {endpoint} - {description}: Status {response.status_code}")
        except Exception as e:
            print(f"✗ {endpoint} - {description}: Failed - {str(e)[:50]}")

def main():
    """Run all automated tests"""
    print(f"🧪 Testing lesser API at {INSTANCE_URL}\n")
    print("=" * 50)
    
    test_public_endpoints()
    test_webfinger()
    test_nodeinfo()
    test_oauth_endpoints()
    test_api_response_headers()
    test_missing_endpoints()
    
    print("\n" + "=" * 50)
    print("\n✅ Automated testing complete!")
    print("\nFor authenticated endpoint testing, run test_api.py")

if __name__ == "__main__":
    main() 