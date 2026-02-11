package main

const (
	oauthErrorInvalidGrant           = "invalid_grant"
	oauthErrorDescriptionDefault     = "oauth error"
	oauthErrorRefreshReauthRequired  = "refresh token invalid; re-auth required"
	oauthErrorDeviceAuthInvalid      = "device authorization invalid"
	oauthErrorDeviceAuthDenied       = "device authorization denied"
	oauthErrorDeviceCodeExpired      = "device code expired"
	oauthErrorDeviceAuthorizationTTL = "device authorization timed out"

	oauthFlowDevice   = "device"
	oauthFlowLoopback = "loopback"
)
