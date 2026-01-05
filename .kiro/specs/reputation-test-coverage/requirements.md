# Requirements Document

## Introduction

This document defines the requirements for improving unit test coverage in the `pkg/reputation` package from the current 49.1% to approximately 90%. The reputation package provides actor reputation calculation algorithms based on activity history and trust metrics for the Lesser ActivityPub social network.

## Glossary

- **Reputation_Calculator**: Component that computes reputation scores for actors based on trust, activity, moderation, and community metrics
- **Vouch_Manager**: Component that handles vouch creation, revocation, and import operations
- **Signer**: Component that handles cryptographic signing of reputation documents using Ed25519
- **Verifier**: Component that verifies reputation document signatures and issuer trust
- **Portable_Reputation**: Exportable reputation document that can be transferred between instances
- **Trust_Score**: Score (0-250) based on trust graph relationships
- **Activity_Score**: Score (0-250) based on posting frequency, account age, and engagement
- **Moderation_Score**: Score (0-250) based on moderation history and violations
- **Community_Score**: Score (0-250) based on vouches and community contributions

## Requirements

### Requirement 1: Calculator Test Coverage

**User Story:** As a developer, I want comprehensive tests for the reputation calculator, so that I can ensure reputation scores are calculated correctly.

#### Acceptance Criteria

1. WHEN NewCalculator is called with valid parameters THEN the Calculator SHALL be created with the provided store, instanceURL, and logger
2. WHEN Calculate is called with valid input THEN the Calculator SHALL return a Reputation with all score components populated
3. WHEN calculateTrustScore is called with empty trust relationships THEN the Calculator SHALL return 0
4. WHEN calculateTrustScore is called with trust relationships THEN the Calculator SHALL weight by recency and return a score between 0-250
5. WHEN calculateActivityScore is called THEN the Calculator SHALL compute score based on account age, post frequency, followers, and recency
6. WHEN calculateModerationScore is called with no violations THEN the Calculator SHALL return 250 (full score)
7. WHEN calculateModerationScore is called with upheld reports THEN the Calculator SHALL deduct points based on severity
8. WHEN calculateModerationScore is called with suspensions THEN the Calculator SHALL deduct 100 points per suspension
9. WHEN calculateCommunityScore is called THEN the Calculator SHALL compute score based on vouches received, vouches given, community notes, and helpful votes
10. WHEN GetBoostFromVouches is called with active vouches THEN the Calculator SHALL return a boost capped at 200 points

### Requirement 2: VouchManager Test Coverage

**User Story:** As a developer, I want comprehensive tests for the vouch manager, so that I can ensure vouch operations work correctly.

#### Acceptance Criteria

1. WHEN NewVouchManager is called THEN the VouchManager SHALL be created with the provided dependencies
2. WHEN CreateVouch is called with invalid confidence (outside 0-1) THEN the VouchManager SHALL return an error
3. WHEN CreateVouch is called when monthly limit is reached THEN the VouchManager SHALL return an error
4. WHEN CreateVouch is called with insufficient voucher reputation (below 500) THEN the VouchManager SHALL return an error
5. WHEN CreateVouch is called with valid parameters THEN the VouchManager SHALL create, sign, and store the vouch
6. WHEN RevokeVouch is called by the voucher THEN the VouchManager SHALL update the vouch status to revoked
7. WHEN RevokeVouch is called by a non-voucher THEN the VouchManager SHALL return an error
8. WHEN GetVouchByID is called with existing ID THEN the VouchManager SHALL return the vouch
9. WHEN GetVouchByID is called with non-existing ID THEN the VouchManager SHALL return an error
10. WHEN GetVouchesForActor is called THEN the VouchManager SHALL return active vouches for the actor
11. WHEN GetVouchesFromActor is called THEN the VouchManager SHALL return all vouches created by the actor
12. WHEN ImportVouch is called with nil vouch THEN the VouchManager SHALL return an error
13. WHEN ImportVouch is called with inactive or revoked vouch THEN the VouchManager SHALL return an error
14. WHEN ImportVouch is called with expired vouch THEN the VouchManager SHALL return an error
15. WHEN ImportVouch is called with invalid signature THEN the VouchManager SHALL return an error
16. WHEN ImportVouch is called with insufficient voucher reputation THEN the VouchManager SHALL return an error
17. WHEN ImportVouch is called with valid vouch THEN the VouchManager SHALL store the vouch
18. WHEN ImportVouches is called with multiple vouches THEN the VouchManager SHALL import valid vouches and skip invalid ones

### Requirement 3: Crypto Verifier Test Coverage

**User Story:** As a developer, I want comprehensive tests for the crypto verifier, so that I can ensure signature verification works correctly.

#### Acceptance Criteria

1. WHEN VerifyPortableReputation is called with expired document THEN the Verifier SHALL return invalid with NotExpired=false
2. WHEN VerifyPortableReputation is called with invalid issuer proof THEN the Verifier SHALL return invalid with SignatureValid=false
3. WHEN VerifyPortableReputation is called with untrusted issuer THEN the Verifier SHALL return invalid with IssuerTrusted=false
4. WHEN VerifyPortableReputation is called with valid document THEN the Verifier SHALL return valid=true
5. WHEN getInstancePublicKey is called with cached key THEN the Verifier SHALL return the cached key
6. WHEN getInstancePublicKey is called with uncached key THEN the Verifier SHALL fetch from .well-known endpoint
7. WHEN isInstanceTrusted is called with blocked domain THEN the Verifier SHALL return false
8. WHEN isInstanceTrusted is called with allowed domain in allow-list mode THEN the Verifier SHALL return true
9. WHEN isInstanceTrusted is called with unblocked domain in open federation THEN the Verifier SHALL return true
10. WHEN VerifyVouchSignature is called with valid signature THEN the Verifier SHALL return true
11. WHEN VerifyVouchSignature is called with invalid signature THEN the Verifier SHALL return false
12. WHEN GetPublicKeyBase64 is called on Signer THEN the Signer SHALL return the base64-encoded public key

### Requirement 4: Service NewService Test Coverage

**User Story:** As a developer, I want comprehensive tests for service initialization, so that I can ensure the service is created correctly.

#### Acceptance Criteria

1. WHEN NewService is called with nil storage THEN the Service SHALL return an error
2. WHEN NewService is called with nil logger THEN the Service SHALL use a no-op logger
3. WHEN NewService is called with invalid private key THEN the Service SHALL return an error
4. WHEN NewService is called with valid config THEN the Service SHALL create all components and return the service

### Requirement 5: Edge Case and Error Path Coverage

**User Story:** As a developer, I want tests for edge cases and error paths, so that I can ensure the system handles failures gracefully.

#### Acceptance Criteria

1. WHEN parseSeverity is called with severity "3" THEN the Service SHALL return 3
2. WHEN addModerationEvents encounters an error THEN the Service SHALL log and continue with empty events
3. WHEN addReportEvents encounters an error THEN the Service SHALL log and continue with empty reports
4. WHEN ImportReputation fails to store reputation THEN the Service SHALL return failure result
