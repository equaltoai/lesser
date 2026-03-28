package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunCLI_DispatchAndExitCodes(t *testing.T) {
	t.Run("no args prints usage and returns 2", func(t *testing.T) {
		var buf bytes.Buffer
		code := runCLI([]string{"lesser"}, &buf)
		require.Equal(t, 2, code)
		require.Contains(t, buf.String(), "Usage:")
	})

	t.Run("help prints usage and returns 0", func(t *testing.T) {
		var buf bytes.Buffer
		code := runCLI([]string{"lesser", helpCommand}, &buf)
		require.Equal(t, 0, code)
		require.Contains(t, buf.String(), "Usage:")
	})

	t.Run("version prints version and returns 0", func(t *testing.T) {
		for _, arg := range []string{"version", "--version"} {
			arg := arg
			t.Run(arg, func(t *testing.T) {
				var buf bytes.Buffer
				code := runCLI([]string{"lesser", arg}, &buf)
				require.Equal(t, 0, code)
				require.Contains(t, buf.String(), "lesser")
				require.NotContains(t, buf.String(), "Usage:")
			})
		}
	})

	t.Run("unknown command returns 2", func(t *testing.T) {
		var buf bytes.Buffer
		code := runCLI([]string{"lesser", "nope"}, &buf)
		require.Equal(t, 2, code)
		require.Contains(t, buf.String(), "Unknown command:")
	})

	t.Run("client requires subcommand", func(t *testing.T) {
		var buf bytes.Buffer
		code := runCLI([]string{"lesser", "client"}, &buf)
		require.Equal(t, 2, code)
		require.Contains(t, buf.String(), "Usage:")
	})

	t.Run("client deploy uses runner and returns 1 on error", func(t *testing.T) {
		previous := runClientDeployFn
		t.Cleanup(func() { runClientDeployFn = previous })

		var called bool
		runClientDeployFn = func(argv []string) error {
			called = true
			require.Equal(t, []string{"--dist", "x"}, argv)
			return errors.New("boom")
		}

		var buf bytes.Buffer
		code := runCLI([]string{"lesser", "client", "deploy", "--dist", "x"}, &buf)
		require.True(t, called)
		require.Equal(t, 1, code)
		require.Contains(t, buf.String(), "Error:")
	})

	t.Run("dispatches simple commands", func(t *testing.T) {
		previousBuild := runBuildFn
		previousVerify := runVerifyFn
		previousDev := runDevFn
		t.Cleanup(func() {
			runBuildFn = previousBuild
			runVerifyFn = previousVerify
			runDevFn = previousDev
		})

		runBuildCalled := 0
		runVerifyCalled := 0
		runDevCalled := 0

		runBuildFn = func(argv []string) error {
			runBuildCalled++
			require.Equal(t, []string{"lambdas"}, argv)
			return nil
		}
		runVerifyFn = func(argv []string) error {
			runVerifyCalled++
			require.Equal(t, []string{"unit"}, argv)
			return nil
		}
		runDevFn = func(argv []string) error {
			runDevCalled++
			require.Equal(t, []string{"init"}, argv)
			return nil
		}

		var buf bytes.Buffer
		require.Equal(t, 0, runCLI([]string{"lesser", "build", "lambdas"}, &buf))
		require.Equal(t, 0, runCLI([]string{"lesser", "verify", "unit"}, &buf))
		require.Equal(t, 0, runCLI([]string{"lesser", valueDev, "init"}, &buf))
		require.Equal(t, 1, runBuildCalled)
		require.Equal(t, 1, runVerifyCalled)
		require.Equal(t, 1, runDevCalled)
	})
}

