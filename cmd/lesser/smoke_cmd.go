package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

type smokeArgs struct {
	BaseURL        string
	Token          string
	Username       string
	ObjectID       string
	AcceptHeader   string
	TimeoutSeconds int
	Insecure       bool
}

func runSmoke(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("missing smoke command (expected: core|federation)")
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "core":
		return runSmokeCoreFromArgs(argv[1:])
	case "federation":
		return runSmokeFederationFromArgs(argv[1:])
	default:
		return fmt.Errorf("unknown smoke command %q", argv[0])
	}
}

func runSmokeCoreFromArgs(argv []string) error {
	fs := flag.NewFlagSet("lesser smoke core", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args smokeArgs
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("SMOKE_BASE_URL"), "base url (env: SMOKE_BASE_URL)")
	fs.StringVar(&args.Token, "token", os.Getenv("SMOKE_TOKEN"), "auth token (env: SMOKE_TOKEN)")
	fs.IntVar(&args.TimeoutSeconds, "timeout-seconds", 15, "timeout seconds (env: SMOKE_TIMEOUT_SECONDS)")
	fs.BoolVar(&args.Insecure, "insecure", envOrDefault("SMOKE_INSECURE", "0") == "1", "allow insecure TLS")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	return runSmokeCore(args)
}

func runSmokeFederationFromArgs(argv []string) error {
	fs := flag.NewFlagSet("lesser smoke federation", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args smokeArgs
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("SMOKE_BASE_URL"), "base url (env: SMOKE_BASE_URL)")
	fs.StringVar(&args.Username, "username", os.Getenv("SMOKE_USERNAME"), "username (env: SMOKE_USERNAME)")
	fs.StringVar(&args.ObjectID, "object-id", os.Getenv("SMOKE_OBJECT_ID"), "object id (env: SMOKE_OBJECT_ID)")
	fs.StringVar(&args.AcceptHeader, "accept-header", envOrDefault("SMOKE_ACCEPT_HEADER", "application/activity+json"), "accept header (env: SMOKE_ACCEPT_HEADER)")
	fs.IntVar(&args.TimeoutSeconds, "timeout-seconds", 15, "timeout seconds (env: SMOKE_TIMEOUT_SECONDS)")
	fs.BoolVar(&args.Insecure, "insecure", envOrDefault("SMOKE_INSECURE", "0") == "1", "allow insecure TLS")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	return runSmokeFederation(args)
}

func runSmokeCore(args smokeArgs) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	if args.BaseURL == "" {
		return fmt.Errorf("--base-url is required (or set SMOKE_BASE_URL)")
	}

	env := map[string]string{
		"SMOKE_BASE_URL":        args.BaseURL,
		"SMOKE_TIMEOUT_SECONDS": fmt.Sprintf("%d", args.TimeoutSeconds),
	}
	if args.Token != "" {
		env["SMOKE_TOKEN"] = args.Token
	}
	if args.Insecure {
		env["SMOKE_INSECURE"] = "1"
	}

	return runCommand(context.Background(), "bash", []string{"scripts/smoke_core.sh"}, execOptions{
		Dir: repoRoot,
		Env: env,
	})
}

func runSmokeFederation(args smokeArgs) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}

	if args.BaseURL == "" {
		return fmt.Errorf("--base-url is required (or set SMOKE_BASE_URL)")
	}
	if args.Username == "" {
		return fmt.Errorf("--username is required (or set SMOKE_USERNAME)")
	}
	if args.ObjectID == "" {
		return fmt.Errorf("--object-id is required (or set SMOKE_OBJECT_ID)")
	}
	if args.AcceptHeader == "" {
		args.AcceptHeader = "application/activity+json"
	}

	env := map[string]string{
		"SMOKE_BASE_URL":        args.BaseURL,
		"SMOKE_USERNAME":        args.Username,
		"SMOKE_OBJECT_ID":       args.ObjectID,
		"SMOKE_ACCEPT_HEADER":   args.AcceptHeader,
		"SMOKE_TIMEOUT_SECONDS": fmt.Sprintf("%d", args.TimeoutSeconds),
	}
	if args.Insecure {
		env["SMOKE_INSECURE"] = "1"
	}

	return runCommand(context.Background(), "bash", []string{"scripts/smoke_federation.sh"}, execOptions{
		Dir: repoRoot,
		Env: env,
	})
}
