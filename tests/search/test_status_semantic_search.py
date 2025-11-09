#!/usr/bin/env python3
"""
Test script for AI-powered status search functionality.
Tests semantic search, fuzzy search, and multi-strategy status search.
"""

import requests
import time
import argparse
from typing import Dict


class StatusSemanticSearchTester:
    def __init__(self, base_url: str, token: str):
        self.base_url = base_url.rstrip('/')
        self.token = token
        self.headers = {
            'Authorization': f'Bearer {token}',
            'Content-Type': 'application/json'
        }

    def test_status_search(self, query: str, filters: Dict = None) -> Dict:
        """Test status search functionality"""
        print(f"\n🔍 Testing status search: '{query}'")
        
        url = f"{self.base_url}/api/v2/search"
        params = {
            'q': query,
            'type': 'statuses',
            'limit': 20
        }
        
        # Add filters if provided
        if filters:
            params.update(filters)
        
        start_time = time.time()
        response = requests.get(url, headers=self.headers, params=params)
        search_time = (time.time() - start_time) * 1000
        
        if response.status_code != 200:
            print(f"❌ Search failed: {response.status_code}")
            print(f"Response: {response.text}")
            return {}
        
        data = response.json()
        statuses = data.get('statuses', [])
        
        print(f"✅ Found {len(statuses)} results in {search_time:.0f}ms")
        
        # Display results
        for i, status in enumerate(statuses[:5]):
            print(f"\n  {i+1}. Status by @{status.get('account', {}).get('username', 'unknown')}")
            content = status.get('content', '').replace('<p>', '').replace('</p>', '').strip()
            print(f"     Content: {content[:150]}...")
            print(f"     Created: {status.get('created_at', 'unknown')}")
            print(f"     Engagement: ❤️ {status.get('favourites_count', 0)} 🔁 {status.get('reblogs_count', 0)} 💬 {status.get('replies_count', 0)}")
            
            # Show any hashtags
            tags = status.get('tags', [])
            if tags:
                hashtags = [f"#{tag['name']}" for tag in tags]
                print(f"     Tags: {', '.join(hashtags)}")
        
        return {
            'results': statuses,
            'search_time': search_time,
            'count': len(statuses)
        }

    def test_semantic_similarity(self):
        """Test semantic search by posting related content"""
        print("\n=== Testing Semantic Similarity ===")
        
        # First, post a few test statuses with different phrasings
        test_posts = [
            "I love programming in Python, it's such a versatile language for data science and web development",
            "Python is my favorite coding language - great for machine learning and building APIs",
            "Just finished a cool ML project using Python and TensorFlow!",
            "Working on a web app with Django framework",
            "Data analysis is so much easier with pandas and numpy"
        ]
        
        posted_ids = []
        print("\n📝 Creating test posts...")
        
        for content in test_posts:
            response = requests.post(
                f"{self.base_url}/api/v1/statuses",
                headers=self.headers,
                json={'status': content}
            )
            if response.status_code == 200:
                status_id = response.json().get('id')
                posted_ids.append(status_id)
                print(f"✅ Posted: {content[:50]}...")
            else:
                print(f"❌ Failed to post: {content[:50]}...")
        
        # Wait for indexing
        print("\n⏳ Waiting for indexing...")
        time.sleep(5)
        
        # Test semantic searches
        semantic_queries = [
            "machine learning python",
            "web development frameworks",
            "data science tools",
            "programming languages"
        ]
        
        for query in semantic_queries:
            self.test_status_search(query)
        
        # Cleanup - delete test posts
        print("\n🧹 Cleaning up test posts...")
        for status_id in posted_ids:
            requests.delete(
                f"{self.base_url}/api/v1/statuses/{status_id}",
                headers=self.headers
            )

    def test_hashtag_search(self):
        """Test hashtag-based search"""
        print("\n=== Testing Hashtag Search ===")
        
        # Create posts with hashtags
        hashtag_posts = [
            "Working on #Python #MachineLearning project",
            "Love #Python for #DataScience work",
            "Building a #WebDev app with #Django #Python"
        ]
        
        posted_ids = []
        print("\n📝 Creating posts with hashtags...")
        
        for content in hashtag_posts:
            response = requests.post(
                f"{self.base_url}/api/v1/statuses",
                headers=self.headers,
                json={'status': content}
            )
            if response.status_code == 200:
                posted_ids.append(response.json().get('id'))
                print(f"✅ Posted: {content}")
        
        # Wait for indexing
        print("\n⏳ Waiting for indexing...")
        time.sleep(5)
        
        # Search by hashtags
        hashtag_queries = ["#Python", "#MachineLearning", "#DataScience", "Python MachineLearning"]
        
        for query in hashtag_queries:
            self.test_status_search(query)
        
        # Cleanup
        print("\n🧹 Cleaning up test posts...")
        for status_id in posted_ids:
            requests.delete(
                f"{self.base_url}/api/v1/statuses/{status_id}",
                headers=self.headers
            )

    def test_filtered_search(self):
        """Test search with various filters"""
        print("\n=== Testing Filtered Search ===")
        
        # Test with different filters
        filter_tests = [
            {
                'name': 'Local only search',
                'query': 'update',
                'filters': {'local': 'true'}
            },
            {
                'name': 'Media only search',
                'query': 'photo',
                'filters': {'has_media': 'true'}
            },
            {
                'name': 'Recent posts (last 24h)',
                'query': 'news',
                'filters': {'min_id': '0'}  # This would need proper implementation
            }
        ]
        
        for test in filter_tests:
            print(f"\n🔍 {test['name']}")
            self.test_status_search(test['query'], test['filters'])

    def test_engagement_based_search(self):
        """Test searching for popular/trending content"""
        print("\n=== Testing Engagement-Based Search ===")
        
        # Search for content with minimum engagement
        # Note: This would require the min_engagement parameter to be implemented
        queries = [
            "trending",
            "popular", 
            "viral"
        ]
        
        for query in queries:
            print(f"\n🔍 Searching for popular content: '{query}'")
            self.test_status_search(query)

    def test_fuzzy_search(self):
        """Test fuzzy/typo-tolerant search"""
        print("\n=== Testing Fuzzy Search ===")
        
        # Test with typos and misspellings
        typo_queries = [
            ("programing", "programming"),  # Single letter typo
            ("pythoon", "python"),          # Letter swap
            ("machne learning", "machine learning"),  # Missing letter
            ("develpoment", "development")   # Letter transposition
        ]
        
        for typo, correct in typo_queries:
            print(f"\n🔍 Testing typo tolerance: '{typo}' (should find '{correct}')")
            self.test_status_search(typo)

    def run_comprehensive_test(self):
        """Run all status search tests"""
        print("\n" + "="*60)
        print("COMPREHENSIVE STATUS SEARCH TEST SUITE")
        print("="*60)
        
        # Basic search test
        print("\n=== Testing Basic Status Search ===")
        self.test_status_search("test")
        
        # Test each search type
        self.test_semantic_similarity()
        self.test_hashtag_search()
        self.test_filtered_search()
        self.test_engagement_based_search()
        self.test_fuzzy_search()
        
        print("\n" + "="*60)
        print("✅ All tests completed!")
        print("="*60)


def main():
    parser = argparse.ArgumentParser(description='Test AI-powered status search functionality')
    parser.add_argument('base_url', help='Base URL of the API (e.g., https://your-instance.com)')
    parser.add_argument('--token', required=True, help='OAuth token for authentication')
    parser.add_argument('--test', choices=['all', 'semantic', 'hashtag', 'filters', 'engagement', 'fuzzy'],
                        default='all', help='Specific test to run')
    parser.add_argument('--query', help='Custom search query to test')
    
    args = parser.parse_args()
    
    tester = StatusSemanticSearchTester(args.base_url, args.token)
    
    if args.query:
        # Test custom query
        tester.test_status_search(args.query)
    elif args.test == 'all':
        tester.run_comprehensive_test()
    elif args.test == 'semantic':
        tester.test_semantic_similarity()
    elif args.test == 'hashtag':
        tester.test_hashtag_search()
    elif args.test == 'filters':
        tester.test_filtered_search()
    elif args.test == 'engagement':
        tester.test_engagement_based_search()
    elif args.test == 'fuzzy':
        tester.test_fuzzy_search()


if __name__ == '__main__':
    main() 
