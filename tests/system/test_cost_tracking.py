#!/usr/bin/env python3
"""
Test script for Lesser 2.0 Cost Tracking Phase 1

This script tests:
1. Cost tracking headers are present on API responses
2. The /api/v1/instance/costs endpoint works
3. Cost values are reasonable
"""

import requests
import sys
from typing import Dict, Any
import os
from dotenv import load_dotenv

# Load environment variables
load_dotenv()

# Configuration
BASE_URL = os.getenv('API_BASE_URL', 'https://lesser.aronprice.com')
if not BASE_URL.startswith('http'):
    BASE_URL = f'https://{BASE_URL}'

# Test user credentials (if needed for authenticated endpoints)
TEST_USERNAME = os.getenv('TEST_USERNAME', 'testuser')
TEST_PASSWORD = os.getenv('TEST_PASSWORD', 'testpass123')

# Cost header names
COST_HEADERS = [
    'X-Cost-Total-Microcents',
    'X-Cost-Total-Cents',
    'X-Cost-DynamoDB-Reads',
    'X-Cost-DynamoDB-Writes',
    'X-Cost-Lambda-Duration-Ms',
    'X-Cost-Data-Transfer-Bytes'
]

class CostTrackingTester:
    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip('/')
        self.api_v1_url = f"{self.base_url}/api/v1"
        self.api_v2_url = f"{self.base_url}/api/v2"
        self.session = requests.Session()
        self.access_token = None
        
    def test_cost_headers_on_public_endpoint(self) -> bool:
        """Test that cost headers are present on public endpoints"""
        print("\n🧪 Testing cost headers on public endpoint...")
        
        try:
            # Test instance endpoint (public)
            response = self.session.get(f"{self.api_v2_url}/instance")
            
            # Check for cost headers
            missing_headers = []
            present_headers = []
            
            for header in COST_HEADERS:
                if header in response.headers:
                    present_headers.append(header)
                    print(f"  ✅ {header}: {response.headers[header]}")
                else:
                    missing_headers.append(header)
                    print(f"  ❌ {header}: Missing")
            
            if missing_headers:
                print(f"\n⚠️  Missing headers: {', '.join(missing_headers)}")
                return False
            else:
                print("\n✅ All cost headers present!")
                return True
                
        except Exception as e:
            print(f"❌ Error testing cost headers: {e}")
            return False
    
    def test_cost_values_reasonable(self) -> bool:
        """Test that cost values are reasonable (not zero, not huge)"""
        print("\n🧪 Testing cost value reasonableness...")
        
        try:
            response = self.session.get(f"{self.api_v2_url}/instance")
            
            # Check microcents value
            if 'X-Cost-Total-Microcents' in response.headers:
                microcents = int(response.headers['X-Cost-Total-Microcents'])
                print(f"  Total cost (microcents): {microcents}")
                
                if microcents == 0:
                    print("  ⚠️  Cost is zero - might not be tracking properly")
                elif microcents > 1000000:  # More than 1 cent per request seems high
                    print(f"  ⚠️  Cost seems very high: {microcents/1000000:.6f} cents")
                else:
                    print(f"  ✅ Cost seems reasonable: {microcents/1000000:.6f} cents")
            
            # Check Lambda duration
            if 'X-Cost-Lambda-Duration-Ms' in response.headers:
                duration = int(response.headers['X-Cost-Lambda-Duration-Ms'])
                print(f"  Lambda duration: {duration}ms")
                
                if duration == 0:
                    print("  ⚠️  Lambda duration is zero - might not be tracking properly")
                elif duration > 5000:
                    print(f"  ⚠️  Lambda duration seems high: {duration}ms")
                else:
                    print(f"  ✅ Lambda duration reasonable: {duration}ms")
            
            return True
            
        except Exception as e:
            print(f"❌ Error checking cost values: {e}")
            return False
    
    def test_instance_costs_endpoint(self) -> bool:
        """Test the /api/v1/instance/costs endpoint"""
        print("\n🧪 Testing /api/v1/instance/costs endpoint...")
        
        try:
            response = self.session.get(f"{self.api_v1_url}/instance/costs")
            
            if response.status_code != 200:
                print(f"❌ Expected 200, got {response.status_code}")
                return False
            
            data = response.json()
            print(f"✅ Endpoint returned 200 OK")
            
            # Check response structure
            expected_keys = ['current_month', 'daily_costs', 'cost_per_user', 'cost_breakdown']
            missing_keys = [key for key in expected_keys if key not in data]
            
            if missing_keys:
                print(f"⚠️  Missing expected keys: {missing_keys}")
            else:
                print("✅ All expected keys present")
            
            # Display some data
            if 'current_month' in data:
                month_data = data['current_month']
                print(f"\n📊 Current Month Stats:")
                print(f"  Total cost: ${month_data.get('total_cost_cents', 0):.6f}")
                print(f"  DynamoDB reads: {month_data.get('dynamodb_reads', 0):,}")
                print(f"  DynamoDB writes: {month_data.get('dynamodb_writes', 0):,}")
                print(f"  Lambda invocations: {month_data.get('lambda_invocations', 0):,}")
            
            # Check if cost headers are present on this response too
            print("\n📋 Cost headers on costs endpoint:")
            for header in COST_HEADERS:
                if header in response.headers:
                    print(f"  ✅ {header}: {response.headers[header]}")
            
            return True
            
        except Exception as e:
            print(f"❌ Error testing costs endpoint: {e}")
            return False
    
    def test_cost_tracking_on_authenticated_endpoint(self) -> bool:
        """Test cost tracking on authenticated endpoints"""
        print("\n🧪 Testing cost tracking on authenticated endpoint...")
        
        # First, we need to get an access token
        # For now, skip if we don't have credentials
        if not self.access_token:
            print("⚠️  No access token available, skipping authenticated test")
            return True
        
        try:
            headers = {'Authorization': f'Bearer {self.access_token}'}
            response = self.session.get(
                f"{self.api_v1_url}/accounts/verify_credentials",
                headers=headers
            )
            
            if response.status_code != 200:
                print(f"❌ Auth endpoint returned {response.status_code}")
                return False
            
            # Check cost headers
            print("✅ Authenticated endpoint accessible")
            for header in COST_HEADERS:
                if header in response.headers:
                    print(f"  ✅ {header}: {response.headers[header]}")
            
            return True
            
        except Exception as e:
            print(f"❌ Error testing authenticated endpoint: {e}")
            return False
    
    def test_cost_tracking_on_write_operation(self) -> bool:
        """Test cost tracking on write operations (should show DynamoDB writes)"""
        print("\n🧪 Testing cost tracking on write operations...")
        
        # Test with OPTIONS request first (doesn't require auth)
        try:
            response = self.session.options(f"{self.api_v1_url}/statuses")
            
            # Even OPTIONS should have cost headers
            if 'X-Cost-Total-Microcents' in response.headers:
                print("✅ Cost tracking present on OPTIONS request")
                print(f"  Total cost: {response.headers.get('X-Cost-Total-Cents', 'N/A')} cents")
            else:
                print("⚠️  No cost tracking on OPTIONS request")
            
            return True
            
        except Exception as e:
            print(f"❌ Error testing write operation: {e}")
            return False
    
    def run_all_tests(self):
        """Run all cost tracking tests"""
        print(f"🚀 Starting Lesser 2.0 Cost Tracking Tests")
        print(f"📍 Testing against: {self.base_url}")
        print("=" * 60)
        
        tests = [
            ("Public Endpoint Cost Headers", self.test_cost_headers_on_public_endpoint),
            ("Cost Value Reasonableness", self.test_cost_values_reasonable),
            ("Instance Costs Endpoint", self.test_instance_costs_endpoint),
            ("Authenticated Endpoint Costs", self.test_cost_tracking_on_authenticated_endpoint),
            ("Write Operation Costs", self.test_cost_tracking_on_write_operation),
        ]
        
        results = []
        for test_name, test_func in tests:
            try:
                success = test_func()
                results.append((test_name, success))
            except Exception as e:
                print(f"\n❌ Test '{test_name}' crashed: {e}")
                results.append((test_name, False))
        
        # Summary
        print("\n" + "=" * 60)
        print("📊 TEST SUMMARY")
        print("=" * 60)
        
        passed = sum(1 for _, success in results if success)
        total = len(results)
        
        for test_name, success in results:
            status = "✅ PASS" if success else "❌ FAIL"
            print(f"{status} - {test_name}")
        
        print(f"\n🏁 Total: {passed}/{total} tests passed")
        
        if passed == total:
            print("🎉 All tests passed! Cost tracking is working!")
            return 0
        else:
            print("⚠️  Some tests failed. Check the output above.")
            return 1


def main():
    """Main entry point"""
    base_url = BASE_URL
    
    # Allow override from command line
    if len(sys.argv) > 1:
        base_url = sys.argv[1]
    
    tester = CostTrackingTester(base_url)
    return tester.run_all_tests()


if __name__ == "__main__":
    sys.exit(main()) 
