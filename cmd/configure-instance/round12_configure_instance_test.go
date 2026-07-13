package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type fakeInstanceRepo struct {
	getRulesCalls int
	setRulesCalls int
	getDescCalls  int
	setDescCalls  int

	rules []storage.InstanceRule
	desc  string
	when  time.Time

	getRulesErr error
	setRulesErr error
	getDescErr  error
	setDescErr  error
}

func (f *fakeInstanceRepo) GetInstanceRules(context.Context) ([]storage.InstanceRule, error) {
	f.getRulesCalls++
	if f.getRulesErr != nil {
		return nil, f.getRulesErr
	}
	return f.rules, nil
}

func (f *fakeInstanceRepo) SetInstanceRules(_ context.Context, rules []storage.InstanceRule) error {
	f.setRulesCalls++
	f.rules = rules
	return f.setRulesErr
}

func (f *fakeInstanceRepo) GetExtendedDescription(context.Context) (string, time.Time, error) {
	f.getDescCalls++
	if f.getDescErr != nil {
		return "", time.Time{}, f.getDescErr
	}
	return f.desc, f.when, nil
}

func (f *fakeInstanceRepo) SetExtendedDescription(_ context.Context, description string) error {
	f.setDescCalls++
	f.desc = description
	return f.setDescErr
}

type fakePushRepo struct {
	getCalls int
	setCalls int

	keys *storage.VAPIDKeys

	getErr error
	setErr error
}

func (f *fakePushRepo) GetVAPIDKeys(context.Context) (*storage.VAPIDKeys, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.keys, nil
}

func (f *fakePushRepo) SetVAPIDKeys(_ context.Context, keys *storage.VAPIDKeys) error {
	f.setCalls++
	f.keys = keys
	return f.setErr
}

type fakeRepos struct {
	instance *fakeInstanceRepo
	push     *fakePushRepo
}

func (f *fakeRepos) Instance() instanceRepository {
	return f.instance
}

func (f *fakeRepos) PushSubscription() pushSubscriptionRepository {
	return f.push
}

func TestParseRules_Round12(t *testing.T) {
	rules := parseRules(" be nice , no spam ")
	require.Len(t, rules, 2)
	require.Equal(t, "1", rules[0].ID)
	require.Equal(t, "2", rules[1].ID)
	require.NotEmpty(t, rules[0].Text)
	require.NotEmpty(t, rules[1].Text)
}

func TestHasAnyAction_Round12(t *testing.T) {
	require.False(t, hasAnyAction(configFlags{}))
	require.True(t, hasAnyAction(configFlags{showConfig: true}))
	require.True(t, hasAnyAction(configFlags{generateVAPID: true}))
	require.True(t, hasAnyAction(configFlags{setRules: "x"}))
	require.True(t, hasAnyAction(configFlags{setDescription: "y"}))
}

func TestRunConfigureInstance_Branches_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origDB := getDynamormClientFn
	origFactory := newRepositoryFactoryFn
	origCfg := getConfigFn
	origScan := scanlnFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		getDynamormClientFn = origDB
		newRepositoryFactoryFn = origFactory
		getConfigFn = origCfg
		scanlnFn = origScan
	})

	instanceRepo := &fakeInstanceRepo{
		rules: []storage.InstanceRule{{ID: "1", Text: "rule"}},
		desc:  "<p>hi</p>",
		when:  time.Now().UTC(),
	}
	pushRepo := &fakePushRepo{
		keys: &storage.VAPIDKeys{PublicKey: "pub", Subject: "subj", CreatedAt: time.Now().UTC()},
	}
	repos := &fakeRepos{instance: instanceRepo, push: pushRepo}

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{Config: &config.Config{DynamoTableName: "table"}, Logger: zap.NewNop()}
	}
	getDynamormClientFn = func(context.Context) (dynamormCore.DB, error) { return nil, nil }
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (repositoriesProvider, error) { return repos, nil }
	getConfigFn = func() *config.Config { return &config.Config{Domain: "example.com"} }

	require.NoError(t, runConfigureInstance(context.Background(), []string{"-show"}))
	require.GreaterOrEqual(t, instanceRepo.getRulesCalls, 1)
	require.GreaterOrEqual(t, instanceRepo.getDescCalls, 1)
	require.GreaterOrEqual(t, pushRepo.getCalls, 1)

	require.NoError(t, runConfigureInstance(context.Background(), []string{"-set-rules", "a,b"}))
	require.Equal(t, 1, instanceRepo.setRulesCalls)

	require.NoError(t, runConfigureInstance(context.Background(), []string{"-set-description", "<p>x</p>"}))
	require.Equal(t, 1, instanceRepo.setDescCalls)

	require.NoError(t, runConfigureInstance(context.Background(), []string{"-generate-vapid"}))
	require.Equal(t, 1, pushRepo.setCalls)
	require.NotNil(t, pushRepo.keys)
	require.NotEmpty(t, pushRepo.keys.PublicKey)

	require.NoError(t, runConfigureInstance(context.Background(), nil))

	require.Error(t, runConfigureInstance(context.Background(), []string{"-unknown-flag"}))

	getDynamormClientFn = func(context.Context) (dynamormCore.DB, error) { return nil, errors.New("no db") }
	require.Error(t, runConfigureInstance(context.Background(), []string{"-show"}))

	getDynamormClientFn = func(context.Context) (dynamormCore.DB, error) { return nil, nil }
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (repositoriesProvider, error) {
		return nil, errors.New("no repos")
	}
	require.Error(t, runConfigureInstance(context.Background(), []string{"-show"}))
}

