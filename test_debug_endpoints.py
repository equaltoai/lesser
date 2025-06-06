#!/usr/bin/env python3
"""
Test script for Lesser debug endpoints
Tests the developer experience debug endpoints:
- Federation trace
- Object inspection  
- Activity replay (when implemented)
"""

import requests
import json
import sys
import time
from datetime import datetime, timezone

# Configuration
BASE_URL = "https://instance.lesser.social"
API_URL = f"{BASE_URL}/api/v1"

# Test credentials - create a test user with admin scope
USERNAME = "admin"
PASSWORD = "adminpass"


def get_oauth_token(client_id, client_secret):
    """Get OAuth token for testing"""
    token_url = f"{BASE_URL}/oauth/token"
    data = {
        "grant_type": "password",
        "client_id": client_id,
        "client_secret": client_secret,
        "username": USERNAME,
        "password": PASSWORD,
        "scope": "read write admin debug"
    }
    
    response = requests.post(token_url, data=data)
    if response.status_code == 200:
        return response.json()["access_token"]
    else:
        print(f"Failed to get token: {response.status_code}")
        print(response.text)
        sys.exit(1)


def create_oauth_client():
    """Create OAuth client for testing"""
    # First, create an OAuth app
    app_data = {
        "client_name": "Debug Test Client",
        "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
        "scopes": "read write admin debug",
        "website": f"{BASE_URL}"
    }
    
    response = requests.post(f"{API_URL}/apps", json=app_data)
    if response.status_code == 200:
        app = response.json()
        return app["client_id"], app["client_secret"]
    else:
        print(f"Failed to create app: {response.status_code}")
        print(response.text)
        sys.exit(1)


def test_federation_trace(token, activity_id=None):
    """Test the federation trace endpoint"""
    print("\n=== Testing Federation Trace ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    
    if not activity_id:
        # First create a test post to get an activity
        post_data = {
            "status": f"Debug test post at {datetime.now(timezone.utc).isoformat()}"
        }
        response = requests.post(f"{API_URL}/statuses", json=post_data, headers=headers)
        if response.status_code == 200:
            status = response.json()
            # Extract activity ID from the status URL
            activity_id = status["url"].split("/")[-1]
            print(f"Created test status with activity ID: {activity_id}")
        else:
            print(f"Failed to create test status: {response.status_code}")
            return
    
    # Test federation trace
    trace_url = f"{API_URL}/debug/federation/trace/{activity_id}"
    response = requests.get(trace_url, headers=headers)
    
    if response.status_code == 200:
        trace = response.json()
        print(f"\nActivity ID: {trace['activity_id']}")
        print(f"Type: {trace['type']}")
        print(f"Actor: {trace['actor']}")
        print(f"Created: {trace.get('created', 'N/A')}")
        print(f"Processing Time: {trace['processing_time']}")
        print(f"\nStorage Locations:")
        for location, path in trace.get('storage_locations', {}).items():
            print(f"  {location}: {path}")
        
        print(f"\nTraces ({len(trace['traces'])} events):")
        for t in trace['traces']:
            print(f"  [{t['timestamp']}] {t['step']} ({t['direction']})")
            if t.get('error'):
                print(f"    ERROR: {t['error']}")
            if t.get('remote_url'):
                print(f"    Remote: {t['remote_url']}")
            if t.get('status_code'):
                print(f"    Status: {t['status_code']}")
                
        # Check response headers
        print(f"\nDebug Headers:")
        print(f"  Processing Time: {response.headers.get('X-Processing-Time')}")
        print(f"  Trace Count: {response.headers.get('X-Debug-Traces')}")
        
    elif response.status_code == 401:
        print("Unauthorized - admin or debug scope required")
    elif response.status_code == 404:
        print(f"Activity not found: {activity_id}")
    else:
        print(f"Error: {response.status_code}")
        print(response.text)


def test_object_inspection(token, object_id=None):
    """Test the object inspection endpoint"""
    print("\n=== Testing Object Inspection ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    
    if not object_id:
        # Create a test object first
        post_data = {
            "status": f"Object inspection test at {datetime.now(timezone.utc).isoformat()}",
            "visibility": "public"
        }
        response = requests.post(f"{API_URL}/statuses", json=post_data, headers=headers)
        if response.status_code == 200:
            status = response.json()
            object_id = status["url"]
            print(f"Created test object: {object_id}")
        else:
            print(f"Failed to create test object: {response.status_code}")
            return
    
    # Test object inspection
    inspect_url = f"{API_URL}/debug/objects/{object_id.split('/')[-1]}"
    response = requests.get(inspect_url, headers=headers)
    
    if response.status_code == 200:
        obj = response.json()
        print(f"\nObject ID: {obj['id']}")
        print(f"Type: {obj['type']}")
        print(f"Created: {obj.get('created', 'N/A')}")
        
        if obj.get('actor'):
            print(f"\nActor:")
            for key, value in obj['actor'].items():
                print(f"  {key}: {value}")
        
        print(f"\nRelationships:")
        for rel, data in obj.get('relationships', {}).items():
            print(f"  {rel}: {json.dumps(data, indent=2)}")
        
        print(f"\nObject Data:")
        # Pretty print the object
        if isinstance(obj.get('object'), dict):
            for key, value in obj['object'].items():
                if isinstance(value, (dict, list)):
                    print(f"  {key}: {json.dumps(value, indent=4)}")
                else:
                    print(f"  {key}: {value}")
                    
    elif response.status_code == 401:
        print("Unauthorized - admin or debug scope required")
    elif response.status_code == 404:
        print(f"Object not found: {object_id}")
    else:
        print(f"Error: {response.status_code}")
        print(response.text)


def test_activity_replay(token, activity_id):
    """Test the activity replay endpoint (when implemented)"""
    print("\n=== Testing Activity Replay ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    replay_url = f"{API_URL}/debug/replay/{activity_id}"
    
    response = requests.post(replay_url, headers=headers)
    
    if response.status_code == 501:
        print("Activity replay not yet implemented")
        print(response.json())
    elif response.status_code == 200:
        print("Activity replayed successfully")
        result = response.json()
        print(json.dumps(result, indent=2))
    else:
        print(f"Error: {response.status_code}")
        print(response.text)


def main():
    """Run debug endpoint tests"""
    print("Lesser Debug Endpoints Test")
    print("===========================")
    
    # Create OAuth client and get token
    print("\nSetting up authentication...")
    client_id, client_secret = create_oauth_client()
    token = get_oauth_token(client_id, client_secret)
    print("Authentication successful!")
    
    # Test federation trace
    test_federation_trace(token)
    
    # Test object inspection
    test_object_inspection(token)
    
    # Test with specific IDs if provided
    if len(sys.argv) > 1:
        if sys.argv[1] == "trace" and len(sys.argv) > 2:
            test_federation_trace(token, sys.argv[2])
        elif sys.argv[1] == "inspect" and len(sys.argv) > 2:
            test_object_inspection(token, sys.argv[2])
        elif sys.argv[1] == "replay" and len(sys.argv) > 2:
            test_activity_replay(token, sys.argv[2])
    
    print("\n=== Debug Endpoints Summary ===")
    print("✅ Federation Trace: /api/v1/debug/federation/trace/:activity_id")
    print("✅ Object Inspection: /api/v1/debug/objects/:object_id")
    print("🚧 Activity Replay: /api/v1/debug/replay/:activity_id (coming soon)")
    print("\nAll debug endpoints require admin or debug scope!")


if __name__ == "__main__":
    main() 