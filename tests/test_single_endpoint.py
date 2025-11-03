#!/usr/bin/env python3
"""
Test a single API endpoint quickly
"""

import requests
import json
import sys
import os
from urllib.parse import urljoin

def test_endpoint(base_url, method, path, token=None, data=None, params=None):
    """Test a single endpoint"""
    # Prepare request
    url = urljoin(base_url, f"/api/v1{path}")
    headers = {}
    
    if token:
        headers['Authorization'] = f'Bearer {token}'
    
    # Make request
    try:
        response = requests.request(
            method=method,
            url=url,
            headers=headers,
            json=data,
            params=params,
            timeout=30
        )
        
        # Print results
        print(f"\n{'='*60}")
        print(f"🧪 Endpoint Test")
        print(f"{'='*60}")
        print(f"Method: {method}")
        print(f"URL: {url}")
        if params:
            print(f"Params: {params}")
        if data:
            print(f"Data: {json.dumps(data, indent=2)}")
        print(f"\n📡 Response:")
        print(f"Status: {response.status_code}")
        print(f"Headers:")
        for key, value in response.headers.items():
            if key.lower().startswith('x-lesser') or key.lower() == 'content-type':
                print(f"  {key}: {value}")
        
        # Try to parse JSON response
        try:
            response_data = response.json()
            print(f"\nBody:")
            print(json.dumps(response_data, indent=2))
        except ValueError:
            # Not JSON, print raw
            print(f"\nBody (raw):")
            print(response.text[:500])
            if len(response.text) > 500:
                print("... (truncated)")
        
        # Check for cost headers
        cost_headers = {k: v for k, v in response.headers.items() if k.lower().startswith('x-lesser-cost')}
        if cost_headers:
            print(f"\n💰 Cost Tracking:")
            for key, value in cost_headers.items():
                print(f"  {key}: {value}")
        
        print(f"\n{'='*60}")
        
        # Return success/failure
        return response.status_code < 400
        
    except Exception as e:
        print(f"\n❌ Request failed: {e}")
        return False

def main():
    """Main entry point"""
    if len(sys.argv) < 3:
        print("Usage: test_single_endpoint.py <method> <path> [token] [data]")
        print("\nExamples:")
        print("  test_single_endpoint.py GET /instance")
        print("  test_single_endpoint.py GET /accounts/verify_credentials YOUR_TOKEN")
        print("  test_single_endpoint.py POST /statuses YOUR_TOKEN '{\"status\":\"Test post\"}'")
        print("\nEnvironment variables:")
        print("  LESSER_URL - Base URL (default: https://lesser.example.com)")
        print("  LESSER_TOKEN - Access token")
        sys.exit(1)
    
    # Parse arguments
    method = sys.argv[1].upper()
    path = sys.argv[2]
    
    # Get base URL from environment or default
    base_url = os.environ.get('LESSER_URL', 'https://lesser.example.com')
    
    # Get token from argument or environment
    token = None
    if len(sys.argv) > 3:
        token = sys.argv[3]
    elif 'LESSER_TOKEN' in os.environ:
        token = os.environ['LESSER_TOKEN']
    
    # Parse data if provided
    data = None
    if len(sys.argv) > 4:
        try:
            data = json.loads(sys.argv[4])
        except json.JSONDecodeError:
            print(f"Warning: Could not parse data as JSON: {sys.argv[4]}")
    
    # Extract params from path if present
    params = None
    if '?' in path:
        path, param_string = path.split('?', 1)
        params = {}
        for param in param_string.split('&'):
            if '=' in param:
                key, value = param.split('=', 1)
                params[key] = value
    
    print(f"🔗 Testing against: {base_url}")
    
    # Test the endpoint
    success = test_endpoint(base_url, method, path, token, data, params)
    
    # Exit with appropriate code
    sys.exit(0 if success else 1)

if __name__ == '__main__':
    main() 
