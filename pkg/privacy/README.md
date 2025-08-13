# Privacy Hashing Package

This package provides cryptographically secure privacy-preserving hashing utilities for protecting sensitive user data while maintaining analytical capabilities.

## Features

- **HMAC-SHA256** based secure hashing with Argon2id key derivation
- **Multiple privacy levels**: none, partial, full
- **Context-aware hashing**: different data types use different derived keys
- **Configurable privacy policies** for different data types
- **Timing attack resistance** using constant-time operations
- **Environment variable configuration**
- **Preset configurations** for development, production, and compliance

## Quick Start

### 1. Generate a Master Key

```bash
# Generate a secure master key
go run -c "
package main
import (\"fmt\"; \"github.com/equaltoai/lesser/pkg/privacy\")
func main() {
    key, _ := privacy.GenerateMasterKeyBase64()
    fmt.Println(\"export PRIVACY_MASTER_KEY=\" + key)
}
"
```

### 2. Set Environment Variables

```bash
export PRIVACY_MASTER_KEY="your_generated_key_here"
export ENABLE_PRIVACY_HASHING="true"
export IP_PRIVACY_LEVEL="partial"
export EMAIL_PRIVACY_LEVEL="partial"
export USERNAME_PRIVACY_LEVEL="full"
```

### 3. Use in Your Application

```go
package main

import (
    "fmt"
    "github.com/equaltoai/lesser/pkg/privacy"
)

func main() {
    // Load from environment
    loader := privacy.NewConfigLoader("PRIVACY")
    hasher, err := loader.LoadHasherFromEnvironment()
    if err != nil {
        panic(err)
    }
    
    // Hash different types of data
    hashedIP, _ := hasher.HashIP("192.168.1.100")
    hashedEmail, _ := hasher.HashEmail("user@example.com")
    hashedUsername, _ := hasher.HashUsername("john_doe")
    
    fmt.Printf("IP: %s\n", hashedIP)
    fmt.Printf("Email: %s\n", hashedEmail)
    fmt.Printf("Username: %s\n", hashedUsername)
}
```

## Privacy Levels

### None
- Returns original data unchanged
- Use only in development or when privacy is not required

### Partial
- **IP Addresses**: Preserves network portion (first 2 octets for IPv4)
- **Email Addresses**: Preserves domain, hashes local part
- **Other Data**: Uses full hashing

### Full
- Complete cryptographic hashing using HMAC-SHA256
- No original data preserved
- Maximum privacy protection

## Configuration

### Environment Variables

```bash
# Required
PRIVACY_MASTER_KEY="base64_encoded_key"

# Privacy levels (none, partial, full)
PRIVACY_IP_PRIVACY_LEVEL="partial"        # Default: partial
PRIVACY_EMAIL_PRIVACY_LEVEL="partial"     # Default: partial
PRIVACY_USERNAME_PRIVACY_LEVEL="full"     # Default: full
PRIVACY_PII_PRIVACY_LEVEL="full"          # Default: full
PRIVACY_GENERIC_PRIVACY_LEVEL="full"      # Default: full

# Performance tuning
PRIVACY_ARGON2_MEMORY="65536"             # Memory in KB, default: 65536
PRIVACY_ARGON2_TIME="3"                   # Iterations, default: 3
PRIVACY_ARGON2_THREADS="4"                # Threads, default: 4
PRIVACY_ARGON2_KEY_LENGTH="32"            # Key length in bytes, default: 32
```

### Programmatic Configuration

```go
// Custom configuration
config := &privacy.HashingConfig{
    MasterKey: masterKeyBytes,
    IPPrivacyLevel: privacy.PrivacyLevelPartial,
    EmailPrivacyLevel: privacy.PrivacyLevelPartial,
    UsernamePrivacyLevel: privacy.PrivacyLevelFull,
    PIIPrivacyLevel: privacy.PrivacyLevelFull,
    GenericPrivacyLevel: privacy.PrivacyLevelFull,
    Argon2Memory: 64 * 1024,  // 64 MB
    Argon2Time: 3,            // 3 iterations
    Argon2Threads: 4,         // 4 threads
    Argon2KeyLen: 32,         // 32 bytes
}

hasher, err := privacy.NewPrivacyHasher(config)
```

### Preset Configurations

```go
pc := &privacy.PresetConfigurations{}

// Development (faster, less secure)
devConfig, _ := pc.GetDevelopmentConfig()

// Production (balanced security/performance)
prodConfig, _ := pc.GetProductionConfig(masterKey)

// Compliance (maximum security)
complianceConfig, _ := pc.GetComplianceConfig(masterKey)
```

## Data Types

The package supports different data types with context-specific key derivation:

- **DataTypeIP**: IP addresses
- **DataTypeEmail**: Email addresses  
- **DataTypeUsername**: Usernames
- **DataTypePII**: Personally identifiable information
- **DataTypeGeneric**: Generic sensitive data

Each data type uses a different derived key, ensuring that the same input produces different hashes for different contexts.

## Security Properties

### Cryptographic Strength
- **HMAC-SHA256**: Industry-standard message authentication
- **Argon2id**: Memory-hard key derivation function
- **Constant-time operations**: Resistant to timing attacks
- **Context separation**: Different keys for different data types

