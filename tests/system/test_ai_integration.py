#!/usr/bin/env python3
"""
Test script for AI Integration Layer (Phase 4.3)
Tests AI-powered content analysis features including:
- Text analysis (sentiment, toxicity, PII)
- Image analysis (NSFW, violence)
- AI-generated content detection
- Spam detection
- Moderation actions
"""

import requests
import json
import time
import os
from typing import Dict, Any
import argparse

# Configuration
BASE_URL = os.getenv("LESSER_API_URL", "https://api.lesser.example.com")
ACCESS_TOKEN = os.getenv("LESSER_ACCESS_TOKEN", "")

# Test content samples
TEST_CONTENT = {
    "toxic": {
        "text": "You are an absolute idiot and should die!",
        "expected": {
            "toxicity_score": 0.8,
            "moderation_action": "hide"
        }
    },
    "spam": {
        "text": "Buy now! Limited offer! Click here: bit.ly/scam FREE FREE FREE 100% guaranteed!",
        "expected": {
            "spam_score": 0.8,
            "moderation_action": "shadow_ban"
        }
    },
    "ai_generated": {
        "text": "As an AI language model, I cannot provide personal opinions, but I can offer information on this topic...",
        "expected": {
            "ai_generated_probability": 0.9,
            "moderation_action": "flag"
        }
    },
    "pii": {
        "text": "My SSN is 123-45-6789 and my email is john@example.com",
        "expected": {
            "contains_pii": True,
            "moderation_action": "flag"
        }
    },
    "normal": {
        "text": "Just had a great lunch at the new restaurant downtown. Highly recommend!",
        "expected": {
            "toxicity_score": 0.1,
            "spam_score": 0.1,
            "moderation_action": "none"
        }
    }
}

def create_headers(token: str = None) -> Dict[str, str]:
    """Create headers with authentication"""
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json"
    }
    if token:
        headers["Authorization"] = f"Bearer {token}"
    return headers

def create_test_post(content: str) -> Dict[str, Any]:
    """Create a test post"""
    url = f"{BASE_URL}/api/v1/statuses"
    data = {
        "status": content,
        "visibility": "public"
    }
    
    response = requests.post(url, json=data, headers=create_headers(ACCESS_TOKEN))
    if response.status_code == 200:
        return response.json()
    else:
        print(f"Failed to create post: {response.status_code} - {response.text}")
        return None

def get_ai_analysis(object_id: str) -> Dict[str, Any]:
    """Get AI analysis for an object"""
    url = f"{BASE_URL}/api/v1/ai/analysis/{object_id}"
    
    response = requests.get(url, headers=create_headers(ACCESS_TOKEN))
    if response.status_code == 200:
        return response.json()
    elif response.status_code == 404:
        return None
    else:
        print(f"Failed to get analysis: {response.status_code} - {response.text}")
        return None

def request_ai_analysis(object_id: str, force: bool = False) -> bool:
    """Request AI analysis for an object"""
    url = f"{BASE_URL}/api/v1/ai/analyze"
    data = {
        "object_id": object_id,
        "object_type": "Note",
        "force": force
    }
    
    response = requests.post(url, json=data, headers=create_headers(ACCESS_TOKEN))
    return response.status_code in [200, 202]

def wait_for_analysis(object_id: str, timeout: int = 30) -> Dict[str, Any]:
    """Wait for AI analysis to complete"""
    start_time = time.time()
    
    while time.time() - start_time < timeout:
        analysis = get_ai_analysis(object_id)
        if analysis:
            return analysis
        time.sleep(2)
    
    return None

def test_text_analysis():
    """Test text analysis features"""
    print("\n=== Testing Text Analysis ===")
    
    for test_name, test_data in TEST_CONTENT.items():
        print(f"\nTesting {test_name} content...")
        
        # Create post
        post = create_test_post(test_data["text"])
        if not post:
            print(f"❌ Failed to create {test_name} post")
            continue
        
        print(f"✅ Created post: {post['id']}")
        
        # Request analysis
        if not request_ai_analysis(post['id']):
            print(f"❌ Failed to request analysis for {test_name}")
            continue
        
        print("⏳ Waiting for analysis...")
        
        # Wait for analysis
        analysis = wait_for_analysis(post['id'])
        if not analysis:
            print(f"❌ Analysis timeout for {test_name}")
            continue
        
        print(f"✅ Analysis complete")
        
        # Verify results
        verify_analysis(test_name, test_data["expected"], analysis)

def verify_analysis(test_name: str, expected: Dict[str, Any], actual: Dict[str, Any]):
    """Verify analysis results match expectations"""
    print(f"\nVerifying {test_name} analysis:")
    
    # Check toxicity score
    if "toxicity_score" in expected:
        if actual.get("text_analysis", {}).get("toxicity_score", 0) >= expected["toxicity_score"]:
            print(f"✅ Toxicity score: {actual['text_analysis']['toxicity_score']:.2f}")
        else:
            print(f"❌ Toxicity score too low: {actual.get('text_analysis', {}).get('toxicity_score', 0):.2f}")
    
    # Check spam score
    if "spam_score" in expected:
        if actual.get("spam_analysis", {}).get("spam_score", 0) >= expected["spam_score"]:
            print(f"✅ Spam score: {actual['spam_analysis']['spam_score']:.2f}")
        else:
            print(f"❌ Spam score too low: {actual.get('spam_analysis', {}).get('spam_score', 0):.2f}")
    
    # Check AI detection
    if "ai_generated_probability" in expected:
        if actual.get("ai_detection", {}).get("ai_generated_probability", 0) >= expected["ai_generated_probability"]:
            print(f"✅ AI detection: {actual['ai_detection']['ai_generated_probability']:.2f}")
        else:
            print(f"❌ AI detection too low: {actual.get('ai_detection', {}).get('ai_generated_probability', 0):.2f}")
    
    # Check PII detection
    if "contains_pii" in expected:
        if actual.get("text_analysis", {}).get("contains_pii", False) == expected["contains_pii"]:
            print(f"✅ PII detection: {actual['text_analysis']['contains_pii']}")
        else:
            print(f"❌ PII detection mismatch")
    
    # Check moderation action
    if "moderation_action" in expected:
        if actual.get("moderation_action") == expected["moderation_action"]:
            print(f"✅ Moderation action: {actual['moderation_action']}")
        else:
            print(f"❌ Wrong moderation action: {actual.get('moderation_action')} (expected: {expected['moderation_action']})")
    
    # Show overall risk and confidence
    print(f"Overall risk: {actual.get('overall_risk', 0):.2f}")
    print(f"Confidence: {actual.get('confidence', 0):.2f}")

