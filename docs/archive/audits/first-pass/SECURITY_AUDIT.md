# Lesser Security Audit (Prototype Phase)

This document tracks security-related findings during the code review of the Lesser ActivityPub server. The project is currently in an unreleased prototype stage, so these findings are intended to guide development toward a more secure and robust implementation.

---

## Findings Summary

| ID      | Severity | Title                                             | Location                       | Status |
|---------|----------|---------------------------------------------------|--------------------------------|--------|
| LSS-001 | <span style="color:red">**Critical**</span> | Cross-Site Scripting (XSS) in HTML Sanitization   | `pkg/activitypub/validation.go` | Open   |
| LSS-002 | <span style="color:orange">**Medium**</span>   | Lack of Type-Safe Unmarshaling                    | `pkg/activitypub/types.go`      | Open   |
| LSS-003 | <span style="color:green">**Low**</span>      | Incomplete Validation Coverage                    | `pkg/activitypub/validation.go` | Open   |
| LSS-004 | <span style="color:green">**Low**</span>      | Bug in Address Validation Error Message           | `pkg/activitypub/validation.go` | Open   |
| LSS-005 | <span style="color:green">**Low**</span>      | Insecure `nil` RNG in RSA Signing                 | `pkg/federation/httpsig.go`     | Open   |
| LSS-006 | <span style="color:blue">**Info**</span>       | Limited HTTP Signature Algorithm Support          | `pkg/federation/httpsig.go`     | Open   |
| LSS-007 | <span style="color:red">**High**</span>     | Server-Side Request Forgery (SSRF)              | `pkg/federation/delivery.go`    | Open   |
| LSS-008 | <span style="color:orange">**Medium**</span>   | Lack of Delivery Retries                        | `pkg/federation/delivery.go`    | Open   |
| LSS-009 | <span style="color:green">**Low**</span>      | Insecure Random Number Generator for ID         | `pkg/federation/delivery.go`    | Open   |
| LSS-010 | <span style="color:red">**High**</span>     | Server-Side Request Forgery (SSRF)              | `pkg/federation/authorized_fetch.go` | Open   |
| LSS-011 | <span style="color:orange">**Medium**</span>   | Unsafe JSON Unmarshaling (DoS)                  | `pkg/federation/authorized_fetch.go` | Open   |
| LSS-012 | <span style="color:green">**Low**</span>      | Insecure `keyId` to `actorId` Conversion        | `pkg/federation/authorized_fetch.go` | Open   |
| LSS-013 | <span style="color:blue">**Info**</span>       | Secure Password Handling Confirmed                | `pkg/auth/password.go`          | Closed |
| LSS-014 | <span style="color:orange">**Medium**</span>   | No Refresh Token Reuse Detection                | `pkg/auth/session.go`           | Open   |
| LSS-015 | <span style="color:green">**Low**</span>      | Insecure Logging Practice                       | `pkg/auth/session.go`           | Open   |
| LSS-016 | <span style="color:blue">**Info**</span>       | WebAuthn `UserVerification` Not Specified       | `pkg/auth/webauthn.go`          | Open   |
| LSS-017 | <span style="color:blue">**Info**</span>       | WebAuthn Origins Not Configurable               | `pkg/auth/webauthn.go`          | Open   |
| LSS-018 | <span style="color:blue">**Info**</span>       | WebAuthn Resident Key Support Missing           | `pkg/auth/webauthn.go`          | Open   |
| LSS-019 | <span style="color:orange">**Medium**</span>   | Unimplemented Lockout Mechanism in Rate Limiter   | `pkg/auth/ratelimit.go`         | Open   |
| LSS-020 | <span style="color:red">**Critical**</span> | Server-Side Request Forgery (SSRF) in Inbox     | `cmd/inbox/main.go`             | Open   |
| LSS-021 | <span style="color:red">**High**</span>     | DoS via Unrestricted Request Body Size          | `cmd/inbox/main.go`             | Open   |
| LSS-022 | <span style="color:green">**Low**</span>      | Insecure Random Number Generator for IDs        | `cmd/inbox/main.go`             | Open   |
| LSS-023 | <span style="color:green">**Low**</span>      | Insecure "Fail Open" on Security Check          | `cmd/inbox/main.go`             | Open   |
| LSS-024 | <span style="color:red">**CRITICAL**</span> | **No Authentication on GraphQL Endpoint**         | `cmd/graphql/main.go`           | Open   |
| LSS-025 | <span style="color:red">**CRITICAL**</span> | **No Central Auth on REST API**                 | `cmd/api/main.go`               | Open   |
| LSS-026 | <span style="color:orange">**Medium**</span>   | Insecure Manual Routing Implementation          | `cmd/api/main.go`               | Open   |
| LSS-027 | <span style="color:red">**High**</span>     | No File Validation Before Processing            | `cmd/media-processor/main.go`   | Open   |
| LSS-028 | <span style="color:orange">**Medium**</span>   | Path Traversal in S3 Key Construction           | `cmd/media-processor/main.go`   | Open   |
| LSS-029 | <span style="color:orange">**Medium**</span>   | DoS via Unrestricted File Download              | `cmd/media-processor/main.go`   | Open   |
| LSS-030 | <span style="color:red">**Critical**</span> | **Data Leakage to Blocked Users**               | `cmd/outbox/main.go`            | Open   |
| LSS-031 | <span style="color:red">**High**</span>     | Unauthenticated Outbox Read Access              | `cmd/outbox/main.go`            | Open   |
| LSS-032 | <span style="color:orange">**Medium**</span>   | DoS via Unrestricted Request Body Size          | `cmd/outbox/main.go`            | Open   |
| LSS-033 | <span style="color:green">**Low**</span>      | Insecure Random Number Generator for IDs        | `cmd/outbox/main.go`            | Open   |