### Privacy Guarantees
- **Non-reversible**: Original data cannot be recovered from hashes
- **Deterministic**: Same input always produces same output
- **Avalanche effect**: Small input changes cause large output changes
- **Uniform distribution**: Hashes are uniformly distributed

### Performance Characteristics
- **Fast hashing**: Typical operations complete in microseconds
- **Configurable**: Adjust security/performance trade-offs
- **Memory efficient**: Bounded memory usage
- **Concurrent safe**: Thread-safe operations

## Integration with Audit Logging

The privacy package integrates seamlessly with the audit logging system:

```go
// In audit.go, privacy hashing is automatically applied
auditLogger := NewAuditLogger(repos, logger, config)
// IP addresses, usernames, and PII are automatically hashed based on configuration
```

### Audit Configuration

```bash
# Enable privacy hashing in audit logs
export ENABLE_PRIVACY_HASHING="true"
export PRIVACY_MASTER_KEY="your_key_here"

# Optional: Configure specific privacy levels
export IP_PRIVACY_LEVEL="partial"      # Preserve network for analysis
export USERNAME_PRIVACY_LEVEL="full"   # Full privacy for usernames
```

## Performance Considerations

### Memory Usage
- Default: 64 MB for Argon2 (production)
- Development: 4 MB (faster)
- Compliance: 256 MB (maximum security)

### CPU Usage
- Argon2 iterations control CPU cost
- More iterations = higher security + slower performance
- Recommended: 3-5 iterations for production

### Caching
- Hashes are deterministic (no need to cache)
- Key derivation is cached per data type
- No persistent state required

## Error Handling

```go
// Check for configuration errors
_, err := hasher.LoadHasherFromEnvironment()
if err != nil {
    // Handle missing master key, invalid configuration, etc.
    log.Fatal(err)
}

// Verify hash integrity
valid, err := hasher.VerifyHash(originalData, hash, dataType)
if err != nil {
    // Handle verification errors
}
```

## Migration and Key Rotation

### Key Rotation Support
```bash
# Enable key rotation (future feature)
export PRIVACY_KEY_ROTATION_ENABLED="true"
export PRIVACY_KEY_ROTATION_INTERVAL="24h"
```

### Migration Strategy
1. **Phase 1**: Deploy with privacy hashing disabled
2. **Phase 2**: Enable with existing data migration
3. **Phase 3**: Full privacy enforcement

```go
// Migration helper
func migrateExistingData(hasher *privacy.PrivacyHasher) {
    // Read existing audit logs
    // Apply privacy hashing retroactively
    // Update records in place
}
```

## Testing

### Unit Tests
```bash
go test ./pkg/privacy/ -v
```

### Security Tests
```bash
go test ./pkg/privacy/ -v -run TestSecurity
```

### Performance Benchmarks
```bash
go test ./pkg/privacy/ -bench=. -benchmem
```

## Compliance Considerations

### GDPR Compliance
- **Right to erasure**: Full hashing prevents data recovery
- **Data minimization**: Partial hashing preserves only necessary data
- **Purpose limitation**: Context-specific hashing

### CCPA Compliance
- **Consumer rights**: Hashing protects personal information
- **Data transparency**: Clear privacy level documentation
- **Opt-out support**: Configurable privacy levels

### SOC 2 Type II
- **Confidentiality**: Strong cryptographic protection
- **Processing integrity**: Deterministic, verifiable hashing
- **Availability**: High-performance, fault-tolerant design

## Best Practices

### Production Deployment
1. **Generate strong master key**: Use `GenerateMasterKeyBase64()`
2. **Store securely**: Use AWS Secrets Manager or similar
3. **Monitor performance**: Track hashing latency
4. **Regular key rotation**: Implement rotation strategy
5. **Audit configuration**: Log privacy settings

### Security Recommendations
1. **Never log master keys**: Use separate logging configuration
2. **Validate input data**: Sanitize before hashing
3. **Monitor for anomalies**: Track unusual hashing patterns
4. **Test regularly**: Run security test suite
5. **Document decisions**: Record privacy level choices

### Development Guidelines
1. **Use development preset**: Faster iteration
2. **Test all privacy levels**: Verify behavior
3. **Mock in unit tests**: Avoid real cryptographic operations
4. **Document data flows**: Track sensitive data usage
5. **Review regularly**: Update privacy requirements

## Troubleshooting

### Common Issues

**"master key not found"**
```bash
# Check environment variable
echo $PRIVACY_MASTER_KEY
# Generate new key if needed
privacy.GenerateMasterKeyBase64()
```

**"invalid master key length"**
- Master key must be at least 32 bytes
- Use base64 encoding for string keys
- Check for whitespace/newlines

**"argon2 memory too low"**
- Minimum 1024 KB (1 MB) required
- Increase for better security
- Balance with available memory

**Performance issues**
- Reduce Argon2 memory/iterations
- Use development preset for testing
- Profile application performance

### Debug Mode
```go
// Enable detailed logging
config.Debug = true
hasher, _ := privacy.NewPrivacyHasher(config)
```

## Contributing

1. Follow security best practices
2. Add comprehensive tests
3. Document security properties
4. Review cryptographic choices
5. Update benchmarks

## License

This package is part of the Lesser ActivityPub server project and follows the same license terms.