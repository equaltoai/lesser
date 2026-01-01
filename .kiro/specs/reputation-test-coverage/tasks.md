# Implementation Plan: Reputation Package Test Coverage

## Overview

This plan implements comprehensive unit tests for the `pkg/reputation` package to achieve ~90% test coverage. Tests are organized by component with property-based tests for critical invariants.

## Tasks

- [x] 1. Create Calculator Tests
  - [x] 1.1 Create calculator_test.go with test infrastructure
    - Create mock storage implementation for calculator tests
    - Set up test logger and instance URL
    - _Requirements: 1.1_

  - [x] 1.2 Test NewCalculator constructor
    - Verify calculator is created with correct fields
    - _Requirements: 1.1_

  - [x] 1.3 Test Calculate function
    - Test with valid input returns complete Reputation
    - Verify all score components are populated
    - Verify logging occurs
    - _Requirements: 1.2_

  - [x] 1.4 Test calculateTrustScore function
    - Test empty trust relationships returns 0
    - Test with incoming trust relationships
    - Test recency weighting (recent vs old relationships)
    - Test diversity bonus calculation
    - Test score capping at 250
    - _Requirements: 1.3, 1.4_

  - [x] 1.5 Test calculateActivityScore function
    - Test account age scoring (up to 50 points)
    - Test post frequency scoring (ideal 1-5 posts/day)
    - Test over-posting penalty
    - Test follower count scoring (logarithmic)
    - Test recency bonus based on last activity
    - Test score capping at 250
    - _Requirements: 1.5_

  - [x] 1.6 Test calculateModerationScore function
    - Test no violations returns 250
    - Test upheld reports deduct based on severity
    - Test dismissed reports add bonus
    - Test suspensions deduct 100 points
    - Test new account penalty for violations
    - Test score bounds (0-250)
    - _Requirements: 1.6, 1.7, 1.8_

  - [x] 1.7 Test calculateCommunityScore function
    - Test vouches received scoring
    - Test vouches given scoring
    - Test community notes scoring
    - Test helpful votes scoring
    - Test score capping at 250
    - _Requirements: 1.9_

  - [x] 1.8 Test GetBoostFromVouches function
    - Test with no vouches returns 0
    - Test with active vouches calculates boost
    - Test inactive/revoked/expired vouches are skipped
    - Test boost capping at 200
    - _Requirements: 1.10_

  - [x] 1.9 Write property test for score bounds
    - **Property 1: Score Bounds Invariant**
    - **Validates: Requirements 1.4, 1.5, 1.9**

  - [x] 1.10 Write property test for total score composition
    - **Property 2: Total Score Composition**
    - **Validates: Requirements 1.2**

  - [x] 1.11 Write property test for vouch boost cap
    - **Property 3: Vouch Boost Cap**
    - **Validates: Requirements 1.10**

- [x] 2. Checkpoint - Calculator Tests
  - Ensure all calculator tests pass, ask the user if questions arise.