---

## Detailed Findings

### LSS-001: Cross-Site Scripting (XSS) in HTML Sanitization

- **Severity**: <span style="color:red">**Critical**</span>
- **Location**: `pkg/activitypub/validation.go`, function `SanitizeHTML`
- **Description**: The current HTML sanitization function uses a blocklist approach with `strings.ReplaceAll` to remove `<script>` tags and `javascript:` protocols. This method is insufficient and can be easily bypassed by attackers using various techniques (e.g., event handlers like `onerror`, different character encodings, or variations in capitalization). This vulnerability could allow an attacker to inject malicious scripts into user-generated content, leading to account compromise, data theft, and other client-side attacks.
- **Recommendation**: Replace the custom sanitizer immediately with a well-vetted, industry-standard library like `bluemonday`. Use a "least-privilege" policy (like `bluemonday.UGCPolicy()`) that allows only a known-safe subset of HTML tags and attributes and denies everything else.

### LSS-002: Lack of Type-Safe Unmarshaling

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `pkg/activitypub/types.go`, structs `Activity` and `Collection`
- **Description**: The `Object` field in the `Activity` struct and the `Items`/`OrderedItems` fields in `Collection` structs use `interface{}`. This forces the application to rely on runtime type assertions, which can lead to unexpected panics if the incoming data's `type` field doesn't match expectations. An attacker could potentially craft malicious payloads to trigger these panics, leading to a denial-of-service (DoS) condition in the service processing the activity.
- **Recommendation**: Implement a custom `UnmarshalJSON` method for these structs. The method should first parse the JSON into a generic map to inspect the `type` property, then unmarshal the data into the corresponding concrete type. This improves robustness and prevents unexpected crashes.

### LSS-003: Incomplete Validation Coverage

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `pkg/activitypub/validation.go`
- **Description**: Validation functions are only implemented for a few core ActivityPub types (`Actor`, `Activity`, `Note`). Many other types defined in `types.go` (e.g., `Article`, `Image`, specific activities like `Follow`, `Like`) lack corresponding validation. Accepting and processing potentially malformed or incomplete objects from federated servers could lead to data corruption or unexpected application behavior.
- **Recommendation**: Implement validation functions for all ActivityPub types that the server can receive and process. This ensures data integrity at the boundary.

### LSS-004: Bug in Address Validation Error Message

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `pkg/activitypub/validation.go`, function `ValidateAddressing`
- **Description**: The error message for an invalid address in a `to` or `cc` field uses `string(rune(i))` to report the index of the invalid entry. This incorrectly converts the integer index to a character, not a numeric string, making the error message confusing and difficult to debug.
- **Recommendation**: Use `strconv.Itoa(i)` to correctly format the index number in the error string.

### LSS-005: Insecure `nil` RNG in RSA Signing

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `pkg/federation/httpsig.go`, function `SignHTTPRequest`
- **Description**: The call to `rsa.SignPKCS1v15` uses `nil` for the `rand` argument. While the Go standard library defaults to using the cryptographically secure `crypto/rand.Reader` in this case, it is best practice to be explicit.
- **Recommendation**: Explicitly pass `rand.Reader` from the `crypto/rand` package to the `rsa.SignPKCS1v15` function to ensure that the most secure random number generator is always used and to make the code's intent clearer.

