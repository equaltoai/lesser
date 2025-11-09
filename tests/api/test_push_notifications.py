#!/usr/bin/env python3
"""
Test script for Lesser push notifications functionality.
Tests VAPID key generation, subscription management, and notification delivery.
"""

import argparse
import requests
import sys
import base64
import os
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.backends import default_backend

def test_vapid_keys(base_url, token):
    """Test that VAPID keys are properly configured"""
    print("\n🔑 Testing VAPID key configuration...")
    
    # Get instance info
    response = requests.get(f"{base_url}/api/v2/instance")
    if response.status_code != 200:
        print(f"❌ Failed to get instance info: {response.status_code}")
        return False
    
    instance_data = response.json()
    vapid_key = instance_data.get('configuration', {}).get('vapid', {}).get('public_key')
    
    if not vapid_key:
        print("❌ No VAPID public key found in instance configuration")
        print("   Run: ./bin/configure-instance -generate-vapid")
        return False
    
    # Validate it's a valid base64 key
    try:
        key_bytes = base64.urlsafe_b64decode(vapid_key + '==')  # Add padding
        if len(key_bytes) != 65:  # Uncompressed P-256 public key
            print(f"❌ Invalid VAPID key length: {len(key_bytes)} (expected 65)")
            return False
    except Exception as e:
        print(f"❌ Invalid VAPID key format: {e}")
        return False
    
    print(f"✅ VAPID public key configured: {vapid_key[:20]}...")
    return True

def test_get_subscription(base_url, token):
    """Test getting current push subscription"""
    print("\n📱 Testing GET push subscription...")
    
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.get(f"{base_url}/api/v1/push/subscription", headers=headers)
    
    if response.status_code != 200:
        print(f"❌ Failed to get subscription: {response.status_code}")
        print(f"   Response: {response.text}")
        return None
    
    data = response.json()
    
    if data.get('id'):
        print(f"✅ Found existing subscription: {data['id']}")
        print(f"   Endpoint: {data.get('endpoint', '')[:50]}...")
        alerts = data.get('alerts', {})
        enabled_alerts = [k for k, v in alerts.items() if v]
        print(f"   Enabled alerts: {', '.join(enabled_alerts) if enabled_alerts else 'None'}")
    else:
        print("ℹ️  No existing subscription found")
    
    return data

def test_create_subscription(base_url, token, endpoint=None):
    """Test creating a push subscription"""
    print("\n📝 Testing POST push subscription...")
    
    if not endpoint:
        # Use a dummy endpoint for testing
        endpoint = "https://fcm.googleapis.com/fcm/send/test-endpoint-12345"
    
    # Generate dummy keys for testing
    private_key = ec.generate_private_key(ec.SECP256R1(), default_backend())
    public_key = private_key.public_key()
    
    # Get public key bytes
    public_bytes = public_key.public_bytes(
        encoding=serialization.Encoding.X962,
        format=serialization.PublicFormat.UncompressedPoint
    )
    p256dh = base64.urlsafe_b64encode(public_bytes).decode('utf-8').rstrip('=')
    
    # Generate random auth secret
    auth = base64.urlsafe_b64encode(os.urandom(16)).decode('utf-8').rstrip('=')
    
    payload = {
        "subscription": {
            "endpoint": endpoint,
            "keys": {
                "p256dh": p256dh,
                "auth": auth
            }
        },
        "data": {
            "follow": True,
            "favourite": True,
            "reblog": True,
            "mention": True,
            "poll": False,
            "follow_request": True,
            "status": False,
            "update": False,
            "admin.sign_up": False,
            "admin.report": False
        }
    }
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    response = requests.post(
        f"{base_url}/api/v1/push/subscription",
        headers=headers,
        json=payload
    )
    
    if response.status_code != 200:
        print(f"❌ Failed to create subscription: {response.status_code}")
        print(f"   Response: {response.text}")
        return None
    
    data = response.json()
    print(f"✅ Created subscription: {data.get('id', 'unknown')}")
    print(f"   Server key: {data.get('server_key', '')[:20]}...")
    
    return data

