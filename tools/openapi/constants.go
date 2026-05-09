package main

const (
	pathGraphQL         = "/api/graphql"
	pathApps            = "/api/v1/apps"
	pathAppsRotate      = "/api/v1/apps/{id}/rotate_secret"
	pathStreamingRoot   = "/api/v1/streaming"
	pathStreamingHealth = "/api/v1/streaming/health"
	pathStreamingPrefix = "/api/v1/streaming/"
	pathTrustJWKS       = "/api/v1/trust/jwks.json"
	pathTrustAttest     = "/api/v1/trust/attestations"
	pathTrustAttestID   = "/api/v1/trust/attestations/{id}"
)