### LSS-006: Limited HTTP Signature Algorithm Support

- **Severity**: <span style="color:blue">**Info**</span>
- **Location**: `pkg/federation/httpsig.go`
- **Description**: The HTTP signature verification and signing logic is hardcoded to only support the `rsa-sha256` algorithm. While this is the most common algorithm used in the Fediverse, the specification allows for others (e.g., `ecdsa-sha256`, `hs2019`). This could lead to federation failures with servers that use a different, valid algorithm.
- **Recommendation**: Refactor the signing and verification functions to support multiple algorithms. Use a map or a switch statement based on the `algorithm` field of the signature to select the correct verification strategy. This will improve interoperability with a wider range of ActivityPub implementations.

### LSS-007: Server-Side Request Forgery (SSRF)

- **Severity**: <span style="color:red">**High**</span>
- **Location**: `pkg/federation/delivery.go`, function `fetchRemoteActor`
- **Description**: The application fetches remote actor documents by making a direct HTTP request to the `actorID` provided in the activity. An attacker could craft a malicious activity with an `actorID` that points to an internal, non-public IP address (e.g., `127.0.0.1`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, or the AWS metadata service at `169.254.169.254`). This would cause the server to make a request to an internal service, potentially exposing sensitive data or enabling further attacks within the infrastructure.
- **Recommendation**: Implement a multi-layered defense:
    1.  **Validate URLs**: Before making any request, validate that the host in the URL is a public, routable IP address. Disallow requests to private, reserved, or loopback IP address ranges.
    2.  **HTTP Client Configuration**: Configure the HTTP client to prevent it from following redirects, as redirects can also be used to bypass URL validation.
    3.  **Network Policies**: Implement strict egress filtering at the network level (e.g., AWS Security Groups) to prevent the application server from making outbound connections to internal services.

### LSS-008: Lack of Delivery Retries

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `pkg/federation/delivery.go`, function `DeliverActivity`
- **Description**: The synchronous `DeliverActivity` function attempts to deliver an activity only once. If the remote server is temporarily unavailable or returns a transient error (e.g., a 5xx status code), the delivery fails permanently. This can lead to missed activities and an inconsistent state across the Fediverse. The SQS implementation also appears to fall back to this synchronous method without a clear retry strategy in the worker.
- **Recommendation**: Implement an exponential backoff retry mechanism for the `DeliverActivity` function. When a delivery fails with a transient error (like a timeout or a 5xx response), the service should wait for a short, increasing duration before trying again, up to a maximum number of retries. This will significantly improve the reliability of federation.

### LSS-009: Insecure Random Number Generator for ID

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `pkg/federation/delivery.go`, function `generateDeliveryID`
- **Description**: The `generateDeliveryID` function uses `math/rand.Read`, which is a pseudo-random number generator. For generating unique identifiers, especially in a distributed system, it is better to use a cryptographically secure source of randomness to minimize the risk of collisions.
- **Recommendation**: Replace `math/rand.Read` with `crypto/rand.Read` to generate the byte slice for the delivery ID. This ensures a higher degree of randomness and reduces the likelihood of generating duplicate IDs.

### LSS-010: Server-Side Request Forgery (SSRF)

- **Severity**: <span style="color:red">**High**</span>
- **Location**: `pkg/federation/authorized_fetch.go`, functions `FetchObject`, `fetchActorWithoutAuth`
- **Description**: This service is also vulnerable to SSRF. The functions make direct HTTP requests to URLs provided in the `objectURL` and `actorURL` parameters without validating them. This vulnerability is identical to `LSS-007` and could be exploited by an attacker to scan internal networks or access sensitive cloud metadata services.
- **Recommendation**: Apply the same multi-layered defense as recommended for `LSS-007`:
    1.  **Validate URLs**: Ensure all outbound request URLs point to public, routable IP addresses.
    2.  **Configure HTTP Client**: Disable redirects in the HTTP client.
    3.  **Network Policies**: Use strict egress filtering to block outbound traffic to internal service ranges.

