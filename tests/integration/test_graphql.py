#!/usr/bin/env python3
"""
Test script for Lesser GraphQL API
"""

import requests
import json
import os

# Get the API endpoint from environment or use default
GRAPHQL_ENDPOINT = os.getenv("GRAPHQL_ENDPOINT", "http://localhost:3000/graphql")

def test_instance_metrics():
    """Test the instanceMetrics query"""
    query = """
    query GetInstanceMetrics {
        instanceMetrics {
            activeUsers
            requestsPerMinute
            averageLatencyMs
            storageUsedGb
            estimatedMonthlyCost
            lastUpdated
        }
    }
    """
    
    response = requests.post(GRAPHQL_ENDPOINT, json={"query": query})
    print("\n=== Instance Metrics Query ===")
    print(f"Status: {response.status_code}")
    print(f"Headers: {dict(response.headers)}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")
    
    # Check for cost headers
    if "X-Cost-Total-Micros" in response.headers:
        print(f"\nCost Information:")
        print(f"  Total Cost: {response.headers.get('X-Cost-Total-Micros')} microcents")
        print(f"  DynamoDB Reads: {response.headers.get('X-Cost-DynamoDB-Reads', '0')}")
        print(f"  DynamoDB Writes: {response.headers.get('X-Cost-DynamoDB-Writes', '0')}")

def test_actor_query():
    """Test the actor query"""
    query = """
    query GetActor($username: String) {
        actor(username: $username) {
            id
            username
            domain
            displayName
            summary
            followers
            following
            statusesCount
            bot
            locked
            createdAt
            updatedAt
        }
    }
    """
    
    variables = {
        "username": "testuser"
    }
    
    response = requests.post(GRAPHQL_ENDPOINT, json={"query": query, "variables": variables})
    print("\n=== Actor Query ===")
    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")

def test_introspection():
    """Test GraphQL introspection to see available queries"""
    query = """
    query IntrospectionQuery {
        __schema {
            queryType {
                fields {
                    name
                    description
                    args {
                        name
                        type {
                            name
                            kind
                        }
                    }
                }
            }
        }
    }
    """
    
    response = requests.post(GRAPHQL_ENDPOINT, json={"query": query})
    print("\n=== Schema Introspection ===")
    print(f"Status: {response.status_code}")
    
    if response.status_code == 200:
        data = response.json()
        if "data" in data and data["data"]["__schema"]:
            print("\nAvailable Queries:")
            for field in data["data"]["__schema"]["queryType"]["fields"]:
                args = ", ".join([f"{arg['name']}: {arg['type']['name'] or arg['type']['kind']}" 
                                for arg in field["args"]])
                print(f"  - {field['name']}({args})")

def test_graphql_error_handling():
    """Test GraphQL error handling"""
    # Invalid query
    query = """
    query InvalidQuery {
        nonExistentField
    }
    """
    
    response = requests.post(GRAPHQL_ENDPOINT, json={"query": query})
    print("\n=== Error Handling Test ===")
    print(f"Status: {response.status_code}")
    print(f"Response: {json.dumps(response.json(), indent=2)}")

if __name__ == "__main__":
    print(f"Testing GraphQL API at: {GRAPHQL_ENDPOINT}")
    
    try:
        # Test basic connectivity first
        test_introspection()
        
        # Test specific queries
        test_instance_metrics()
        test_actor_query()
        test_graphql_error_handling()
        
    except requests.exceptions.ConnectionError:
        print(f"\nError: Could not connect to GraphQL endpoint at {GRAPHQL_ENDPOINT}")
        print("Make sure the Lambda is running or deployed")
    except Exception as e:
        print(f"\nUnexpected error: {e}") 