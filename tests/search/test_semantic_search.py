#!/usr/bin/env python3
"""
Test script for AI-enhanced semantic search functionality.
Tests AWS Bedrock embeddings, Comprehend query analysis, and semantic search.
"""

import requests
import time
import argparse
from typing import Dict, List


class SemanticSearchTester:
    def __init__(self, base_url: str, token: str):
        self.base_url = base_url.rstrip('/')
        self.token = token
        self.headers = {
            'Authorization': f'Bearer {token}',
            'Content-Type': 'application/json'
        }

    def test_semantic_search(self, query: str, semantic: bool = True) -> Dict:
        """Test semantic search functionality"""
        print(f"\n🔍 Testing semantic search: '{query}' (semantic={semantic})")
        
        url = f"{self.base_url}/api/v2/search"
        params = {
            'q': query,
            'type': 'accounts',
            'limit': 10,
            'semantic': str(semantic).lower()
        }
        
        start_time = time.time()
        response = requests.get(url, headers=self.headers, params=params)
        search_time = (time.time() - start_time) * 1000
        
        if response.status_code != 200:
            print(f"❌ Search failed: {response.status_code}")
            print(f"Response: {response.text}")
            return {}
        
        data = response.json()
        accounts = data.get('accounts', [])
        
        print(f"✅ Found {len(accounts)} results in {search_time:.0f}ms")
        
        # Display results
        for i, account in enumerate(accounts[:5]):
            print(f"\n  {i+1}. @{account['username']} - {account.get('display_name', '')}")
            if account.get('note'):
                bio = account['note'].replace('<p>', '').replace('</p>', '').strip()[:100]
                print(f"     Bio: {bio}...")
            
        return {
            'results': accounts,
            'search_time': search_time,
            'count': len(accounts)
        }

    def test_similar_queries(self, queries: List[str]):
        """Test semantic similarity between related queries"""
        print("\n🧠 Testing semantic similarity between queries:")
        
        results_by_query = {}
        
        for query in queries:
            result = self.test_semantic_search(query, semantic=True)
            results_by_query[query] = [acc['username'] for acc in result.get('results', [])]
        
        # Compare results
        print("\n📊 Similarity analysis:")
        for i, query1 in enumerate(queries):
            for j, query2 in enumerate(queries[i+1:], i+1):
                set1 = set(results_by_query.get(query1, []))
                set2 = set(results_by_query.get(query2, []))
                
                overlap = set1.intersection(set2)
                similarity = len(overlap) / max(len(set1), len(set2), 1) * 100
                
                print(f"\n  '{query1}' vs '{query2}':")
                print(f"  Similarity: {similarity:.1f}%")
                print(f"  Common results: {overlap if overlap else 'None'}")

    def test_typo_tolerance(self, correct_query: str, typo_queries: List[str]):
        """Test if semantic search handles typos better than regular search"""
        print(f"\n🔤 Testing typo tolerance for '{correct_query}':")
        
        # Test correct query
        correct_result = self.test_semantic_search(correct_query, semantic=True)
        correct_users = {acc['username'] for acc in correct_result.get('results', [])}
        
        # Test typo queries
        for typo in typo_queries:
            print(f"\n  Testing typo: '{typo}'")
            
            # Regular search
            regular = self.test_semantic_search(typo, semantic=False)
            regular_users = {acc['username'] for acc in regular.get('results', [])}
            
            # Semantic search
            semantic = self.test_semantic_search(typo, semantic=True)
            semantic_users = {acc['username'] for acc in semantic.get('results', [])}
            
            # Compare results
            regular_match = len(regular_users.intersection(correct_users))
            semantic_match = len(semantic_users.intersection(correct_users))
            
            print(f"  Regular search found {regular_match}/{len(correct_users)} correct results")
            print(f"  Semantic search found {semantic_match}/{len(correct_users)} correct results")
            
            if semantic_match > regular_match:
                print("  ✅ Semantic search performed better!")
            elif semantic_match == regular_match:
                print("  ➖ Both performed equally")
            else:
                print("  ❌ Regular search performed better")

    def test_contextual_search(self, context_queries: List[Dict[str, str]]):
        """Test if semantic search understands context better"""
        print("\n🎯 Testing contextual understanding:")
        
        for test in context_queries:
            query = test['query']
            expected_types = test.get('expected_types', [])
            
            print(f"\n  Query: '{query}'")
            print(f"  Expected to find: {', '.join(expected_types)}")
            
            result = self.test_semantic_search(query, semantic=True)
            
            # Analyze results based on bio content
            found_types = []
            for acc in result.get('results', [])[:5]:
                bio = acc.get('note', '').lower()
                for expected in expected_types:
                    if expected.lower() in bio:
                        found_types.append(expected)
                        break
            
            success_rate = len(found_types) / len(expected_types) * 100 if expected_types else 0
            print(f"  Found {len(found_types)}/{len(expected_types)} expected types ({success_rate:.0f}%)")

    def benchmark_performance(self):
        """Compare performance of regular vs semantic search"""
        print("\n⚡ Performance benchmark:")
        
        test_queries = [
            "developer",
            "artist photographer",
            "open source contributor",
            "machine learning engineer",
            "content creator"
        ]
        
        regular_times = []
        semantic_times = []
        
        for query in test_queries:
            # Regular search
            start = time.time()
            requests.get(f"{self.base_url}/api/v2/search", 
                        headers=self.headers, 
                        params={'q': query, 'type': 'accounts', 'semantic': 'false'})
            regular_times.append((time.time() - start) * 1000)
            
            # Semantic search
            start = time.time()
            requests.get(f"{self.base_url}/api/v2/search", 
                        headers=self.headers, 
                        params={'q': query, 'type': 'accounts', 'semantic': 'true'})
            semantic_times.append((time.time() - start) * 1000)
            
            time.sleep(0.1)  # Avoid rate limiting
        
        avg_regular = sum(regular_times) / len(regular_times)
        avg_semantic = sum(semantic_times) / len(semantic_times)
        
        print(f"\n  Regular search average: {avg_regular:.0f}ms")
        print(f"  Semantic search average: {avg_semantic:.0f}ms")
        print(f"  Overhead: {avg_semantic - avg_regular:.0f}ms ({(avg_semantic/avg_regular - 1)*100:.0f}%)")