### LSS-011: Unsafe JSON Unmarshaling (DoS)

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `pkg/federation/authorized_fetch.go`, function `FetchObject`
- **Description**: The `FetchObject` function decodes arbitrary JSON from remote servers into a `map[string]interface{}`. A malicious server could respond with a very large or deeply nested JSON object. Decoding this without limits could consume excessive memory or CPU, leading to a denial-of-service (DoS) attack.
- **Recommendation**: Implement safer JSON decoding practices:
    1.  **Limit Response Size**: Use `io.LimitReader` to cap the size of the HTTP response body before it is passed to the JSON decoder. This prevents attacks based on very large objects.
    2.  **Avoid Recursive Decoding**: While `encoding/json` has some internal limits, consider using a streaming parser for very large or complex objects if they need to be processed, which avoids deep recursion.

### LSS-012: Insecure `keyId` to `actorId` Conversion

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `pkg/federation/authorized_fetch.go`, function `VerifyAuthorizedFetch`
- **Description**: The function assumes that the actor's ID can be reliably determined by removing the fragment from the `keyId` of a signed request. This is a common convention but not a strict rule. An attacker could craft a `keyId` where the base URL points to a different actor, potentially causing the server to fetch the wrong public key and either fail verification or, in a flawed implementation, accept a signature from an unintended actor.
- **Recommendation**: After fetching the actor document using the base URL from the `keyId`, verify that the public key specified in the `keyId` (including the fragment) is actually present in the fetched actor's document. The `id` of the `publicKey` object in the actor document should match the `keyId` from the signature header. Do not trust that the base URL is the actor's final, canonical ID.

### LSS-013: Secure Password Handling Confirmed

- **Severity**: <span style="color:blue">**Info**</span>
- **Location**: `pkg/auth/password.go`
- **Description**: The password handling implementation was reviewed and found to be secure. It correctly uses the `bcrypt` algorithm with a modern cost factor, which is the current industry best practice.
- **Recommendation**: No action required. Continue to monitor bcrypt best practices and consider increasing the cost factor in the future as computing power increases.

### LSS-014: No Refresh Token Reuse Detection

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `pkg/auth/session.go`, function `ValidateRefreshToken`
- **Description**: The session manager implements refresh token rotation, which is a strong security measure. However, it does not appear to detect the reuse of a compromised token. A best-practice pattern for token rotation is to invalidate the entire session family if an old (already used) refresh token is presented to the server. This indicates that the token was likely stolen, and the user's account may be compromised. The current implementation allows an old token to be used within a grace period but does not treat its reuse as a security event.
- **Recommendation**: Enhance the `ValidateRefreshToken` function. When a token from the `PreviousRefreshToken` field is used, the system should immediately revoke the entire session (and possibly all sessions for that user) and log a high-priority security alert. This provides a powerful mechanism for detecting and responding to session hijacking attempts.

### LSS-015: Insecure Logging Practice

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `pkg/auth/session.go`, functions `CreateSession`, `RevokeAllUserSessions`
- **Description**: The code uses `fmt.Printf` for logging errors in several places. This bypasses the structured logging framework (`zap`) used elsewhere in the application. Unstructured logging makes logs harder to parse and monitor. Furthermore, printing raw error messages to standard output could inadvertently leak sensitive information, such as database connection details or internal file paths, if not handled carefully.
- **Recommendation**: Replace all instances of `fmt.Printf` with the structured logger (e.g., `sm.storage.logger.Warn(...)` or `sm.storage.logger.Error(...)`). This ensures that all log messages are consistent, structured, and can be properly managed and secured in a production environment.

### LSS-016: WebAuthn `UserVerification` Not Specified

- **Severity**: <span style="color:blue">**Info**</span>
- **Location**: `pkg/auth/webauthn.go`, function `NewWebAuthnService`
- **Description**: The WebAuthn configuration does not explicitly set the `AuthenticatorSelection.UserVerification` preference. This setting controls whether the user must verify their presence with the authenticator (e.g., via biometrics or a PIN). While the library's default is often reasonable (`preferred`), for a high-security application, this should be an explicit choice. Not requiring user verification can make it easier for an attacker to use a stolen, unlocked device.
- **Recommendation**: Explicitly set the `UserVerification` requirement in the `webauthn.Config`. A common strategy is to set it to `protocol.VerificationPreferred` for general logins but allow for it to be overridden to `protocol.VerificationRequired` for step-up authentication before sensitive actions.

### LSS-017: WebAuthn Origins Not Configurable

