#!/usr/bin/env python3
"""
Test script for Lesser 2.0 Enhanced Metrics API (Phase 1.3)

This script tests:
1. Current instance metrics endpoint
2. Daily aggregates endpoint
3. Predictive analytics endpoint
"""

import requests
import json
import sys
from datetime import datetime, timedelta
from typing import Dict, Any
import os
from dotenv import load_dotenv

# Load environment variables
load_dotenv()

# Configuration
BASE_URL = os.getenv('API_BASE_URL', 'https://lesser.aronprice.com')
if not BASE_URL.startswith('http'):
    BASE_URL = f'https://{BASE_URL}'

class EnhancedMetricsTester:
    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip('/')
        self.api_v1_url = f"{self.base_url}/api/v1"
        self.session = requests.Session()
        
    def test_instance_metrics(self) -> bool:
        """Test the /api/v1/instance/metrics endpoint"""
        print("\n🧪 Testing instance metrics endpoint...")
        
        try:
            response = self.session.get(f"{self.api_v1_url}/instance/metrics")
            
            if response.status_code != 200:
                print(f"❌ Expected 200, got {response.status_code}")
                return False
            
            data = response.json()
            print(f"✅ Endpoint returned 200 OK")
            
            # Check response structure
            expected_keys = ['current', 'system']
            missing_keys = [key for key in expected_keys if key not in data]
            
            if missing_keys:
                print(f"⚠️  Missing expected keys: {missing_keys}")
            else:
                print("✅ All expected keys present")
            
            # Display current metrics
            if 'current' in data:
                current = data['current']
                print(f"\n📊 Current Metrics:")
                print(f"  Active users: {current.get('active_users', 'N/A')}")
                print(f"  Requests/min: {current.get('requests_per_minute', 'N/A')}")
                print(f"  Avg latency: {current.get('avg_latency_ms', 'N/A')}ms")
                print(f"  Timestamp: {current.get('timestamp', 'N/A')}")
            
            # Display system info
            if 'system' in data:
                system = data['system']
                print(f"\n💻 System Info:")
                print(f"  Version: {system.get('version', 'N/A')}")
                print(f"  Region: {system.get('region', 'N/A')}")
            
            return True
            
        except Exception as e:
            print(f"❌ Error testing instance metrics: {e}")
            return False
    
    def test_daily_aggregates(self) -> bool:
        """Test the /api/v1/instance/metrics/daily endpoint"""
        print("\n🧪 Testing daily aggregates endpoint...")
        
        try:
            # Test with default (7 days)
            response = self.session.get(f"{self.api_v1_url}/instance/metrics/daily")
            
            if response.status_code != 200:
                print(f"❌ Expected 200, got {response.status_code}")
                return False
            
            data = response.json()
            print(f"✅ Endpoint returned 200 OK")
            
            # Check response structure
            expected_keys = ['period', 'daily_aggregates']
            missing_keys = [key for key in expected_keys if key not in data]
            
            if missing_keys:
                print(f"⚠️  Missing expected keys: {missing_keys}")
            else:
                print("✅ All expected keys present")
            
            # Display period info
            if 'period' in data:
                period = data['period']
                print(f"\n📅 Period:")
                print(f"  Start: {period.get('start', 'N/A')}")
                print(f"  End: {period.get('end', 'N/A')}")
                print(f"  Days: {period.get('days', 'N/A')}")
            
            # Display aggregate summary
            if 'daily_aggregates' in data:
                aggregates = data['daily_aggregates']
                print(f"\n📊 Daily Aggregates: {len(aggregates)} days")
                
                if aggregates and len(aggregates) > 0:
                    # Show first day
                    first = aggregates[0]
                    print(f"\n  Latest day ({first.get('date', 'N/A')}):")
                    if 'metrics' in first:
                        metrics = first['metrics']
                        print(f"    Requests: {metrics.get('total_requests', 'N/A')}")
                        print(f"    Users: {metrics.get('unique_users', 'N/A')}")
                        print(f"    Cost: ${metrics.get('cost_cents', 'N/A'):.4f}")
            
            # Test with custom days parameter
            print("\n🧪 Testing with custom days parameter (14)...")
            response = self.session.get(f"{self.api_v1_url}/instance/metrics/daily?days=14")
            
            if response.status_code == 200:
                data = response.json()
                if 'period' in data and data['period'].get('days') == 14:
                    print("✅ Custom days parameter works")
                else:
                    print("⚠️  Custom days parameter not reflected in response")
            
            return True
            
        except Exception as e:
            print(f"❌ Error testing daily aggregates: {e}")
            return False
    
    def test_predictive_analytics(self) -> bool:
        """Test the /api/v1/instance/analytics endpoint"""
        print("\n🧪 Testing predictive analytics endpoint...")
        
        try:
            response = self.session.get(f"{self.api_v1_url}/instance/analytics")
            
            if response.status_code != 200:
                print(f"❌ Expected 200, got {response.status_code}")
                return False
            
            data = response.json()
            print(f"✅ Endpoint returned 200 OK")
            
            # Check response structure
            expected_keys = ['projections', 'recommendations', 'generated_at']
            missing_keys = [key for key in expected_keys if key not in data]
            
            if missing_keys:
                print(f"⚠️  Missing expected keys: {missing_keys}")
            else:
                print("✅ All expected keys present")
            
            # Display projections
            if 'projections' in data:
                proj = data['projections']
                print(f"\n📈 Projections:")
                
                # Monthly cost
                if 'monthly_cost' in proj:
                    cost = proj['monthly_cost']
                    print(f"\n  💰 Monthly Cost:")
                    print(f"    Current: ${cost.get('current_month', 0):.2f}")
                    print(f"    Next month: ${cost.get('next_month', 0):.2f}")
                    print(f"    3 months: ${cost.get('three_months', 0):.2f}")
                    print(f"    Confidence: {cost.get('confidence_level', 0)*100:.0f}%")
                
                # Storage growth
                if 'storage_growth' in proj:
                    storage = proj['storage_growth']
                    print(f"\n  💾 Storage Growth:")
                    print(f"    Monthly rate: {storage.get('monthly_rate_percent', 0):.1f}%")
                    print(f"    30 days: {storage.get('projected_gb_30_days', 0):.1f} GB")
                    print(f"    90 days: {storage.get('projected_gb_90_days', 0):.1f} GB")
                
                # User growth
                if 'user_growth' in proj:
                    users = proj['user_growth']
                    print(f"\n  👥 User Growth:")
                    print(f"    Monthly rate: {users.get('monthly_rate_percent', 0):.1f}%")
                    print(f"    30 days: {users.get('projected_mau_30_days', 0)} MAU")
                    print(f"    90 days: {users.get('projected_mau_90_days', 0)} MAU")
            
            # Display recommendations
            if 'recommendations' in data and data['recommendations']:
                print(f"\n💡 Recommendations:")
                for i, rec in enumerate(data['recommendations'], 1):
                    print(f"\n  {i}. {rec.get('type', 'N/A').title()} ({rec.get('priority', 'N/A')})")
                    print(f"     {rec.get('description', 'N/A')}")
                    if 'potential_savings_percent' in rec:
                        print(f"     Potential savings: {rec['potential_savings_percent']}%")
                    if 'cost_impact_percent' in rec:
                        print(f"     Cost impact: +{rec['cost_impact_percent']}%")
            
            return True
            
        except Exception as e:
            print(f"❌ Error testing predictive analytics: {e}")
            return False
    
    def test_cost_headers_on_metrics(self) -> bool:
        """Test that cost headers are present on metrics endpoints"""
        print("\n🧪 Testing cost headers on metrics endpoints...")
        
        endpoints = [
            "/instance/metrics",
            "/instance/metrics/daily",
            "/instance/analytics"
        ]
        
        all_have_headers = True
        
        for endpoint in endpoints:
            response = self.session.get(f"{self.api_v1_url}{endpoint}")
            
            if 'X-Cost-Total-Microcents' in response.headers:
                cost_cents = response.headers.get('X-Cost-Total-Cents', 'N/A')
                print(f"✅ {endpoint}: Cost tracked ({cost_cents} cents)")
            else:
                print(f"❌ {endpoint}: No cost headers")
                all_have_headers = False
        
        return all_have_headers
    
    def run_all_tests(self):
        """Run all enhanced metrics tests"""
        print(f"🚀 Starting Lesser 2.0 Enhanced Metrics Tests")
        print(f"📍 Testing against: {self.base_url}")
        print("=" * 60)
        
        tests = [
            ("Instance Metrics", self.test_instance_metrics),
            ("Daily Aggregates", self.test_daily_aggregates),
            ("Predictive Analytics", self.test_predictive_analytics),
            ("Cost Headers on Metrics", self.test_cost_headers_on_metrics),
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
            print("🎉 All tests passed! Enhanced metrics are working!")
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
    
    tester = EnhancedMetricsTester(base_url)
    return tester.run_all_tests()


if __name__ == "__main__":
    sys.exit(main()) 