- [x] 3. Create VouchManager Tests
  - [x] 3.1 Create vouch_test.go with test infrastructure
    - Create mock storage for vouch operations
    - Create mock signer for vouch signing
    - _Requirements: 2.1_

  - [x] 3.2 Test NewVouchManager constructor
    - Verify manager is created with correct fields
    - _Requirements: 2.1_

  - [x] 3.3 Test CreateVouch validation
    - Test invalid confidence (< 0 or > 1) returns error
    - Test monthly limit reached returns error
    - Test insufficient reputation (< 500) returns error
    - _Requirements: 2.2, 2.3, 2.4_

  - [x] 3.4 Test CreateVouch success path
    - Test valid parameters creates and stores vouch
    - Verify vouch fields are set correctly
    - Verify signature is applied
    - _Requirements: 2.5_

  - [x] 3.5 Test RevokeVouch function
    - Test voucher can revoke their vouch
    - Test non-voucher cannot revoke (returns error)
    - Test vouch not found returns error
    - _Requirements: 2.6, 2.7_

  - [x] 3.6 Test GetVouchByID function
    - Test existing ID returns vouch
    - Test non-existing ID returns error
    - _Requirements: 2.8, 2.9_

  - [x] 3.7 Test GetVouchesForActor and GetVouchesFromActor
    - Test returns correct vouches
    - Test empty results
    - Test error handling
    - _Requirements: 2.10, 2.11_

  - [x] 3.8 Test ImportVouch validation
    - Test nil vouch returns error
    - Test inactive vouch returns error
    - Test revoked vouch returns error
    - Test expired vouch returns error
    - Test invalid signature returns error
    - Test insufficient voucher reputation returns error
    - Test duplicate vouch is skipped
    - _Requirements: 2.12, 2.13, 2.14, 2.15, 2.16_

  - [x] 3.9 Test ImportVouch success path
    - Test valid vouch is stored
    - _Requirements: 2.17_

  - [x] 3.10 Test ImportVouches batch operation
    - Test imports valid vouches and skips invalid
    - Test returns correct count
    - _Requirements: 2.18_

  - [x] 3.11 Write property test for confidence validation
    - **Property 4: Confidence Validation**
    - **Validates: Requirements 2.2**

  - [x] 3.12 Write property test for reputation threshold
    - **Property 5: Reputation Threshold**
    - **Validates: Requirements 2.4**

- [x] 4. Checkpoint - VouchManager Tests
  - Ensure all vouch manager tests pass, ask the user if questions arise.

- [x] 5. Extend Crypto Tests
  - [x] 5.1 Test VerifyPortableReputation function
    - Test expired document returns NotExpired=false
    - Test invalid issuer proof returns SignatureValid=false
    - Test untrusted issuer returns IssuerTrusted=false
    - Test valid document returns Valid=true
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

  - [x] 5.2 Test getInstancePublicKey function
    - Test cached key is returned
    - Test uncached key fetches from .well-known endpoint
    - Test HTTP error handling
    - Test invalid response handling
    - _Requirements: 3.5, 3.6_

  - [x] 5.3 Test isInstanceTrusted function
    - Test blocked domain returns false
    - Test allowed domain in allow-list mode returns true
    - Test domain not in allow-list returns false
    - Test unblocked domain in open federation returns true
    - Test nil storage defaults to true
    - Test URL parsing error handling
    - _Requirements: 3.7, 3.8, 3.9_

  - [x] 5.4 Test VerifyVouchSignature function
    - Test valid signature returns true
    - Test invalid signature returns false
    - Test key fetch error handling
    - _Requirements: 3.10, 3.11_

  - [x] 5.5 Test GetPublicKeyBase64 on Signer
    - Test returns base64-encoded public key
    - _Requirements: 3.12_

- [x] 6. Checkpoint - Crypto Tests
  - Ensure all crypto tests pass, ask the user if questions arise.

- [x] 7. Extend Service Tests
  - [x] 7.1 Test NewService with valid config
    - Create mock storage with all required repositories
    - Test service is created with all components
    - _Requirements: 4.4_

  - [x] 7.2 Test NewService error paths
    - Test nil storage returns error (already covered)
    - Test invalid private key returns error
    - _Requirements: 4.1, 4.3_

  - [x] 7.3 Test remaining edge cases
    - Test parseSeverity with all values
    - Test addModerationEvents error path
    - Test ImportReputation store failure
    - _Requirements: 5.1, 5.2, 5.4_

  - [x] 7.4 Write property test for moderation score monotonicity
    - **Property 6: Moderation Score Monotonicity**
    - **Validates: Requirements 1.7**

- [x] 8. Final Checkpoint - All Tests
  - Run full test suite with coverage
  - Verify coverage is at or above 90%
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- All tasks including property-based tests are required
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Reuse existing mock patterns from `service_round20_coverage_test.go`