- **Severity**: <span style="color:blue">**Info**</span>
- **Location**: `pkg/auth/webauthn.go`, function `NewWebAuthnService`
- **Description**: The `RPOrigins` (Relying Party Origins) are hardcoded based on the `domain` parameter. This lacks flexibility for deployments that might involve multiple valid origins, such as `localhost` for development, a staging domain, and the production domain. An incorrect origin configuration will cause WebAuthn ceremonies to fail in the browser.
- **Recommendation**: Make the list of allowed origins configurable, for example, through an environment variable that takes a comma-separated list of strings. This allows the same codebase to be deployed in different environments without modification.

### LSS-018: WebAuthn Resident Key Support Missing

- **Severity**: <span style="color:blue">**Info**</span>
- **Location**: `pkg/auth/webauthn.go`, function `BeginLogin`
- **Description**: The login flow requires the `username` to be provided before starting the WebAuthn ceremony. This means the implementation does not support "discoverable credentials" (resident keys), which allow users to log in without entering a username first. This is a user experience and feature issue rather than a security vulnerability.
- **Recommendation**: To support discoverable credentials, create a separate login initiation endpoint that does not require a username. When calling `webAuthn.BeginLogin`, pass a `nil` user. The client-side logic will need to be adapted to handle this "passwordless" flow.

### LSS-019: Unimplemented Lockout Mechanism in Rate Limiter

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `pkg/auth/ratelimit.go`, function `imposeLockout`
- **Description**: The `imposeLockout` function is a stub and does not implement any logic. The comment "For now, we'll rely on the attempt count mechanism" suggests that the system does not create an explicit, persistent lockout record in storage. Instead, it relies on re-calculating the number of recent attempts on every check. This makes the rate-limiting system's correctness entirely dependent on a complex and unaudited implementation within the storage layer. An error in the storage layer's query could cause the rate limiting to fail silently.
- **Recommendation**: Implement the `imposeLockout` function. When a rate limit is triggered, it should create a dedicated record in the storage layer (e.g., `lockout:ip:1.2.3.4`) with a specific Time-To-Live (TTL) matching the lockout duration. The `CheckRateLimit` function should then be simplified to check for the existence of this explicit record. This approach is more robust, easier to debug, and less prone to logic errors in database queries.

### LSS-020: Server-Side Request Forgery (SSRF) in Inbox

- **Severity**: <span style="color:red">**Critical**</span>
- **Location**: `cmd/inbox/main.go`, function `fetchActorPublicKey`
- **Description**: This is another instance of the same SSRF vulnerability identified in `LSS-007` and `LSS-010`. The inbox handler fetches an actor's public key by making a direct HTTP request to the `actorURL` provided in the incoming activity payload. This URL is not validated, allowing an attacker to force the server to make requests to internal services, which could expose sensitive infrastructure metadata or internal APIs.
- **Recommendation**: This vulnerability must be fixed urgently. Apply the same multi-layered defense as recommended previously:
    1.  **Validate URLs**: Create a centralized, shared utility for making outbound HTTP requests that rigorously validates all URLs against a blocklist of private/reserved IP ranges.
    2.  **Network Policies**: Enforce strict egress filtering at the firewall/security group level to prevent the Lambda function from initiating connections to internal IPs.

### LSS-021: DoS via Unrestricted Request Body Size

- **Severity**: <span style="color:red">**High**</span>
- **Location**: `cmd/inbox/main.go`, function `handlePostInbox`
- **Description**: The inbox handler reads the entire request body into memory using `[]byte(request.Body)` without imposing any size limit. An attacker could send a POST request with a very large body (e.g., several gigabytes), which would exhaust the memory of the Lambda instance, causing it to crash. Repeatedly sending such requests would lead to a denial-of-service (DoS) attack.
- **Recommendation**: Always use `io.LimitReader` to wrap the request body before reading from it. Set a reasonable limit (e.g., 1MB) that is sufficient for legitimate ActivityPub objects but prevents memory exhaustion attacks. The read operation will return an error if the body exceeds the limit.

### LSS-022: Insecure Random Number Generator for IDs

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `cmd/inbox/main.go`, function `generateRandomString`
- **Description**: The `generateRandomString` function, used to create activity IDs, uses `time.Now().UnixNano()` as a seed for randomness. This is not a cryptographically secure source of random numbers and can lead to predictable IDs and potential collisions, especially in a high-throughput or distributed environment.
- **Recommendation**: Replace the custom random string generator with a function that uses `crypto/rand`. A simple way to do this is to generate a slice of random bytes and then encode them to a hex or base64 string.