def test_update_subscription(base_url, token):
    """Test updating push subscription alerts"""
    print("\n🔄 Testing PUT push subscription...")
    
    payload = {
        "data": {
            "follow": False,
            "favourite": True,
            "reblog": False,
            "mention": True,
            "poll": True,
            "follow_request": False,
            "status": False,
            "update": False,
            "admin.sign_up": False,
            "admin.report": False
        }
    }
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    response = requests.put(
        f"{base_url}/api/v1/push/subscription",
        headers=headers,
        json=payload
    )
    
    if response.status_code != 200:
        print(f"❌ Failed to update subscription: {response.status_code}")
        print(f"   Response: {response.text}")
        return False
    
    data = response.json()
    alerts = data.get('alerts', {})
    enabled_alerts = [k for k, v in alerts.items() if v]
    print(f"✅ Updated subscription alerts")
    print(f"   Enabled: {', '.join(enabled_alerts)}")
    
    return True

def test_delete_subscription(base_url, token):
    """Test deleting push subscription"""
    print("\n🗑️  Testing DELETE push subscription...")
    
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.delete(f"{base_url}/api/v1/push/subscription", headers=headers)
    
    if response.status_code != 204:
        print(f"❌ Failed to delete subscription: {response.status_code}")
        print(f"   Response: {response.text}")
        return False
    
    print("✅ Deleted subscription")
    return True

def test_notification_trigger(base_url, token, username):
    """Test triggering a notification by following/unfollowing a test account"""
    print("\n🔔 Testing notification trigger...")
    
    # First, make sure we have a subscription
    sub = test_get_subscription(base_url, token)
    if not sub or not sub.get('id'):
        print("ℹ️  Creating test subscription first...")
        sub = test_create_subscription(base_url, token)
        if not sub:
            print("❌ Failed to create test subscription")
            return False
    
    # Find a test account to follow (could be yourself or a test account)
    test_account = username  # Following yourself for testing
    
    print(f"   Following @{test_account} to trigger notification...")
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Follow the account
    response = requests.post(
        f"{base_url}/api/v1/accounts/{test_account}/follow",
        headers=headers
    )
    
    if response.status_code == 200:
        print(f"✅ Successfully followed @{test_account}")
        print("   A push notification should be queued if infrastructure is deployed")
        
        # Unfollow to clean up
        requests.post(f"{base_url}/api/v1/accounts/{test_account}/unfollow", headers=headers)
    else:
        print(f"❌ Failed to follow account: {response.status_code}")
        return False
    
    return True

def main():
    parser = argparse.ArgumentParser(description='Test Lesser push notifications')
    parser.add_argument('base_url', help='Base URL of the Lesser instance (e.g., https://lesser.example.com)')
    parser.add_argument('--token', required=True, help='OAuth access token with push scope')
    parser.add_argument('--username', help='Your username (for notification trigger test)')
    parser.add_argument('--skip-vapid', action='store_true', help='Skip VAPID key test')
    parser.add_argument('--skip-get', action='store_true', help='Skip GET subscription test')
    parser.add_argument('--skip-create', action='store_true', help='Skip CREATE subscription test')
    parser.add_argument('--skip-update', action='store_true', help='Skip UPDATE subscription test')
    parser.add_argument('--skip-delete', action='store_true', help='Skip DELETE subscription test')
    parser.add_argument('--skip-trigger', action='store_true', help='Skip notification trigger test')
    
    args = parser.parse_args()
    
    # Remove trailing slash from base URL
    base_url = args.base_url.rstrip('/')
    
    print(f"🧪 Testing push notifications on {base_url}")
    
    all_passed = True
    
    # Test VAPID keys
    if not args.skip_vapid:
        if not test_vapid_keys(base_url, args.token):
            all_passed = False
    
    # Test GET subscription
    if not args.skip_get:
        test_get_subscription(base_url, args.token)
    
    # Test CREATE subscription
    if not args.skip_create:
        if not test_create_subscription(base_url, args.token):
            all_passed = False
    
    # Test UPDATE subscription
    if not args.skip_update:
        if not test_update_subscription(base_url, args.token):
            all_passed = False
    
    # Test notification trigger
    if not args.skip_trigger and args.username:
        if not test_notification_trigger(base_url, args.token, args.username):
            all_passed = False
    
    # Test DELETE subscription (do this last)
    if not args.skip_delete:
        if not test_delete_subscription(base_url, args.token):
            all_passed = False
    
    print("\n" + "="*50)
    if all_passed:
        print("✅ All push notification tests passed!")
    else:
        print("❌ Some tests failed. Check the output above.")
        sys.exit(1)

if __name__ == "__main__":
    main() 