def test_ai_stats():
    """Test AI statistics endpoint"""
    print("\n=== Testing AI Statistics ===")
    
    periods = ["hour", "day", "week", "month"]
    
    for period in periods:
        url = f"{BASE_URL}/api/v1/ai/stats?period={period}"
        response = requests.get(url, headers=create_headers())
        
        if response.status_code == 200:
            stats = response.json()
            print(f"\n✅ Stats for {period}:")
            print(f"  Total analyses: {stats.get('total_analyses', 0)}")
            print(f"  Toxic content: {stats.get('toxic_content', 0)} ({stats.get('toxicity_rate', 0):.1%})")
            print(f"  Spam detected: {stats.get('spam_detected', 0)} ({stats.get('spam_rate', 0):.1%})")
            print(f"  AI generated: {stats.get('ai_generated', 0)} ({stats.get('ai_content_rate', 0):.1%})")
            print(f"  NSFW content: {stats.get('nsfw_content', 0)} ({stats.get('nsfw_rate', 0):.1%})")
            print(f"  PII detected: {stats.get('pii_detected', 0)}")
            
            if "moderation_actions" in stats:
                print("  Moderation actions:")
                for action, count in stats["moderation_actions"].items():
                    print(f"    {action}: {count}")
        else:
            print(f"❌ Failed to get stats for {period}: {response.status_code}")

def test_ai_capabilities():
    """Test AI capabilities endpoint"""
    print("\n=== Testing AI Capabilities ===")
    
    url = f"{BASE_URL}/api/v1/ai/capabilities"
    response = requests.get(url, headers=create_headers())
    
    if response.status_code == 200:
        capabilities = response.json()
        print("\n✅ AI Capabilities:")
        
        # Text analysis
        if "text_analysis" in capabilities:
            print("\nText Analysis:")
            for feature, enabled in capabilities["text_analysis"].items():
                status = "✅" if enabled else "❌"
                print(f"  {status} {feature}")
        
        # Image analysis
        if "image_analysis" in capabilities:
            print("\nImage Analysis:")
            for feature, enabled in capabilities["image_analysis"].items():
                status = "✅" if enabled else "❌"
                print(f"  {status} {feature}")
        
        # AI detection
        if "ai_detection" in capabilities:
            print("\nAI Detection:")
            for feature, enabled in capabilities["ai_detection"].items():
                status = "✅" if enabled else "❌"
                print(f"  {status} {feature}")
        
        # Moderation actions
        if "moderation_actions" in capabilities:
            print(f"\nModeration Actions: {', '.join(capabilities['moderation_actions'])}")
        
        # Cost information
        if "cost_per_analysis" in capabilities:
            print("\nCost per operation:")
            for operation, cost in capabilities["cost_per_analysis"].items():
                print(f"  {operation}: ${cost:.4f}")
    else:
        print(f"❌ Failed to get capabilities: {response.status_code}")

def test_image_analysis():
    """Test image analysis features (if available)"""
    print("\n=== Testing Image Analysis ===")
    
    # This would require actual image URLs
    # For now, just show a placeholder
    print("⚠️  Image analysis testing requires actual image URLs")
    print("   In production, this would test:")
    print("   - NSFW detection")
    print("   - Violence detection")
    print("   - Text extraction from images")
    print("   - Celebrity recognition")

def main():
    """Run all AI integration tests"""
    parser = argparse.ArgumentParser(description="Test AI Integration Layer")
    parser.add_argument("--base-url", default=BASE_URL, help="Base URL for the API")
    parser.add_argument("--token", default=ACCESS_TOKEN, help="Access token for authentication")
    parser.add_argument("--test", choices=["text", "stats", "capabilities", "image", "all"], 
                       default="all", help="Which tests to run")
    
    args = parser.parse_args()
    
    global BASE_URL, ACCESS_TOKEN
    BASE_URL = args.base_url
    ACCESS_TOKEN = args.token
    
    print(f"🤖 Testing AI Integration Layer")
    print(f"Base URL: {BASE_URL}")
    
    if not ACCESS_TOKEN:
        print("⚠️  No access token provided. Some tests may fail.")
        print("   Set LESSER_ACCESS_TOKEN environment variable or use --token")
    
    if args.test == "all" or args.test == "capabilities":
        test_ai_capabilities()
    
    if args.test == "all" or args.test == "text":
        test_text_analysis()
    
    if args.test == "all" or args.test == "stats":
        test_ai_stats()
    
    if args.test == "all" or args.test == "image":
        test_image_analysis()
    
    print("\n✅ AI integration tests complete!")

if __name__ == "__main__":
    main() 