def main():
    parser = argparse.ArgumentParser(description='Test semantic search functionality')
    parser.add_argument('base_url', help='Base URL of the instance (e.g., https://your-instance.com)')
    parser.add_argument('--token', required=True, help='OAuth token for authentication')
    parser.add_argument('--test', choices=['all', 'basic', 'similarity', 'typo', 'context', 'performance'],
                       default='all', help='Which tests to run')
    
    args = parser.parse_args()
    
    tester = SemanticSearchTester(args.base_url, args.token)
    
    print("🚀 Starting semantic search tests...")
    
    if args.test in ['all', 'basic']:
        # Basic semantic search test
        tester.test_semantic_search("software developer", semantic=True)
        tester.test_semantic_search("software developer", semantic=False)
    
    if args.test in ['all', 'similarity']:
        # Test semantic similarity
        similar_queries = [
            "software engineer",
            "programmer",
            "developer",
            "coder",
            "tech professional"
        ]
        tester.test_similar_queries(similar_queries)
    
    if args.test in ['all', 'typo']:
        # Test typo tolerance
        tester.test_typo_tolerance(
            "photographer",
            ["photografer", "fotografer", "photgrapher", "potographer"]
        )
    
    if args.test in ['all', 'context']:
        # Test contextual understanding
        context_tests = [
            {
                'query': 'people who love coffee',
                'expected_types': ['coffee', 'caffeine', 'barista', 'espresso']
            },
            {
                'query': 'open source contributors',
                'expected_types': ['github', 'open source', 'oss', 'contributor']
            },
            {
                'query': 'creative professionals',
                'expected_types': ['artist', 'designer', 'creative', 'art']
            }
        ]
        tester.test_contextual_search(context_tests)
    
    if args.test in ['all', 'performance']:
        # Performance benchmark
        tester.benchmark_performance()
    
    print("\n✅ All tests completed!")


if __name__ == '__main__':
    main() 
