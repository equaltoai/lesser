# Week 1 Implementation Prompt - Critical Foundation (REVISED)

## Context
You are working on the Lesser codebase, a serverless ActivityPub implementation. The audit reports contained outdated information - the storage migration is already complete. This revised prompt focuses on the actual critical issues that need fixing.

## Actual State of the Codebase
- ✅ Storage migration (h.store → h.repos) is COMPLETE
- ⚠️ Sonic JSON is only used in one file: `pkg/common/json.go`
- ⚠️ 18 TODO/FIXME/HACK comments exist
- ⚠️ 69 instances of direct `os.Getenv()` usage
- ⚠️ Security vulnerabilities from gosec need addressing

## Your Mission for Week 1 (Revised)

### Day 1-2: Replace Sonic with Standard JSON

**Objective:** Remove the Sonic dependency and use standard library JSON until Go 1.25 is available.

**Current State:** 
- Only `pkg/common/json.go` uses Sonic
- Go 1.24 is current version

**Instructions:**

1. **Update pkg/common/json.go to use encoding/json:**
   ```go
   package common

   import (
       "encoding/json"
       "io"
   )

   // Marshal serializes v to JSON
   func Marshal(v any) ([]byte, error) {
       return json.Marshal(v)
   }

   // Unmarshal deserializes JSON data
   func Unmarshal(data []byte, v any) error {
       return json.Unmarshal(data, v)
   }

   // MarshalString for string output
   func MarshalString(v any) (string, error) {
       bytes, err := json.Marshal(v)
       if err != nil {
           return "", err
       }
       return string(bytes), nil
   }

   // NewEncoder creates a streaming JSON encoder
   func NewEncoder(w io.Writer) *json.Encoder {
       return json.NewEncoder(w)
   }

   // NewDecoder creates a streaming JSON decoder
   func NewDecoder(r io.Reader) *json.Decoder {
       return json.NewDecoder(r)
   }
   
   // TODO: Upgrade to encoding/json/v2 when Go 1.25 is available
   ```

2. **Remove Sonic from dependencies:**
   ```bash
   go mod edit -droprequire github.com/bytedance/sonic
   go mod tidy
   ```

3. **Test compilation:**
   ```bash
   go build ./...
   ```

**Success Criteria:**
- No Sonic imports in the codebase
- All JSON operations work correctly
- Code compiles successfully

### Day 2-3: Fix Critical Security Vulnerabilities

**Objective:** Address the highest severity security issues from gosec findings.

#### Priority 1: Timing Attack Prevention

1. **Find password verification code:**
   ```bash
   grep -rn "VerifyPassword\|bcrypt.CompareHashAndPassword" --include="*.go"
   ```

2. **Ensure constant-time comparison is used:**
   - bcrypt.CompareHashAndPassword already uses constant-time comparison
   - If custom verification exists, use `crypto/subtle.ConstantTimeCompare`

#### Priority 2: Path Traversal Prevention (CWE-22)

1. **Find instances in non-generated code:**
   ```bash
   grep -rn "os.Open\|ioutil.ReadFile\|os.ReadFile" --include="*.go" --exclude="*generated*" --exclude="*test.go"
   ```

2. **Fix each instance with path sanitization:**
   ```go
   import "path/filepath"
   
   // Sanitize and validate paths
   cleanPath := filepath.Clean(userInput)
   if !strings.HasPrefix(cleanPath, expectedBaseDir) {
       return errors.New("invalid path")
   }
   ```

#### Priority 3: Weak Cryptography

1. **Find RSA key generation:**
   ```bash
   grep -rn "rsa.GenerateKey" --include="*.go"
   ```

2. **If found, ensure 4096-bit keys:**
   ```go
   key, err := rsa.GenerateKey(rand.Reader, 4096)
   ```

3. **Check for weak hashes:**
   ```bash
   grep -rn "md5\|sha1" --include="*.go" -i
   ```

4. **Replace with SHA-256 or better**

#### Priority 4: JWT Secret Management

1. **Find JWT secret usage:**
   ```bash
   grep -rn "jwt\|JWT" --include="*.go" | grep -i "secret\|key"
   ```

2. **Ensure secrets come from environment or AWS Secrets Manager:**
   - No hardcoded secrets in code
   - Secrets should be loaded from config/environment

### Day 3-4: Environment Configuration Cleanup

**Objective:** Replace direct `os.Getenv()` with centralized configuration.

1. **Find all os.Getenv usage:**
   ```bash
   grep -rn "os\.Getenv" --include="*.go" > getenv_usage.txt
   wc -l getenv_usage.txt  # Should show 69
   ```

2. **Group by package to understand patterns:**
   ```bash
   grep -rn "os\.Getenv" --include="*.go" | cut -d: -f1 | sort | uniq -c | sort -rn
   ```

3. **For each file with os.Getenv:**
   - Import the config package
   - Replace `os.Getenv("KEY")` with `config.Get("KEY")`
   - Replace `os.Getenv("KEY")` with default with `config.GetString("KEY", "default")`
   
   **Exception:** Keep os.Getenv for AWS Lambda runtime variables:
   - `_LAMBDA_*`
   - `AWS_LAMBDA_*`
   - `LAMBDA_*`

4. **Verify all are replaced:**
   ```bash
   # Should be much lower, only Lambda runtime vars
   grep -rn "os\.Getenv" --include="*.go" | grep -v "_LAMBDA\|AWS_LAMBDA\|LAMBDA_" | wc -l
   ```

### Day 5: Critical TODO Resolution

**Objective:** Address the most critical TODOs.

1. **List all TODOs:**
   ```bash
   grep -rn "TODO\|FIXME\|HACK" --include="*.go" > todos.txt
   ```

2. **Prioritize by location:**
   - Focus on auth, security, and data handling TODOs
   - Skip UI/cosmetic TODOs
   - Skip TODOs in test files

3. **For each critical TODO:**
   - Either implement the fix
   - Or create a GitHub issue and add issue number to the comment

## Verification Checklist

After completing Week 1, verify:

```bash
# 1. No Sonic dependency
grep -n "sonic" go.mod  # Should find nothing

# 2. Build succeeds
go build ./...

# 3. Linting passes
golangci-lint run ./...

# 4. Reduced os.Getenv usage (only Lambda runtime vars)
grep -rn "os\.Getenv" --include="*.go" | grep -v "_LAMBDA\|AWS_LAMBDA\|LAMBDA_" | wc -l  # Should be 0 or very low

# 5. Critical paths compile
go build ./cmd/api
go build ./cmd/auth
go build ./cmd/graphql
```

## Important Notes

1. **The storage migration is already done** - ignore audit comments about h.store
2. **Don't edit generated files** - Skip any issues in `*generated*.go` files
3. **Check .golangci.yml** - Many issues may already be excluded for valid reasons
4. **Test compilation frequently** - After each major change
5. **Document decisions** - If you skip a TODO or security issue, explain why

## Common Pitfalls to Avoid

1. **Don't edit graph/generated.go** - It's auto-generated
2. **Don't fix "issues" that are already complete** - Storage migration is done
3. **Don't blindly trust audit reports** - They may be outdated
4. **Don't partially complete tasks** - If there are 69 os.Getenv, fix all 69 (minus Lambda exceptions)

## Success Metrics for Week 1

- [ ] Sonic dependency removed
- [ ] Security vulnerabilities in source code addressed
- [ ] os.Getenv usage minimized (Lambda runtime only)
- [ ] Critical TODOs addressed or tracked
- [ ] Codebase compiles without errors
- [ ] golangci-lint runs successfully

Begin with replacing Sonic as it's the simplest and highest impact change. Good luck!