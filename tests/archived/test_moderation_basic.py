#!/usr/bin/env python3
"""
Basic test script for the Reactive Moderation Mesh
Demonstrates the flow of flagging content, reviewing it, and reaching consensus
"""

BASE_URL = "https://your-instance.com"  # Replace with your instance URL

def test_moderation_flow():
    """Test the basic moderation flow"""
    
    print("=== Lesser Reactive Moderation Mesh Demo ===\n")
    
    # Note: In a real implementation, you would:
    # 1. Authenticate users and get tokens
    # 2. Create actual content to moderate
    # 3. Have multiple moderators with different trust scores
    
    # Step 1: Flag content
    print("1. Flagging content for moderation...")
    flag_data = {
        "object_id": "https://instance.com/users/testuser/statuses/12345",
        "object_type": "status",
        "category": "hate_speech",
        "severity": 3,
        "reason": "This post contains harmful language targeting a protected group",
        "evidence": [
            {
                "type": "keyword_match",
                "score": 0.95,
                "description": "Matched harmful keyword patterns"
            }
        ]
    }
    print(f"   Flagging object: {flag_data['object_id']}")
    print(f"   Category: {flag_data['category']}, Severity: {flag_data['severity']}")
    
    # Would make API call:
    # response = requests.post(f"{BASE_URL}/api/v1/moderation/flag", json=flag_data, headers=auth_headers)
    
    event_id = "evt_123456789"  # Would come from response
    print(f"   ✓ Created moderation event: {event_id}\n")
    
    # Step 2: Moderators review the content
    print("2. Moderators reviewing the flagged content...")
    
    # Simulate 3 moderators with different trust scores
    moderators = [
        {"id": "mod1", "trust_score": 0.9, "action": "remove", "confidence": 0.95},
        {"id": "mod2", "trust_score": 0.7, "action": "remove", "confidence": 0.8},
        {"id": "mod3", "trust_score": 0.5, "action": "warning", "confidence": 0.6},
    ]
    
    for mod in moderators:
        review_data = {
            "event_id": event_id,
            "action": mod["action"],
            "confidence": mod["confidence"],
            "notes": f"Reviewed by {mod['id']}"
        }
        
        print(f"   Moderator {mod['id']} (trust: {mod['trust_score']}):")
        print(f"     - Action: {mod['action']}")
        print(f"     - Confidence: {mod['confidence']}")
        print(f"     - Weight: {mod['trust_score'] * mod['confidence']:.2f}")
        
        # Would make API call:
        # response = requests.post(f"{BASE_URL}/api/v1/moderation/review", json=review_data, headers=mod_headers)
    
    print()
    
    # Step 3: Consensus calculation (happens automatically in Lambda)
    print("3. Calculating consensus...")
    
    # Calculate weighted consensus
    total_weight = sum(m['trust_score'] * m['confidence'] for m in moderators)
    remove_weight = sum(m['trust_score'] * m['confidence'] for m in moderators if m['action'] == 'remove')
    warning_weight = sum(m['trust_score'] * m['confidence'] for m in moderators if m['action'] == 'warning')
    
    remove_consensus = remove_weight / total_weight
    warning_consensus = warning_weight / total_weight
    
    print(f"   Total trust weight: {total_weight:.2f}")
    print(f"   'remove' support: {remove_consensus:.1%} ({remove_weight:.2f} weight)")
    print(f"   'warning' support: {warning_consensus:.1%} ({warning_weight:.2f} weight)")
    
    if remove_consensus > 0.7:  # Default consensus threshold
        print(f"   ✓ Consensus reached: REMOVE (confidence: {remove_consensus:.1%})\n")
        decision = "remove"
    else:
        print(f"   ✗ No consensus reached (highest: {max(remove_consensus, warning_consensus):.1%})\n")
        decision = None
    
    # Step 4: Trust score updates (happens automatically)
    if decision:
        print("4. Updating trust scores based on consensus...")
        for mod in moderators:
            if mod['action'] == decision:
                delta = 0.01 * remove_consensus
                print(f"   {mod['id']}: +{delta:.3f} (agreed with consensus)")
            else:
                delta = -0.005 * (1 - remove_consensus)
                print(f"   {mod['id']}: {delta:.3f} (disagreed with consensus)")
    
    print("\n=== Demo Complete ===")
    print("\nKey Points:")
    print("- Trust scores weight reviews (not just vote counting)")
    print("- Consensus requires sufficient agreement (70% default)")
    print("- Trust evolves based on alignment with consensus")
    print("- All decisions are transparent and auditable")

if __name__ == "__main__":
    test_moderation_flow() 
