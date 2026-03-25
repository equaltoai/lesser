package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(runCLI(os.Args, os.Stderr))
}

var (
	runUpFn                   = runUp
	runDownFn                 = runDown
	runClientDeployFn         = runClientDeploy
	runInitAdminFn            = runInitAdmin
	runBuildFn                = runBuild
	runGenerateFn             = runGenerate
	runVerifyFn               = runVerify
	runTestFn                 = runTest
	runCoverageFn             = runCoverage
	runDevFn                  = runDev
	runFmtFn                  = runFmt
	runLintFn                 = runLint
	runSecScanFn              = runSecScan
	runVulnCheckFn            = runVulnCheck
	runGqlgenFn               = runGqlgen
	runTidyFn                 = runTidy
	runSchemaFn               = runSchema
	runExportSchemaFn         = runExportSchema
	runLogsFn                 = runLogs
	runMetricsFn              = runMetrics
	runErrorsFn               = runErrors
	runDashboardFn            = runDashboard
	runSmokeFn                = runSmoke
	runAuthFn                 = runAuth
	runAPIFn                  = runAPI
	runSoulFn                 = runSoul
	runMigrateUserKeysFn      = runMigrateUserKeys
	runMigrateConversationsFn = runMigrateConversations
	runMigrateNumericIDsFn    = runMigrateNumericIDs
)

func runCLI(args []string, stderr io.Writer) int {
	applyCLIDefaultEnv()

	if len(args) < 2 {
		printUsageTo(stderr)
		return 2
	}

	cmd := args[1]
	switch cmd {
	case "version", "--version":
		printVersionTo(stderr)
		return 0
	case "client":
		return handleClientCommand(args, stderr)
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsageTo(stderr)
		return 0
	default:
		if runner, ok := commandRunner(cmd); ok {
			return exitCodeFromErr(runner(args[2:]), stderr)
		}

		printUsageTo(stderr)
		_, _ = fmt.Fprintln(stderr, "\nUnknown command:", cmd)
		return 2
	}
}

func commandRunner(cmd string) (func([]string) error, bool) {
	runners := map[string]func([]string) error{
		"up":                    func(argv []string) error { return runUpFn(argv) },
		"down":                  func(argv []string) error { return runDownFn(argv) },
		"destroy":               func(argv []string) error { return runDownFn(argv) },
		"init-admin":            func(argv []string) error { return runInitAdminFn(argv) },
		"build":                 func(argv []string) error { return runBuildFn(argv) },
		"generate":              func(argv []string) error { return runGenerateFn(argv) },
		"verify":                func(argv []string) error { return runVerifyFn(argv) },
		"test":                  func(argv []string) error { return runTestFn(argv) },
		"coverage":              func(argv []string) error { return runCoverageFn(argv) },
		valueDev:                func(argv []string) error { return runDevFn(argv) },
		"fmt":                   func(argv []string) error { return runFmtFn(argv) },
		"lint":                  func(argv []string) error { return runLintFn(argv) },
		"sec-scan":              func(argv []string) error { return runSecScanFn(argv) },
		"vuln-check":            func(argv []string) error { return runVulnCheckFn(argv) },
		"gqlgen":                func(argv []string) error { return runGqlgenFn(argv) },
		"tidy":                  func(argv []string) error { return runTidyFn(argv) },
		valueSchema:             func(argv []string) error { return runSchemaFn(argv) },
		"export-schema":         func(argv []string) error { return runExportSchemaFn(argv) },
		"logs":                  func(argv []string) error { return runLogsFn(argv) },
		"metrics":               func(argv []string) error { return runMetricsFn(argv) },
		"errors":                func(argv []string) error { return runErrorsFn(argv) },
		"dashboard":             func(argv []string) error { return runDashboardFn(argv) },
		"smoke":                 func(argv []string) error { return runSmokeFn(argv) },
		"auth":                  func(argv []string) error { return runAuthFn(argv) },
		"api":                   func(argv []string) error { return runAPIFn(argv) },
		"soul":                  func(argv []string) error { return runSoulFn(argv) },
		"migrate-user-keys":     func(argv []string) error { return runMigrateUserKeysFn(argv) },
		"migrate-conversations": func(argv []string) error { return runMigrateConversationsFn(argv) },
		"migrate-numeric-ids":   func(argv []string) error { return runMigrateNumericIDsFn(argv) },
	}

	runner, ok := runners[cmd]
	return runner, ok
}