func TestShowHelpers_ErrorBranches_Round12(t *testing.T) {
	instanceRepo := &fakeInstanceRepo{getRulesErr: errors.New("boom"), getDescErr: errors.New("boom")}
	pushRepo := &fakePushRepo{getErr: errors.New("boom")}
	appCtx := &appContext{
		ctx:    context.Background(),
		logger: zap.NewNop(),
		repos:  &fakeRepos{instance: instanceRepo, push: pushRepo},
	}

	showInstanceRules(appCtx)
	showExtendedDescription(appCtx)
	showVAPIDConfiguration(appCtx)
}

func TestNewRepositoryFactoryFn_Default_Round12(t *testing.T) {
	db := dynamormmocks.NewMockExtendedDB()
	repos, err := newRepositoryFactoryFn(db, "table", zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, repos)
	require.NotNil(t, repos.Instance())
	require.NotNil(t, repos.PushSubscription())
}

func TestGetDomain_PromptAndError_Round12(t *testing.T) {
	origCfg := getConfigFn
	origScan := scanlnFn
	t.Cleanup(func() {
		getConfigFn = origCfg
		scanlnFn = origScan
	})

	getConfigFn = func() *config.Config { return &config.Config{Domain: ""} }
	scanlnFn = func(a ...any) (int, error) {
		*(a[0].(*string)) = "example.com"
		return 1, nil
	}
	domain, err := getDomain()
	require.NoError(t, err)
	require.Equal(t, "example.com", domain)

	scanlnFn = func(...any) (int, error) { return 0, errors.New("no input") }
	_, err = getDomain()
	require.Error(t, err)
}

func TestEncodeKeys_PadsPrivateKey_Round12(t *testing.T) {
	curve := elliptic.P256()
	d := big.NewInt(1)
	x, y := curve.ScalarBaseMult(d.Bytes())
	privateKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y},
		D:         d,
	}

	_, privateKeyBase64, err := encodeKeys(privateKey)
	require.NoError(t, err)
	require.NotEmpty(t, privateKeyBase64)
}

func TestGenerateECDSAKey_ErrorBranch_Round12(t *testing.T) {
	origReader := rand.Reader
	t.Cleanup(func() { rand.Reader = origReader })
	rand.Reader = io.LimitReader(failingReader{}, 1)

	_, err := generateECDSAKey()
	require.Error(t, err)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("no entropy")
}

func TestGenerateVAPIDKeys_SaveError_Round12(t *testing.T) {
	origCfg := getConfigFn
	t.Cleanup(func() { getConfigFn = origCfg })
	getConfigFn = func() *config.Config { return &config.Config{Domain: "example.com"} }

	pushRepo := &fakePushRepo{setErr: errors.New("boom")}
	appCtx := &appContext{
		ctx:    context.Background(),
		logger: zap.NewNop(),
		repos:  &fakeRepos{instance: &fakeInstanceRepo{}, push: pushRepo},
	}

	require.Error(t, generateVAPIDKeys(appCtx))
}
