package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	theorydb "github.com/theory-cloud/tabletheory/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type stubAccountHydrationVerifier struct {
	getUserFn       func(context.Context, string) (*storage.User, error)
	getAccountFn    func(context.Context, string) (*storage.Account, error)
	getGovernanceFn func(context.Context, string) (*storage.AgentGovernanceState, error)
}

func (s stubAccountHydrationVerifier) GetUser(ctx context.Context, username string) (*storage.User, error) {
	return s.getUserFn(ctx, username)
}

func (s stubAccountHydrationVerifier) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	return s.getAccountFn(ctx, username)
}

func (s stubAccountHydrationVerifier) GetAgentGovernanceState(ctx context.Context, username string) (*storage.AgentGovernanceState, error) {
	return s.getGovernanceFn(ctx, username)
}

func captureAccountHydrationStdout(t *testing.T, fn func()) string {
	t.Helper()

	previousStdout := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = previousStdout
	})

	fn()

	require.NoError(t, writer.Close())
	out, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return string(out)
}

func TestParseAccountHydrationUsernames(t *testing.T) {
	require.Nil(t, parseAccountHydrationUsernames(""))
	require.Equal(t,
		[]string{"arch", "medic", "pilot"},
		parseAccountHydrationUsernames(" arch,medic,arch,, pilot "),
	)
}

func TestLoadAccountHydrationFixtureUsernames(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fixtures.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"medic":{},"arch":{}," pilot ":{},"":{}}`), 0o644))

		got, err := loadAccountHydrationFixtureUsernames(path)
		require.NoError(t, err)
		require.Equal(t, []string{"arch", "medic", "pilot"}, got)
	})

	t.Run("read error", func(t *testing.T) {
		_, err := loadAccountHydrationFixtureUsernames(filepath.Join(t.TempDir(), "missing.json"))
		require.Error(t, err)
	})

	t.Run("parse error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o644))

		_, err := loadAccountHydrationFixtureUsernames(path)
		require.Error(t, err)
	})

	t.Run("empty fixture errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"":{}}`), 0o644))

		_, err := loadAccountHydrationFixtureUsernames(path)
		require.Error(t, err)
	})
}

func TestResolveAccountHydrationUsernames(t *testing.T) {
	t.Run("uses explicit csv", func(t *testing.T) {
		got, err := resolveAccountHydrationUsernames("medic,arch")
		require.NoError(t, err)
		require.Equal(t, []string{"medic", "arch"}, got)
	})

	t.Run("uses fixture from repo root", func(t *testing.T) {
		previousRepoRoot := findRepoRootFn
		t.Cleanup(func() { findRepoRootFn = previousRepoRoot })

		repoRoot := t.TempDir()
		fixtureDir := filepath.Join(repoRoot, "testdata", "account_hydration")
		require.NoError(t, os.MkdirAll(fixtureDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(fixtureDir, "live_agents.json"),
			[]byte(`{"pilot":{},"arch":{}}`),
			0o644,
		))
		findRepoRootFn = func() (string, error) { return repoRoot, nil }

		got, err := resolveAccountHydrationUsernames("")
		require.NoError(t, err)
		require.Equal(t, []string{"arch", "pilot"}, got)
	})
}

