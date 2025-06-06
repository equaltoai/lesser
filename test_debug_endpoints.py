#!/usr/bin/env python3
"""
Test script for Lesser debug endpoints
Tests the developer experience debug endpoints:
- Federation trace
- Object inspection  
- Activity replay
- Federation domain debugging
- Object explanation with storage details
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


def test_activity_replay(token, activity_id=None):
    """Test the activity replay endpoint"""
    print("\n=== Testing Activity Replay ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    
    if not activity_id:
        # Create a test activity first
        post_data = {
            "status": f"Activity replay test at {datetime.now(timezone.utc).isoformat()}"
        }
        response = requests.post(f"{API_URL}/statuses", json=post_data, headers=headers)
        if response.status_code == 200:
            status = response.json()
            activity_id = status["url"].split("/")[-1]
            print(f"Created test activity: {activity_id}")
        else:
            print(f"Failed to create test activity: {response.status_code}")
            return
    
    replay_url = f"{API_URL}/debug/replay/{activity_id}"
    
    response = requests.post(replay_url, headers=headers)
    
    if response.status_code == 200:
        print("Activity replayed successfully!")
        result = response.json()
        print(f"\nReplay Result:")
        print(json.dumps(result, indent=2))
    elif response.status_code == 400:
        print("Bad request:", response.json())
    elif response.status_code == 404:
        print(f"Activity not found: {activity_id}")
    else:
        print(f"Error: {response.status_code}")
        print(response.text)


def test_federation_domain(token, domain="mastodon.social"):
    """Test the federation domain debug endpoint"""
    print(f"\n=== Testing Federation Domain Debug ({domain}) ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    domain_url = f"{API_URL}/debug/federation/domain/{domain}"
    
    response = requests.get(domain_url, headers=headers)
    
    if response.status_code == 200:
        domain_info = response.json()
        print(f"\nDomain: {domain_info['domain']}")
        print(f"Status: {domain_info['status']}")
        print(f"Last Contact: {domain_info.get('last_contact', 'Never')}")
        print(f"Shared Inbox: {domain_info.get('shared_inbox', 'Unknown')}")
        print(f"Activity Count: {domain_info['activity_count']}")
        
        if domain_info.get('known_actors'):
            print(f"\nKnown Actors ({len(domain_info['known_actors'])}):")
            for actor in domain_info['known_actors'][:5]:  # Show first 5
                print(f"  - {actor}")
                
        if domain_info.get('instance_info'):
            print(f"\nInstance Info:")
            software = domain_info['instance_info'].get('software', {})
            print(f"  Software: {software.get('name', 'unknown')} {software.get('version', '')}")
            protocols = domain_info['instance_info'].get('protocols', [])
            print(f"  Protocols: {', '.join(protocols)}")
            
        if domain_info.get('recent_errors'):
            print(f"\nRecent Errors:")
            for error in domain_info['recent_errors'][:3]:  # Show first 3
                print(f"  - {error}")
                
    elif response.status_code == 401:
        print("Unauthorized - admin or debug scope required")
    else:
        print(f"Error: {response.status_code}")
        print(response.text)


def test_object_explanation(token, object_id=None):
    """Test the object explanation endpoint"""
    print("\n=== Testing Object Explanation ===")
    
    headers = {"Authorization": f"Bearer {token}"}
    
    if not object_id:
        # Create a test object first
        post_data = {
            "status": f"Object explanation test at {datetime.now(timezone.utc).isoformat()}",
            "visibility": "public"
        }
        response = requests.post(f"{API_URL}/statuses", json=post_data, headers=headers)
        if response.status_code == 200:
            status = response.json()
            object_id = status["url"].split("/")[-1]
            print(f"Created test object: {object_id}")
            
            # Like it to create references
            requests.post(f"{API_URL}/statuses/{object_id}/favourite", headers=headers)
        else:
            print(f"Failed to create test object: {response.status_code}")
            return
    
    # Test object explanation
    explain_url = f"{API_URL}/debug/objects/{object_id}/explain"
    response = requests.get(explain_url, headers=headers)
    
    if response.status_code == 200:
        explanation = response.json()
        
        print(f"\n=== Storage Details ===")
        storage = explanation.get('storage', {})
        print(f"Table: {storage.get('table')}")
        print(f"Partition Key: {storage.get('partition_key')}")
        print(f"Sort Key: {storage.get('sort_key')}")
        print(f"Size: {storage.get('size_bytes')} bytes")
        print(f"Last Modified: {storage.get('last_modified')}")
        
        print(f"\n=== Indexes Used ===")
        for index in explanation.get('indexes', []):
            print(f"  - {index}")
            
        print(f"\n=== References ===")
        refs = explanation.get('references', {})
        print(f"Likes: {refs.get('likes', 0)}")
        print(f"Announces: {refs.get('announces', 0)}")
        print(f"Replies: {refs.get('replies', 0)}")
        
        print(f"\n=== Cost Breakdown ===")
        cost = explanation.get('cost_breakdown', {})
        print(f"Read Cost Units: {cost.get('read_cost_units')}")
        print(f"Write Cost Units: {cost.get('write_cost_units')}")
        print(f"Storage Cost Monthly: {cost.get('storage_cost_monthly')}")
        print(f"Total Access Cost: {cost.get('total_access_cost')}")
        
        if cost.get('explanation'):
            print(f"\nCost Explanation:")
            for key, value in cost['explanation'].items():
                print(f"  {key}: {value}")
                
        # Check cost header
        print(f"\nCost Header: {response.headers.get('X-Cost-Micros')} micros")
        
    elif response.status_code == 401:
        print("Unauthorized - admin or debug scope required")
    elif response.status_code == 404:
        print(f"Object not found: {object_id}")
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
    
    # Test all debug endpoints
    test_federation_trace(token)
    test_object_inspection(token)
    test_activity_replay(token)
    test_federation_domain(token)
    test_object_explanation(token)
    
    # Test with specific IDs if provided
    if len(sys.argv) > 1:
        if sys.argv[1] == "trace" and len(sys.argv) > 2:
            test_federation_trace(token, sys.argv[2])
        elif sys.argv[1] == "inspect" and len(sys.argv) > 2:
            test_object_inspection(token, sys.argv[2])
        elif sys.argv[1] == "replay" and len(sys.argv) > 2:
            test_activity_replay(token, sys.argv[2])
        elif sys.argv[1] == "domain" and len(sys.argv) > 2:
            test_federation_domain(token, sys.argv[2])
        elif sys.argv[1] == "explain" and len(sys.argv) > 2:
            test_object_explanation(token, sys.argv[2])
    
    print("\n=== Debug Endpoints Summary ===")
    print("✅ Federation Trace: /api/v1/debug/federation/trace/:activity_id")
    print("✅ Object Inspection: /api/v1/debug/objects/:object_id")
    print("✅ Activity Replay: /api/v1/debug/replay/:activity_id")
    print("✅ Federation Domain: /api/v1/debug/federation/domain/:domain")
    print("✅ Object Explanation: /api/v1/debug/objects/:object_id/explain")
    print("\nAll debug endpoints require admin or debug scope!")


if __name__ == "__main__":
    main() 