func handleClientCommand(args []string, stderr io.Writer) int {
	if len(args) < 3 {
		printUsageTo(stderr)
		return 2
	}

	switch args[2] {
	case "deploy":
		return exitCodeFromErr(runClientDeployFn(args[3:]), stderr)
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsageTo(stderr)
		return 0
	default:
		printUsageTo(stderr)
		_, _ = fmt.Fprintln(stderr, "\nUnknown client command:", args[2])
		return 2
	}
}

func printUsage() {
	printUsageTo(os.Stderr)
}

func printUsageTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  lesser version | lesser --version")
	_, _ = fmt.Fprintln(w, "  lesser up --app <slug> --base-domain <example.com> [--aws-profile <profile>] [--stage dev|staging|live] [--provisioning-input <path>] [--bootstrap-wallet-address <0x...>] [--with-staging] [--out <path>] [--rebuild-lambdas]")
	_, _ = fmt.Fprintln(w, "  lesser init-admin --app <slug> --base-domain <example.com> [--aws-profile <profile>] --stage dev|staging|live [--provisioning-input <path>] --wallet-address <0x...> --signature <0x...> [--message <string> | --message-file <path>] [--username <username>] [--chain-id <n>]")
	_, _ = fmt.Fprintln(w, "  lesser down --app <slug> --base-domain <example.com> --aws-profile <profile> [--state <path>] [--purge-artifacts]")
	_, _ = fmt.Fprintln(w, "  lesser client deploy --app <slug> --base-domain <example.com> --aws-profile <profile> --dist <dir> [--stage dev|live|staging|both|all] [--state <path>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "  lesser build [--rebuild-lambdas]                # rebuild deployment artifacts (lambdas, auth-ui, go build)")
	_, _ = fmt.Fprintln(w, "  lesser build lambdas [--rebuild]               # (re)build bin/*.zip")
	_, _ = fmt.Fprintln(w, "  lesser build lambda <name>                     # build a single bin/<name>.zip")
	_, _ = fmt.Fprintln(w, "  lesser generate openapi|graphql-coverage|inventory|schema")
	_, _ = fmt.Fprintln(w, "  lesser schema [--out <path>] | lesser export-schema")
	_, _ = fmt.Fprintln(w, "  lesser verify [all|ci|docs|ai-training|schema|audit|supply-chain|graphql-coverage|openapi|inventory|lambda-set|unit|smoke|cdk] [--smoke] [--cdk]")
	_, _ = fmt.Fprintln(w, "  lesser test [all|unit|integration|race]")
	_, _ = fmt.Fprintln(w, "  lesser test coverage [--scope all|pkg] [--exclude-generated=true|false] [--include-testing] [--include-tools]")
	_, _ = fmt.Fprintln(w, "  lesser coverage scoreboard [--profile <path>] [--mode package|file] [--package <prefix>] [--top <n>] [--min-total <pct>] [--exclude-generated=true|false]")
	_, _ = fmt.Fprintln(w, "  lesser dev [init|dynamodb|seed-and-validate]   # local development")
	_, _ = fmt.Fprintln(w, "  lesser fmt | lesser lint [--fix] | lesser tidy")
	_, _ = fmt.Fprintln(w, "  lesser sec-scan | lesser vuln-check | lesser gqlgen")
	_, _ = fmt.Fprintln(w, "  lesser logs --app <slug> --function <name> [--env dev|staging|live] [--aws-profile <profile>]")
	_, _ = fmt.Fprintln(w, "  lesser metrics|errors|dashboard --app <slug> [--env dev|staging|live] [--aws-profile <profile>] [--region <aws-region>]")
	_, _ = fmt.Fprintln(w, "  lesser smoke core|federation [flags...]")
	_, _ = fmt.Fprintln(w, "  lesser auth login|logout|status|whoami|device [flags...]")
	_, _ = fmt.Fprintln(w, "  lesser api request --method GET --path /api/v1/accounts/verify_credentials [flags...]")
	_, _ = fmt.Fprintln(w, "  lesser soul ens set|preview|publish [flags...]")
	_, _ = fmt.Fprintln(w, "  lesser migrate-user-keys [--app <slug>] [--env dev|staging|live] [--aws-profile <profile>] [--table <name>] [--limit <n>] [--apply]")
	_, _ = fmt.Fprintln(w, "  lesser migrate-conversations [--app <slug>] [--env dev|staging|live] [--aws-profile <profile>] [--table <name>] [--limit <n>] [--apply]")
	_, _ = fmt.Fprintln(w, "  lesser migrate-numeric-ids [--app <slug>] [--env dev|staging|live] [--aws-profile <profile>] [--table <name>] [--limit <n>] [--apply]")
}

func exitCodeFromErr(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "Error:", err)
	return 1
}
