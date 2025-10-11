package middleware

import (
	"os"
	"strings"
)

// GetFederationCORSConfig returns CORS config for ActivityPub federation endpoints
func GetFederationCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"}, // Required for federation
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept",
			"Content-Type",
			"Date",
			"Digest",
			"Host",
			"Signature",
			"User-Agent",
			"Accept-Encoding",
			"Authorization", // For signed fetches
		},
		ExposedHeaders:   []string{"Content-Type", "Date"},
		AllowCredentials: false, // NEVER true with wildcard origins
		MaxAge:           86400,
	}
}

// GetWebClientCORSConfig returns CORS config for web client API endpoints
func GetWebClientCORSConfig() CORSConfig {
	allowedOrigins := []string{}

	if originsEnv := os.Getenv("ALLOWED_ORIGINS"); originsEnv != "" {
		allowedOrigins = strings.Split(originsEnv, ",")
	}

	// Fallback to domain if no specific origins set
	if len(allowedOrigins) == 0 {
		if domain := os.Getenv("DOMAIN_NAME"); domain != "" {
			allowedOrigins = []string{"https://" + domain}
		}
	}

	// Development fallback
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:3000", "https://localhost:3000"}
	}

	return CORSConfig{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Content-Type",
			"Authorization",
			"X-Requested-With",
			"Accept",
		},
		ExposedHeaders:   []string{"Content-Type", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		AllowCredentials: true,
		MaxAge:           3600,
	}
}