### LSS-023: Insecure "Fail Open" on Security Check

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `cmd/inbox/main.go`, function `handlePostInbox`
- **Description**: When checking if a domain is blocked, the code explicitly continues processing if the check fails, with a comment "fail open rather than closed". For a security control like a domain blocklist, this is the wrong approach. If the blocklist check fails, an activity from a potentially malicious domain could be processed.
- **Recommendation**: Change the logic to "fail closed." If the `store.IsDomainBlocked` function returns an error, the activity should be rejected with an internal server error. This ensures that the system does not process activities from unknown-risk sources when security controls are malfunctioning.

### LSS-024: No Authentication on GraphQL Endpoint

- **Severity**: <span style="color:red">**CRITICAL**</span>
- **Location**: `cmd/graphql/main.go`, function `lambdaHandler`
- **Description**: The main Lambda handler for the GraphQL API does not implement any authentication or authorization middleware. It sets up the `gqlgen` server and immediately passes requests to it. This means that the entire GraphQL API, including all queries and mutations, is unauthenticated and publicly exposed. Any anonymous user can execute potentially destructive mutations like `deleteObject` or query for sensitive data like direct message timelines or notifications. This constitutes a complete absence of access control.
- **Recommendation**: **This is the most critical vulnerability found and must be fixed immediately.**
    1.  **Implement Auth Middleware**: The `lambdaHandler` must be wrapped in an authentication middleware. This middleware should be responsible for extracting the session token (e.g., JWT) from the request headers, validating it, and parsing the user's claims. If the token is missing or invalid, the request must be rejected with a `401 Unauthorized` error.
    2.  **Inject User into Context**: After successful authentication, the middleware should inject the authenticated user's identity (e.g., username, roles, scopes) into the request context.
    3.  **Enforce Authorization in Resolvers**: The resolver functions must then use the user identity from the context to perform authorization checks for every single operation, ensuring that the authenticated user has the permission to perform the requested action on the target resource. Until this is implemented, the API should not be exposed to the internet.

### LSS-025: No Central Auth on REST API

- **Severity**: <span style="color:red">**CRITICAL**</span>
- **Location**: `cmd/api/main.go`, function `handleRequest`
- **Description**: The main request handler for the REST-style API is a very large routing block that dispatches requests to individual handler functions. Although an `authMiddleware` is initialized in the `init()` function, it is **never used**. There is no central middleware to enforce authentication and authorization. This means each of the hundreds of individual handler functions is responsible for implementing its own security checks. This pattern is extremely dangerous and almost guarantees that some endpoints will be left unprotected by mistake. Any handler that forgets to perform an auth check is, by default, a public and unauthenticated endpoint.
- **Recommendation**: **This is a critical architectural flaw and must be fixed immediately.**
    1.  **Use a Router with Middleware Support**: Replace the manual `if/else if` routing block with a standard HTTP router library (e.g., `chi`, `gorilla/mux`).
    2.  **Enforce Authentication by Default**: Apply the authentication middleware to all routes by default. Routes that are intentionally public (e.g., OAuth registration) should be explicitly excluded. This "deny by default" approach is much safer.
    3.  **Centralize Auth Logic**: The middleware should handle token validation and inject the user context, just as recommended for the GraphQL API in `LSS-024`. This removes the burden of authentication from the individual handlers, allowing them to focus on authorization.

### LSS-026: Insecure Manual Routing Implementation

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `cmd/api/main.go`, function `handleRequest`
- **Description**: The API router is implemented as a single, massive function with a long chain of `if/else if` statements that perform string matching on the request path. This approach is highly fragile and prone to security vulnerabilities. For example, a slight misordering of the checks or a subtle error in a `strings.HasPrefix` call could cause a request to be handled by the wrong logic or bypass a security check intended for a more specific path.
- **Recommendation**: Refactor the routing logic to use a dedicated HTTP router library. These libraries provide robust, well-tested pattern matching for URL paths, support for path parameters, and a clean model for attaching middleware to groups of routes. This will make the API more secure, maintainable, and easier to reason about.

### LSS-027: No File Validation Before Processing

- **Severity**: <span style="color:red">**High**</span>
- **Location**: `cmd/media-processor/main.go`, function `processMediaJob`
- **Description**: The media processor fetches a file from S3 and passes it to a processing function (e.g., `processImage`) based on a `MimeType` field retrieved from a database record. The service never independently verifies the actual file type of the content it downloads. An attacker could upload a malicious file (e.g., a decompression bomb, a polyglot file with embedded scripts, or a file designed to exploit a vulnerability in a downstream library) and give it a fake MIME type like `image/jpeg`. The processor would then blindly pass this malicious file to the image processing library, which could lead to denial-of-service or remote code execution.
- **Recommendation**: **Never trust user-provided metadata like MIME types.** Before passing the downloaded data to any processing library, perform server-side file type detection. Use a library that inspects the file's magic bytes (the first few bytes of the content) to determine its true type. A good choice in Go is `net/http.DetectContentType`. The processing should only proceed if the detected file type matches an allowlist of safe types (e.g., `image/jpeg`, `image/png`, `video/mp4`).

