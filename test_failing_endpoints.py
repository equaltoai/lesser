#!/usr/bin/env python3
import requests
import os
import json

# Get token from environment or file
token = os.environ.get('LESSER_TOKEN', '')
if not token and os.path.exists(os.path.expanduser('~/.lesser_token')):
    with open(os.path.expanduser('~/.lesser_token'), 'r') as f:
        token = f.read().strip()

if not token:
    print("No token found. Please set LESSER_TOKEN or create ~/.lesser_token")
    exit(1)

base_url = "https://lesser.host"
headers = {
    "Authorization": f"Bearer {token}",
    "Content-Type": "application/json"
}

# Test endpoints that are failing
failing_endpoints = [
    ("GET", "/api/v1/moderation/trust", None),
    ("GET", "/api/v1/ai/stats", None),
    ("GET", "/api/v1/reputation/https://lesser.host/users/aron", None),
    ("GET", "/api/v1/accounts/aron/notes", None),
]

print(f"Testing failing endpoints with token: {token[:20]}...")
print("=" * 80)

for method, endpoint, data in failing_endpoints:
    url = f"{base_url}{endpoint}"
    print(f"\n{method} {endpoint}")
    print("-" * 40)
    
    try:
        if method == "GET":
            response = requests.get(url, headers=headers)
        else:
            response = requests.request(method, url, headers=headers, json=data)
        
        print(f"Status: {response.status_code}")
        print(f"Headers: {dict(response.headers)}")
        
        try:
            body = response.json()
            print(f"Response: {json.dumps(body, indent=2)}")
        except:
            print(f"Response (text): {response.text}")
            
    except Exception as e:
        print(f"Error: {e}") 