func TestRunCLI_DispatchesAllCommands(t *testing.T) {
	prevUp := runUpFn
	prevDown := runDownFn
	prevClientDeploy := runClientDeployFn
	prevInitAdmin := runInitAdminFn
	prevBuild := runBuildFn
	prevGenerate := runGenerateFn
	prevVerify := runVerifyFn
	prevTest := runTestFn
	prevCoverage := runCoverageFn
	prevDev := runDevFn
	prevFmt := runFmtFn
	prevLint := runLintFn
	prevSecScan := runSecScanFn
	prevVulnCheck := runVulnCheckFn
	prevGqlgen := runGqlgenFn
	prevTidy := runTidyFn
	prevSchema := runSchemaFn
	prevExportSchema := runExportSchemaFn
	prevLogs := runLogsFn
	prevMetrics := runMetricsFn
	prevErrors := runErrorsFn
	prevDashboard := runDashboardFn
	prevSmoke := runSmokeFn
	prevAuth := runAuthFn
	prevAPI := runAPIFn
	prevSoul := runSoulFn
	prevMigrateUserKeys := runMigrateUserKeysFn
	prevMigrateConversations := runMigrateConversationsFn
	prevMigrateConversationMetadata := runMigrateConversationMetadataFn
	prevMigrateConversationParticipantSnapshots := runMigrateConversationParticipantSnapshotsFn
	prevMigrateAgentGovernanceState := runMigrateAgentGovernanceStateFn
	prevMigrateDirectMessageState := runMigrateDirectMessageStateFn
	prevMigrateNumericIDs := runMigrateNumericIDsFn
	prevMigrateMCPAuthCutover := runMigrateMCPAuthCutoverFn
	t.Cleanup(func() {
		runUpFn = prevUp
		runDownFn = prevDown
		runClientDeployFn = prevClientDeploy
		runInitAdminFn = prevInitAdmin
		runBuildFn = prevBuild
		runGenerateFn = prevGenerate
		runVerifyFn = prevVerify
		runTestFn = prevTest
		runCoverageFn = prevCoverage
		runDevFn = prevDev
		runFmtFn = prevFmt
		runLintFn = prevLint
		runSecScanFn = prevSecScan
		runVulnCheckFn = prevVulnCheck
		runGqlgenFn = prevGqlgen
		runTidyFn = prevTidy
		runSchemaFn = prevSchema
		runExportSchemaFn = prevExportSchema
		runLogsFn = prevLogs
		runMetricsFn = prevMetrics
		runErrorsFn = prevErrors
		runDashboardFn = prevDashboard
		runSmokeFn = prevSmoke
		runAuthFn = prevAuth
		runAPIFn = prevAPI
		runSoulFn = prevSoul
		runMigrateUserKeysFn = prevMigrateUserKeys
		runMigrateConversationsFn = prevMigrateConversations
		runMigrateConversationMetadataFn = prevMigrateConversationMetadata
		runMigrateConversationParticipantSnapshotsFn = prevMigrateConversationParticipantSnapshots
		runMigrateAgentGovernanceStateFn = prevMigrateAgentGovernanceState
		runMigrateDirectMessageStateFn = prevMigrateDirectMessageState
		runMigrateNumericIDsFn = prevMigrateNumericIDs
		runMigrateMCPAuthCutoverFn = prevMigrateMCPAuthCutover
	})

	type call struct {
		argv []string
	}
	calls := map[string]call{}
	stub := func(name string) func([]string) error {
		return func(argv []string) error {
			calls[name] = call{argv: append([]string(nil), argv...)}
			return nil
		}
	}

	runUpFn = stub("up")
	runDownFn = stub("down")
	runClientDeployFn = stub("client deploy")
	runInitAdminFn = stub("init-admin")
	runBuildFn = stub("build")
	runGenerateFn = stub("generate")
	runVerifyFn = stub("verify")
	runTestFn = stub("test")
	runCoverageFn = stub("coverage")
	runDevFn = stub(valueDev)
	runFmtFn = stub("fmt")
	runLintFn = stub("lint")
	runSecScanFn = stub("sec-scan")
	runVulnCheckFn = stub("vuln-check")
	runGqlgenFn = stub("gqlgen")
	runTidyFn = stub("tidy")
	runSchemaFn = stub(valueSchema)
	runExportSchemaFn = stub("export-schema")
	runLogsFn = stub("logs")
	runMetricsFn = stub("metrics")
	runErrorsFn = stub("errors")
	runDashboardFn = stub("dashboard")
	runSmokeFn = stub("smoke")
	runAuthFn = stub("auth")
	runAPIFn = stub("api")
	runSoulFn = stub("soul")
	runMigrateUserKeysFn = stub("migrate-user-keys")
	runMigrateConversationsFn = stub("migrate-conversations")
	runMigrateConversationMetadataFn = stub("migrate-conversation-metadata")
	runMigrateConversationParticipantSnapshotsFn = stub("migrate-conversation-participant-snapshots")
	runMigrateAgentGovernanceStateFn = stub("migrate-agent-governance-state")
	runMigrateDirectMessageStateFn = stub("migrate-direct-message-state")
	runMigrateNumericIDsFn = stub("migrate-numeric-ids")
	runMigrateMCPAuthCutoverFn = stub("migrate-mcp-auth-cutover")

	var buf bytes.Buffer
	require.Equal(t, 0, runCLI([]string{"lesser", helpFlagShort}, &buf))
	require.Equal(t, 0, runCLI([]string{"lesser", helpFlagLong}, &buf))

	require.Equal(t, 0, runCLI([]string{"lesser", "up", "--arg"}, &buf))
	require.Equal(t, []string{"--arg"}, calls["up"].argv)
	require.Equal(t, 0, runCLI([]string{"lesser", "down", "--app", "x"}, &buf))
	require.Equal(t, []string{"--app", "x"}, calls["down"].argv)
	require.Equal(t, 0, runCLI([]string{"lesser", "destroy", "--app", "x"}, &buf))
	require.Equal(t, []string{"--app", "x"}, calls["down"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "init-admin", "--arg"}, &buf))
	require.Equal(t, []string{"--arg"}, calls["init-admin"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "client", helpFlagLong}, &buf))
	require.Equal(t, 0, runCLI([]string{"lesser", "client", helpCommand}, &buf))

	require.Equal(t, 2, runCLI([]string{"lesser", "client", "wat"}, &buf))
	require.Contains(t, buf.String(), "Unknown client command:")

	require.Equal(t, 0, runCLI([]string{"lesser", "client", "deploy", "--dist", "x"}, &buf))
	require.Equal(t, []string{"--dist", "x"}, calls["client deploy"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "build", "lambdas"}, &buf))
	require.Equal(t, []string{"lambdas"}, calls["build"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "generate", "openapi"}, &buf))
	require.Equal(t, []string{"openapi"}, calls["generate"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "verify", "unit"}, &buf))
	require.Equal(t, []string{"unit"}, calls["verify"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "test", "unit"}, &buf))
	require.Equal(t, []string{"unit"}, calls["test"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "coverage", "scoreboard"}, &buf))
	require.Equal(t, []string{"scoreboard"}, calls["coverage"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", valueDev, "init"}, &buf))
	require.Equal(t, []string{"init"}, calls[valueDev].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "fmt"}, &buf))
	require.Empty(t, calls["fmt"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "lint"}, &buf))
	require.Empty(t, calls["lint"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "sec-scan"}, &buf))
	require.Empty(t, calls["sec-scan"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "vuln-check"}, &buf))
	require.Empty(t, calls["vuln-check"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "gqlgen"}, &buf))
	require.Empty(t, calls["gqlgen"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "tidy"}, &buf))
	require.Empty(t, calls["tidy"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", valueSchema, "--out", "x"}, &buf))
	require.Equal(t, []string{"--out", "x"}, calls[valueSchema].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "export-schema"}, &buf))
	require.Empty(t, calls["export-schema"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "logs", "--app", "x"}, &buf))
	require.Equal(t, []string{"--app", "x"}, calls["logs"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "metrics", "--app", "x"}, &buf))
	require.Equal(t, []string{"--app", "x"}, calls["metrics"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "errors", "--app", "x"}, &buf))
	require.Equal(t, []string{"--app", "x"}, calls["errors"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "dashboard", "--app", "x"}, &buf))
	require.Equal(t, []string{"--app", "x"}, calls["dashboard"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "smoke", "core"}, &buf))
	require.Equal(t, []string{"core"}, calls["smoke"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "auth", "status", "--base-url", "https://example.com"}, &buf))
	require.Equal(t, []string{"status", "--base-url", "https://example.com"}, calls["auth"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "api", "request", "--method", "GET", "--path", "/api/v1/accounts/verify_credentials"}, &buf))
	require.Equal(t, []string{"request", "--method", "GET", "--path", "/api/v1/accounts/verify_credentials"}, calls["api"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "soul", "ens", "preview", "--agent-id", "0xabc"}, &buf))
	require.Equal(t, []string{"ens", "preview", "--agent-id", "0xabc"}, calls["soul"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-user-keys", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-user-keys"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-conversations", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-conversations"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-conversation-metadata", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-conversation-metadata"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-conversation-participant-snapshots", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-conversation-participant-snapshots"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-agent-governance-state", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-agent-governance-state"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-direct-message-state", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-direct-message-state"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-numeric-ids", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-numeric-ids"].argv)

	require.Equal(t, 0, runCLI([]string{"lesser", "migrate-mcp-auth-cutover", "--env", "dev"}, &buf))
	require.Equal(t, []string{"--env", "dev"}, calls["migrate-mcp-auth-cutover"].argv)
}

func TestExitCodeFromErr(t *testing.T) {
	var buf bytes.Buffer
	require.Equal(t, 0, exitCodeFromErr(nil, &buf))

	require.Equal(t, 1, exitCodeFromErr(errors.New("boom"), &buf))
	require.Contains(t, buf.String(), "Error: boom")
}