### LSS-028: Path Traversal in S3 Key Construction

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `cmd/media-processor/main.go`, functions `processImage`, `processVideo`, etc.
- **Description**: The S3 keys for storing processed media are constructed by directly embedding the `username` from the event message (e.g., `media/{username}/{mediaID}/...`). A malicious user could register an account with a username containing path traversal characters, such as `../otheruser/` or `../../system`. This could allow them to write files to arbitrary locations within the S3 bucket, potentially overwriting other users' data or system files.
- **Recommendation**: Always sanitize any user-provided input that is used to construct file paths or object keys. Before using the `username` in the S3 key, it should be sanitized to remove directory traversal characters (`/`, `\`, `..`, etc.). A simple approach is to use `filepath.Base` on the username or to implement a strict allowlist regex (e.g., `^[a-zA-Z0-9_-]+$`) and reject any username that doesn't match.

### LSS-029: DoS via Unrestricted File Download

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `cmd/media-processor/main.go`, function `downloadFromS3`
- **Description**: The function downloads the entire file from S3 into memory with `io.ReadAll` without any size limit. An attacker could upload an extremely large file. When the processor attempts to handle this file, it will consume all available memory in the Lambda environment, causing a crash and a denial-of-service.
- **Recommendation**: This is the same vulnerability as `LSS-021`. Before reading the file content, check the object's `Content-Length` from the S3 metadata. If it exceeds a reasonable limit (e.g., 50MB), the job should be failed immediately without attempting to download the file. For streaming processing where the whole file isn't needed in memory, use `io.LimitReader` to prevent reading excessive data.

### LSS-030: Data Leakage to Blocked Users

- **Severity**: <span style="color:red">**Critical**</span>
- **Location**: `cmd/outbox/main.go`, function `deliverActivityRemotely`
- **Description**: The outbox delivery function prepares a list of recipients (followers and mentioned users) and passes it to the `federation.DeliveryService`. The function **does not filter this recipient list against the sender's blocklist.** This means that activities, including followers-only and direct messages, will be delivered to users who the sender has explicitly blocked. This is a critical safety and privacy failure.
- **Recommendation**: Before calling `deliveryService.DeliverToFollowers` or `deliveryService.DeliverToRecipients`, the `deliverActivityRemotely` function must first retrieve the actor's blocklist from storage. It must then iterate through the resolved list of recipient inboxes and remove any that belong to an actor on the blocklist.

### LSS-031: Unauthenticated Outbox Read Access

- **Severity**: <span style="color:red">**High**</span>
- **Location**: `cmd/outbox/main.go`, function `handleGetOutbox`
- **Description**: The `handleGetOutbox` function, which serves the content of a user's outbox, has no authentication or authorization checks. This makes every user's outbox contents publicly readable. For users who have enabled protected or followers-only mode, this is a severe privacy breach, as it leaks all their posts to the public.
- **Recommendation**: The `handleGetOutbox` function must be protected by the same authentication middleware as the POST handler. After authentication, the handler should check if the requesting user is the owner of the outbox or a follower. Based on the requesting user's relationship to the outbox owner, the handler must filter the returned activities to respect their visibility settings (e.g., a non-follower should only see public posts).

### LSS-032: DoS via Unrestricted Request Body Size

- **Severity**: <span style="color:orange">**Medium**</span>
- **Location**: `cmd/outbox/main.go`, function `handlePostOutbox`
- **Description**: This is the same vulnerability as `LSS-021`. The handler reads the entire request body into memory without a size limit. Even though this endpoint is authenticated, a malicious or compromised local user could send a very large payload to crash the service.
- **Recommendation**: Wrap the request body with `io.LimitReader` to enforce a reasonable maximum size (e.g., 1MB) for incoming activity JSON.

### LSS-033: Insecure Random Number Generator for IDs

- **Severity**: <span style="color:green">**Low**</span>
- **Location**: `cmd/outbox/main.go`, functions `generateActivityID`, `generateRandomString`
- **Description**: This service uses the same insecure, time-based random string generator as the inbox. This can lead to predictable IDs and potential collisions.
- **Recommendation**: Replace the custom random string generator with a function that uses `crypto/rand` for all ID generation.

---

## Conclusion: Architectural Recommendations for Hardening

This security audit has identified 33 issues, including four critical and four high-severity vulnerabilities. The findings indicate that while the business logic and feature set of the application are advanced, the foundational security controls—particularly for the client-facing APIs—are not yet implemented, as is common in an early-stage prototype.

The pattern of vulnerabilities points to a clear, prioritized path for hardening the application. The following architectural recommendations should be considered during remediation. It is strongly advised to adopt these patterns over custom implementations, as they address complex security problems that have been solved by open, well-vetted libraries.

### 1. Implement Centralized Authentication via Middleware

The most critical priority is to fix the lack of authentication on the `cmd/graphql` (LSS-024) and `cmd/api` (LSS-025) services.

- **The Pattern:** In a Lambda environment, this is achieved by creating a middleware function that wraps the main application logic. This middleware must be the first code to execute after the handler starts. Its responsibilities are to:
    1.  Inspect the request for a session token.
    2.  Validate the token.
    3.  **Immediately reject** any unauthenticated/invalid requests with a `401 Unauthorized` error.
    4.  Inject a validated user identity object into the request's `context.Context` for valid sessions.
- **Why it's necessary:** This "deny by default" approach ensures that no new API endpoint can be accidentally exposed without authentication. It removes the burden and risk of every single endpoint being responsible for its own security checks. The `cmd/outbox` service's `handlePostOutbox` function provides a good template for this pattern.
- **Future State:** For even better performance and separation of concerns, consider using an **API Gateway Lambda Authorizer**. This is a dedicated Lambda that validates the token at the API Gateway level, before the request even reaches your main application code.

### 2. Adopt a Standard HTTP Router for the REST API

The manual `if/else` routing in `cmd/api` (LSS-026) is a high-risk pattern that should be replaced.

- **The Pattern:** Use the `aws-lambda-go-api-proxy/httpadapter` library to wrap a standard, robust HTTP router like `chi` or `gorilla/mux`. Your main Lambda handler's only job is to pass the event to this adapter. The router then handles all path matching and dispatches to your handler functions.
- **Why it addresses the "overhead" concern:**
    - **Performance:** The performance overhead of a library like `chi` is measured in nanoseconds and is completely negligible compared to any business logic or database call your application makes.
    - **Simplicity:** The "simplicity" of manual routing is deceptive. It forces you to take on the hidden complexity and security risks of a custom implementation. The true simplicity comes from declaring your routes and middleware in a clean, readable, and maintainable way.
- **Why it's more secure:**
    - It allows you to apply the centralized authentication middleware (from point #1) to entire groups of routes.
    - It provides safe, automatic parsing of URL parameters (e.g., `/statuses/{statusID}`).
    - It eliminates the risk of incorrect route matching from fragile `strings.HasPrefix` logic.

### 3. Build a Secure, Centralized HTTP Client for Federation

The recurring SSRF vulnerabilities (LSS-007, LSS-010, LSS-020) highlight the need for a single, secure way to make outbound HTTP requests.

- **The Pattern:** Create a new utility package (e.g., `pkg/httpclient`). This package should provide a default `http.Client` that is pre-configured to:
    - Validate all outbound URLs against a blocklist of private/reserved IP ranges.
    - Disable redirects (`CheckRedirect`).
    - Enforce reasonable timeouts.
- **Why it's necessary:** This consolidates all SSRF-prevention logic in one place. All other services (`federation`, `inbox`, etc.) must be refactored to use this client. This eliminates the risk of a developer forgetting to add SSRF protections for a new feature that makes outbound requests.

### 4. Systematically Remediate Other High-Priority Findings

The detailed recommendations for the remaining high-priority findings should be followed:
- **Data Leakage (LSS-030):** The `outbox` must be modified to filter all recipient lists against the sender's blocklist before delivery.
- **Denial of Service (LSS-021, LSS-029, LSS-032):** All handlers that process request bodies or download files must enforce strict size limits.
- **Cross-Site Scripting (LSS-001):** The placeholder HTML sanitizer must be replaced with a robust library like `bluemonday`.

By focusing on these architectural improvements, the security posture of the Lesser prototype will be significantly improved, providing a solid foundation to build upon.

*This concludes the security audit.* 