package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func runAuth(argv []string) error {
	if len(argv) == 0 {
		printUsage()
		return nil
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "login":
		return runAuthLogin(argv[1:])
	case "logout":
		return runAuthLogout(argv[1:])
	case "status":
		return runAuthStatus(argv[1:])
	case "whoami":
		return runAuthWhoami(argv[1:])
	case "device":
		return runAuthDevice(argv[1:])
	default:
		if argv[0] != "" && argv[0][0] == '-' {
			printUsage()
			return nil
		}
		return fmt.Errorf("unknown auth command %q", argv[0])
	}
}

type authFlags struct {
	BaseURL     string
	Scopes      string
	SecretFile  string
	Debug       bool
	JSON        bool
	Flow        string
	NoBrowser   bool
	ClientID    string
	DeviceCode  string
	ExpiresIn   int
	PollSeconds int
}

func (a *authFlags) debugf(format string, args ...any) {
	if a == nil || !a.Debug {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}

func runAuthLogin(argv []string) error {
	fs := flag.NewFlagSet("lesser auth login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args authFlags
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("LESSER_BASE_URL"), "instance base url (env: LESSER_BASE_URL)")
	fs.StringVar(&args.Scopes, "scopes", envOrDefault("LESSER_OAUTH_SCOPES", "read write follow push"), "oauth scopes (env: LESSER_OAUTH_SCOPES)")
	fs.StringVar(&args.SecretFile, "secret-file", os.Getenv("LESSER_AUTH_SECRET_FILE"), "path to auth secret file (env: LESSER_AUTH_SECRET_FILE)")
	fs.BoolVar(&args.Debug, "debug", false, "enable debug logging (never prints tokens)")
	fs.BoolVar(&args.JSON, "json", false, "print device login instructions as json (never prints tokens)")
	fs.StringVar(&args.Flow, "flow", envOrDefault("LESSER_OAUTH_FLOW", oauthFlowDevice), "oauth flow: device|loopback (env: LESSER_OAUTH_FLOW)")
	fs.BoolVar(&args.NoBrowser, "no-browser", false, "do not try to open a browser for loopback flow")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	baseURL, err := normalizeBaseURL(args.BaseURL)
	if err != nil {
		return err
	}

	secret, err := readAuthSecret(args.SecretFile)
	if err != nil {
		return err
	}
	key := deriveAuthKey(baseURL, secret)

	flow := strings.ToLower(strings.TrimSpace(args.Flow))
	if flow == "" {
		flow = oauthFlowDevice
	}
	if flow == oauthFlowLoopback {
		if args.JSON {
			return fmt.Errorf("--json is only supported for device flow")
		}
		return runAuthLoopbackLogin(baseURL, key, &args)
	}
	if flow != oauthFlowDevice {
		return fmt.Errorf("unsupported oauth flow %q (expected device or loopback)", flow)
	}

	clientID, err := getOrCreateOAuthClientID(context.Background(), baseURL, args.Scopes, key, &args)
	if err != nil {
		return err
	}

	deviceResp, err := startDeviceAuthorization(context.Background(), baseURL, clientID, args.Scopes)
	if err != nil {
		return err
	}

	if args.JSON {
		out := map[string]any{
			"verification_uri":          deviceResp.VerificationURI,
			"verification_uri_complete": deviceResp.VerificationURIComplete,
			"user_code":                 deviceResp.UserCode,
			"expires_in":                deviceResp.ExpiresIn,
			"interval":                  deviceResp.Interval,
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	fmt.Println("Complete device login in your browser:")
	if strings.TrimSpace(deviceResp.VerificationURIComplete) != "" {
		fmt.Println("  ", deviceResp.VerificationURIComplete)
	} else {
		fmt.Println("  ", deviceResp.VerificationURI)
		fmt.Println("  Code:", deviceResp.UserCode)
	}

	pollInterval := time.Duration(deviceResp.Interval) * time.Second
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}

	tokenResp, err := pollDeviceToken(context.Background(), baseURL, clientID, deviceResp.DeviceCode, pollInterval, time.Duration(deviceResp.ExpiresIn)*time.Second, &args)
	if err != nil {
		return err
	}

	username, scopes, err := resolveViewerAndScopes(context.Background(), baseURL, tokenResp.AccessToken, tokenResp.Scope)
	if err != nil {
		return err
	}

	session := &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     clientID,
		RefreshToken: tokenResp.RefreshToken,
		Username:     username,
		Scopes:       scopes,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := writeAuthSession(baseURL, key, session); err != nil {
		return err
	}

	fmt.Printf("Authenticated as @%s on %s\n", username, baseURL)
	return nil
}

func runAuthLogout(argv []string) error {
	fs := flag.NewFlagSet("lesser auth logout", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var baseURL string
	fs.StringVar(&baseURL, "base-url", os.Getenv("LESSER_BASE_URL"), "instance base url (env: LESSER_BASE_URL)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return err
	}

	removed, err := deleteAuthSession(baseURL)
	if err != nil {
		return err
	}
	if !removed {
		fmt.Printf("No session found for %s\n", baseURL)
		return nil
	}

	fmt.Printf("Logged out from %s\n", baseURL)
	return nil
}

func runAuthStatus(argv []string) error {
	fs := flag.NewFlagSet("lesser auth status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args authFlags
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("LESSER_BASE_URL"), "instance base url (env: LESSER_BASE_URL)")
	fs.StringVar(&args.SecretFile, "secret-file", os.Getenv("LESSER_AUTH_SECRET_FILE"), "path to auth secret file (env: LESSER_AUTH_SECRET_FILE)")
	fs.BoolVar(&args.Debug, "debug", false, "enable debug logging (never prints tokens)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	baseURL, err := normalizeBaseURL(args.BaseURL)
	if err != nil {
		return err
	}

	secret, err := readAuthSecret(args.SecretFile)
	if err != nil {
		return err
	}
	key := deriveAuthKey(baseURL, secret)

	session, err := readAuthSession(baseURL, key)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Not logged in to %s\n", baseURL)
			return nil
		}
		return err
	}

	fmt.Printf("Logged in to %s as @%s\n", baseURL, session.Username)
	if len(session.Scopes) > 0 {
		fmt.Println("Scopes:", strings.Join(session.Scopes, " "))
	}
	return nil
}

func runAuthWhoami(argv []string) error {
	fs := flag.NewFlagSet("lesser auth whoami", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args authFlags
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("LESSER_BASE_URL"), "instance base url (env: LESSER_BASE_URL)")
	fs.StringVar(&args.SecretFile, "secret-file", os.Getenv("LESSER_AUTH_SECRET_FILE"), "path to auth secret file (env: LESSER_AUTH_SECRET_FILE)")
	fs.BoolVar(&args.Debug, "debug", false, "enable debug logging (never prints tokens)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	baseURL, err := normalizeBaseURL(args.BaseURL)
	if err != nil {
		return err
	}

	secret, err := readAuthSecret(args.SecretFile)
	if err != nil {
		return err
	}
	key := deriveAuthKey(baseURL, secret)

	session, err := readAuthSession(baseURL, key)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not logged in to %s (run: lesser auth login --base-url %s)", baseURL, baseURL)
		}
		return err
	}

	tokenResp, err := refreshAccessToken(context.Background(), baseURL, session.ClientID, session.RefreshToken)
	if err != nil {
		return err
	}

	username, scopes, err := resolveViewerAndScopes(context.Background(), baseURL, tokenResp.AccessToken, tokenResp.Scope)
	if err != nil {
		return err
	}

	updated := *session
	updated.Username = username
	updated.Scopes = scopes
	if strings.TrimSpace(tokenResp.RefreshToken) != "" && tokenResp.RefreshToken != session.RefreshToken {
		updated.RefreshToken = tokenResp.RefreshToken
	}
	updated.UpdatedAt = time.Now().UTC()
	if err := writeAuthSession(baseURL, key, &updated); err != nil {
		return err
	}

	fmt.Printf("@%s\n", username)
	return nil
}

func runAuthDevice(argv []string) error {
	if len(argv) == 0 {
		printUsage()
		return nil
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "start":
		return runAuthDeviceStart(argv[1:])
	case "poll":
		return runAuthDevicePoll(argv[1:])
	default:
		return fmt.Errorf("unknown auth device command %q", argv[0])
	}
}

func runAuthDeviceStart(argv []string) error {
	fs := flag.NewFlagSet("lesser auth device start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args authFlags
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("LESSER_BASE_URL"), "instance base url (env: LESSER_BASE_URL)")
	fs.StringVar(&args.Scopes, "scopes", envOrDefault("LESSER_OAUTH_SCOPES", "read write follow push"), "oauth scopes (env: LESSER_OAUTH_SCOPES)")
	fs.StringVar(&args.SecretFile, "secret-file", os.Getenv("LESSER_AUTH_SECRET_FILE"), "path to auth secret file (env: LESSER_AUTH_SECRET_FILE)")
	fs.StringVar(&args.ClientID, "client-id", os.Getenv("LESSER_OAUTH_CLIENT_ID"), "oauth client id (env: LESSER_OAUTH_CLIENT_ID)")
	fs.BoolVar(&args.Debug, "debug", false, "enable debug logging (never prints tokens)")
	fs.BoolVar(&args.JSON, "json", false, "print json output (includes device_code)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	baseURL, err := normalizeBaseURL(args.BaseURL)
	if err != nil {
		return err
	}

	clientID := strings.TrimSpace(args.ClientID)
	if clientID == "" {
		secret, err := readAuthSecret(args.SecretFile)
		if err != nil {
			return err
		}
		key := deriveAuthKey(baseURL, secret)

		clientID, err = getOrCreateOAuthClientID(context.Background(), baseURL, args.Scopes, key, &args)
		if err != nil {
			return err
		}
	}

	deviceResp, err := startDeviceAuthorization(context.Background(), baseURL, clientID, args.Scopes)
	if err != nil {
		return err
	}

	if args.JSON {
		out := map[string]any{
			"base_url":                  baseURL,
			"client_id":                 clientID,
			"device_code":               deviceResp.DeviceCode,
			"user_code":                 deviceResp.UserCode,
			"verification_uri":          deviceResp.VerificationURI,
			"verification_uri_complete": deviceResp.VerificationURIComplete,
			"expires_in":                deviceResp.ExpiresIn,
			"interval":                  deviceResp.Interval,
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	fmt.Println("Complete device login in your browser:")
	if strings.TrimSpace(deviceResp.VerificationURIComplete) != "" {
		fmt.Println("  ", deviceResp.VerificationURIComplete)
	} else {
		fmt.Println("  ", deviceResp.VerificationURI)
		fmt.Println("  Code:", deviceResp.UserCode)
	}
	fmt.Println("Then run: lesser auth device poll --base-url", baseURL, "--client-id", clientID, "--device-code <device_code>")
	return nil
}

func runAuthDevicePoll(argv []string) error {
	fs := flag.NewFlagSet("lesser auth device poll", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args authFlags
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("LESSER_BASE_URL"), "instance base url (env: LESSER_BASE_URL)")
	fs.StringVar(&args.SecretFile, "secret-file", os.Getenv("LESSER_AUTH_SECRET_FILE"), "path to auth secret file (env: LESSER_AUTH_SECRET_FILE)")
	fs.StringVar(&args.ClientID, "client-id", os.Getenv("LESSER_OAUTH_CLIENT_ID"), "oauth client id (env: LESSER_OAUTH_CLIENT_ID)")
	fs.StringVar(&args.DeviceCode, "device-code", os.Getenv("LESSER_OAUTH_DEVICE_CODE"), "device code from start step (env: LESSER_OAUTH_DEVICE_CODE)")
	fs.IntVar(&args.ExpiresIn, "expires-in", 10*60, "device code ttl seconds (default 600)")
	fs.IntVar(&args.PollSeconds, "interval", 10, "poll interval seconds (default 10)")
	fs.BoolVar(&args.Debug, "debug", false, "enable debug logging (never prints tokens)")
	fs.BoolVar(&args.JSON, "json", false, "print json output (never prints tokens)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	baseURL, err := normalizeBaseURL(args.BaseURL)
	if err != nil {
		return err
	}

	clientID := strings.TrimSpace(args.ClientID)
	if err := requireNonEmpty("client-id", clientID); err != nil {
		return err
	}

	deviceCode := strings.TrimSpace(args.DeviceCode)
	if err := requireNonEmpty("device-code", deviceCode); err != nil {
		return err
	}

	secret, err := readAuthSecret(args.SecretFile)
	if err != nil {
		return err
	}
	key := deriveAuthKey(baseURL, secret)

	interval := time.Duration(args.PollSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	tokenResp, err := pollDeviceToken(context.Background(), baseURL, clientID, deviceCode, interval, time.Duration(args.ExpiresIn)*time.Second, &args)
	if err != nil {
		return err
	}

	username, scopes, err := resolveViewerAndScopes(context.Background(), baseURL, tokenResp.AccessToken, tokenResp.Scope)
	if err != nil {
		return err
	}

	session := &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     clientID,
		RefreshToken: tokenResp.RefreshToken,
		Username:     username,
		Scopes:       scopes,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := writeAuthSession(baseURL, key, session); err != nil {
		return err
	}

	if args.JSON {
		out := map[string]any{
			"base_url":  baseURL,
			"client_id": clientID,
			"username":  username,
			"scopes":    scopes,
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	fmt.Printf("Authenticated as @%s on %s\n", username, baseURL)
	return nil
}