func TestExecuteAccountHydrationVerification(t *testing.T) {
	ctx := context.Background()
	actor := &storage.Account{User: &storage.User{Username: "arch"}, Actor: &activitypub.Actor{}}

	t.Run("requires verifier", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, nil, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("requires usernames", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{}, nil)
		require.Error(t, err)
	})

	t.Run("get user error", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(context.Context, string) (*storage.User, error) { return nil, errors.New("boom") },
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("nil user errors", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(context.Context, string) (*storage.User, error) { return nil, nil },
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("get account error", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(context.Context, string) (*storage.User, error) { return &storage.User{Username: "arch"}, nil },
			getAccountFn: func(context.Context, string) (*storage.Account, error) {
				return nil, errors.New("boom")
			},
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("nil account user errors", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn:    func(context.Context, string) (*storage.User, error) { return &storage.User{Username: "arch"}, nil },
			getAccountFn: func(context.Context, string) (*storage.Account, error) { return &storage.Account{}, nil },
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("username mismatch errors", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(context.Context, string) (*storage.User, error) { return &storage.User{Username: "arch"}, nil },
			getAccountFn: func(context.Context, string) (*storage.Account, error) {
				return &storage.Account{User: &storage.User{Username: "pilot"}, Actor: &activitypub.Actor{}}, nil
			},
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("missing actor errors", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(context.Context, string) (*storage.User, error) { return &storage.User{Username: "arch"}, nil },
			getAccountFn: func(context.Context, string) (*storage.Account, error) {
				return &storage.Account{User: &storage.User{Username: "arch"}}, nil
			},
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("governance error", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(context.Context, string) (*storage.User, error) {
				return &storage.User{Username: "arch", IsAgent: true}, nil
			},
			getAccountFn: func(context.Context, string) (*storage.Account, error) { return actor, nil },
			getGovernanceFn: func(context.Context, string) (*storage.AgentGovernanceState, error) {
				return nil, errors.New("boom")
			},
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("nil governance errors", func(t *testing.T) {
		_, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(context.Context, string) (*storage.User, error) {
				return &storage.User{Username: "arch", IsAgent: true}, nil
			},
			getAccountFn:    func(context.Context, string) (*storage.Account, error) { return actor, nil },
			getGovernanceFn: func(context.Context, string) (*storage.AgentGovernanceState, error) { return nil, nil },
		}, []string{"arch"})
		require.Error(t, err)
	})

	t.Run("success counts checked users and agent rows", func(t *testing.T) {
		summary, err := executeAccountHydrationVerification(ctx, stubAccountHydrationVerifier{
			getUserFn: func(_ context.Context, username string) (*storage.User, error) {
				return &storage.User{Username: username, IsAgent: username == "arch"}, nil
			},
			getAccountFn: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{User: &storage.User{Username: username}, Actor: &activitypub.Actor{}}, nil
			},
			getGovernanceFn: func(context.Context, string) (*storage.AgentGovernanceState, error) {
				return &storage.AgentGovernanceState{Username: "arch"}, nil
			},
		}, []string{"arch", "alice"})
		require.NoError(t, err)
		require.Equal(t, 2, summary.Checked)
		require.Equal(t, 1, summary.AgentRows)
		require.Equal(t, []string{"arch", "alice"}, summary.ResolvedNames)
	})
}

func TestResolveAccountHydrationDomain(t *testing.T) {
	require.Equal(t, "", resolveAccountHydrationDomain("dev", ""))
	require.NotEmpty(t, resolveAccountHydrationDomain("dev", "example.com"))
}

func TestRunVerifyAccountHydration(t *testing.T) {
	previousLoadAWS := loadAWSConfigForCLIFn
	previousNewDB := tabletheoryNewFn
	previousNewVerifier := newAccountHydrationVerifierFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWS
		tabletheoryNewFn = previousNewDB
		newAccountHydrationVerifierFn = previousNewVerifier
	})

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "Sim", nil
	}

	mockDB := new(dynamormmocks.MockDB)
	mockDB.On("Close").Return(nil).Maybe()
	tabletheoryNewFn = func(cfg session.Config) (theorydb.DB, error) {
		require.Equal(t, "us-east-1", cfg.Region)
		return mockDB, nil
	}

	newAccountHydrationVerifierFn = func(_ theorydb.DB, tableName string, domain string) accountHydrationVerifier {
		require.Equal(t, "test-table", tableName)
		require.NotEmpty(t, domain)
		return stubAccountHydrationVerifier{
			getUserFn: func(_ context.Context, username string) (*storage.User, error) {
				return &storage.User{Username: username, IsAgent: true}, nil
			},
			getAccountFn: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{User: &storage.User{Username: username}, Actor: &activitypub.Actor{}}, nil
			},
			getGovernanceFn: func(_ context.Context, username string) (*storage.AgentGovernanceState, error) {
				return &storage.AgentGovernanceState{Username: username}, nil
			},
		}
	}

	output := captureAccountHydrationStdout(t, func() {
		require.NoError(t, runVerifyAccountHydration([]string{
			"--table", "test-table",
			"--usernames", "arch,medic",
			"--aws-profile", "Sim",
			"--base-domain", "example.com",
			"--env", "dev",
			"--app", "simulacrum",
		}))
	})

	require.Contains(t, output, "verify account-hydration complete")
	require.Contains(t, output, "table: test-table")
	require.Contains(t, output, "env: dev")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "checked: 2")
	require.Contains(t, output, "agent_rows: 2")
	require.Contains(t, output, "arch")
	require.Contains(t, output, "medic")
}

func TestRunVerifyDispatchesAccountHydration(t *testing.T) {
	previousLoadAWS := loadAWSConfigForCLIFn
	previousNewDB := tabletheoryNewFn
	previousNewVerifier := newAccountHydrationVerifierFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWS
		tabletheoryNewFn = previousNewDB
		newAccountHydrationVerifierFn = previousNewVerifier
	})

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "", nil
	}

	mockDB := new(dynamormmocks.MockDB)
	mockDB.On("Close").Return(nil).Maybe()
	tabletheoryNewFn = func(session.Config) (theorydb.DB, error) {
		return mockDB, nil
	}

	newAccountHydrationVerifierFn = func(theorydb.DB, string, string) accountHydrationVerifier {
		return stubAccountHydrationVerifier{
			getUserFn: func(_ context.Context, username string) (*storage.User, error) {
				return &storage.User{Username: username}, nil
			},
			getAccountFn: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{User: &storage.User{Username: username}, Actor: &activitypub.Actor{}}, nil
			},
			getGovernanceFn: func(_ context.Context, username string) (*storage.AgentGovernanceState, error) {
				return &storage.AgentGovernanceState{Username: username}, nil
			},
		}
	}

	output := captureAccountHydrationStdout(t, func() {
		require.NoError(t, runVerify([]string{"account-hydration", "--table", "test-table", "--usernames", "arch"}))
	})
	require.Contains(t, output, "checked: 1")
}
