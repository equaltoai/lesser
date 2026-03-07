package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestBuildSoulENSConfig_NormalizesValues(t *testing.T) {
	cfg, err := buildSoulENSConfig(
		" 0X8DB124B1D48E366002DB4E61CC1501EEB8561E1EF06FD6F9ABF9F984501D13AB ",
		" Agent-Alice.LesserLab.ETH ",
		" 0x000000000000000000000000000000000000cAFe ",
		" Mainnet ",
	)
	require.NoError(t, err)
	require.Equal(t, "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab", cfg.AgentID)
	require.Equal(t, "agent-alice.lesserlab.eth", cfg.Name)
	require.Equal(t, "0x000000000000000000000000000000000000cafE", cfg.ResolverAddress)
	require.Equal(t, models.SoulENSChainMainnet, cfg.Chain)
}

func TestBuildSoulENSConfig_RejectsInvalidValues(t *testing.T) {
	testCases := []struct {
		name    string
		agentID string
		ensName string
		address string
		chain   string
		want    string
	}{
		{name: "agent", agentID: "bad", ensName: "agent.lesser.eth", chain: "sepolia", want: "invalid agent id"},
		{name: "name", agentID: "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab", ensName: "not-ens", chain: "sepolia", want: "invalid ENS name"},
		{name: "resolver", agentID: "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab", ensName: "agent.lesser.eth", address: "bad", chain: "sepolia", want: "invalid resolver address"},
		{name: "chain", agentID: "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab", ensName: "agent.lesser.eth", chain: "holesky", want: "invalid chain"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := buildSoulENSConfig(testCase.agentID, testCase.ensName, testCase.address, testCase.chain)
			require.ErrorContains(t, err, testCase.want)
		})
	}
}

func TestSoulENSHelpers(t *testing.T) {
	require.Equal(t, "0xabc", normalizeSoulAgentID(" 0xAbC "))
	require.Equal(t, "agent.example.eth", mustNormalizeSoulENSName(t, " Agent.Example.ETH "))
	require.Equal(t, "0x000000000000000000000000000000000000cafE", mustNormalizeSoulENSResolverAddress(t, "0x000000000000000000000000000000000000cAFe"))
	require.Equal(t, models.SoulENSChainSepolia, mustNormalizeSoulENSChain(t, " Sepolia "))

	cfg := &models.InstanceSoulENSChannel{
		AgentID:         "0xabc",
		Name:            "agent.example.eth",
		ResolverAddress: "0x000000000000000000000000000000000000cAFe",
		Chain:           models.SoulENSChainSepolia,
	}
	require.Equal(t, map[string]any{
		"agentId": "0xabc",
		"channels": map[string]any{
			"ens": map[string]any{
				"name":            "agent.example.eth",
				"chain":           models.SoulENSChainSepolia,
				"resolverAddress": "0x000000000000000000000000000000000000cAFe",
			},
		},
	}, soulENSChannelFragment(cfg))

	require.Equal(t, "value", extractSoulStringField(map[string]any{"key": " value "}, "key"))
	require.Empty(t, extractSoulStringField(map[string]any{"key": 1}, "key"))
	require.Equal(t, "2", mustNormalizeSoulRegistrationVersion(t, map[string]any{"version": "2"}))
	require.Equal(t, "3", mustNormalizeSoulRegistrationVersion(t, map[string]any{"version": "3"}))
	_, err := normalizeSoulCurrentRegistrationVersion(map[string]any{"version": "1"})
	require.ErrorContains(t, err, "legacy v1")
	_, err = normalizeSoulCurrentRegistrationVersion(map[string]any{"version": "9"})
	require.ErrorContains(t, err, "unsupported")
}

func TestEnsureSoulRegistrationObject(t *testing.T) {
	root := map[string]any{
		"channels": map[string]any{"ens": map[string]any{"name": "agent.example.eth"}},
	}
	channels, err := ensureSoulRegistrationObject(root, "channels")
	require.NoError(t, err)
	require.Equal(t, "agent.example.eth", channels["ens"].(map[string]any)["name"])

	created, err := ensureSoulRegistrationObject(root, "attestations")
	require.NoError(t, err)
	require.Empty(t, created)

	_, err = ensureSoulRegistrationObject(map[string]any{"channels": "bad"}, "channels")
	require.ErrorContains(t, err, "must be an object")

	_, err = ensureSoulRegistrationObject(nil, "channels")
	require.ErrorContains(t, err, "nil")
}

func TestBuildSoulRegistrationUpdatePayload_RejectsNegativeVersion(t *testing.T) {
	_, err := buildSoulRegistrationUpdatePayload([]byte(`{}`), -1)
	require.ErrorContains(t, err, "non-negative")
}

func TestBuildSignedSoulENSRegistration_RejectsInvalidInputs(t *testing.T) {
	signingKey, walletAddress := mustSoulSigningKey(t)
	validConfig := &models.InstanceSoulENSChannel{
		AgentID: "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab",
		Name:    "agent-alice.lesserlab.eth",
		Chain:   "sepolia",
	}
	validRegistration := mustSoulJSON(t, map[string]any{
		"version":      "3",
		"agentId":      validConfig.AgentID,
		"wallet":       walletAddress,
		"channels":     map[string]any{},
		"attestations": map[string]any{},
	})
	validSigningMaterial := &soulSigningMaterial{Address: walletAddress, PrivateKey: signingKey, Source: "test"}
	validLatestVersion := soulLatestVersion{VersionNumber: 4, RegistrationURI: "s3://bucket/agent/4.json"}

	testCases := []struct {
		name                string
		currentRegistration []byte
		latestVersion       soulLatestVersion
		cfg                 *models.InstanceSoulENSChannel
		signingMaterial     *soulSigningMaterial
		want                string
	}{
		{name: "missing config", currentRegistration: validRegistration, latestVersion: validLatestVersion, signingMaterial: validSigningMaterial, want: "config is required"},
		{name: "missing signing key", currentRegistration: validRegistration, latestVersion: validLatestVersion, cfg: validConfig, want: "signing material is required"},
		{name: "missing latest version", currentRegistration: validRegistration, latestVersion: soulLatestVersion{RegistrationURI: "s3://bucket/current.json"}, cfg: validConfig, signingMaterial: validSigningMaterial, want: "latest soul version is required"},
		{name: "missing latest uri", currentRegistration: validRegistration, latestVersion: soulLatestVersion{VersionNumber: 1}, cfg: validConfig, signingMaterial: validSigningMaterial, want: "registration URI is required"},
		{name: "invalid registration", currentRegistration: []byte("{"), latestVersion: validLatestVersion, cfg: validConfig, signingMaterial: validSigningMaterial, want: "parse current registration"},
		{name: "agent mismatch", currentRegistration: mustSoulJSON(t, map[string]any{"version": "3", "agentId": "0x0000000000000000000000000000000000000000000000000000000000000000", "wallet": walletAddress}), latestVersion: validLatestVersion, cfg: validConfig, signingMaterial: validSigningMaterial, want: "does not match configured agent"},
		{name: "missing wallet", currentRegistration: mustSoulJSON(t, map[string]any{"version": "3", "agentId": validConfig.AgentID}), latestVersion: validLatestVersion, cfg: validConfig, signingMaterial: validSigningMaterial, want: "missing wallet"},
		{name: "wallet mismatch", currentRegistration: mustSoulJSON(t, map[string]any{"version": "3", "agentId": validConfig.AgentID, "wallet": "0x000000000000000000000000000000000000cAFe"}), latestVersion: validLatestVersion, cfg: validConfig, signingMaterial: validSigningMaterial, want: "does not match registration wallet"},
		{name: "channels wrong type", currentRegistration: mustSoulJSON(t, map[string]any{"version": "3", "agentId": validConfig.AgentID, "wallet": walletAddress, "channels": "bad", "attestations": map[string]any{}}), latestVersion: validLatestVersion, cfg: validConfig, signingMaterial: validSigningMaterial, want: "channels must be an object"},
		{name: "attestations wrong type", currentRegistration: mustSoulJSON(t, map[string]any{"version": "3", "agentId": validConfig.AgentID, "wallet": walletAddress, "channels": map[string]any{}, "attestations": "bad"}), latestVersion: validLatestVersion, cfg: validConfig, signingMaterial: validSigningMaterial, want: "attestations must be an object"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := buildSignedSoulENSRegistration(
				testCase.currentRegistration,
				testCase.latestVersion,
				testCase.cfg,
				testCase.signingMaterial,
				"",
			)
			require.ErrorContains(t, err, testCase.want)
		})
	}
}

func TestDefaultSoulBootstrapPath(t *testing.T) {
	previousUserHomeDir := userHomeDirFn
	t.Cleanup(func() { userHomeDirFn = previousUserHomeDir })

	userHomeDirFn = func() (string, error) { return "/tmp/home", nil }
	require.Equal(
		t,
		filepath.Join("/tmp/home", ".lesser", "sim", "example.com", "bootstrap.json"),
		defaultSoulBootstrapPath("sim", "example.com"),
	)
	require.Empty(t, defaultSoulBootstrapPath("", "example.com"))

	userHomeDirFn = func() (string, error) { return "", errors.New("boom") }
	require.Empty(t, defaultSoulBootstrapPath("sim", "example.com"))
}

func TestParseSoulSigningPayload(t *testing.T) {
	privateKey, walletAddress := mustSoulSigningKey(t)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	material, err := parseSoulSigningPayload(`{"address":"`+walletAddress+`","private_key":"`+privateKeyHex+`"}`, "json")
	require.NoError(t, err)
	require.Equal(t, strings.ToLower(walletAddress), material.Address)
	require.Equal(t, "json", material.Source)

	material, err = parseSoulSigningPayload("0x"+privateKeyHex, "raw")
	require.NoError(t, err)
	require.Equal(t, strings.ToLower(walletAddress), material.Address)

	_, err = parseSoulSigningPayload(`{"address":"0x000000000000000000000000000000000000cAFe","private_key":"`+privateKeyHex+`"}`, "json")
	require.ErrorContains(t, err, "address mismatch")

	_, err = parseSoulSigningPayload(" ", "raw")
	require.ErrorContains(t, err, "empty")
}

func TestResolveSoulSigningMaterial(t *testing.T) {
	privateKey, walletAddress := mustSoulSigningKey(t)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	t.Run("private key hex", func(t *testing.T) {
		material, err := resolveSoulSigningMaterial(context.Background(), soulTarget{}, soulENSPublishFlags{
			PrivateKeyHex: privateKeyHex,
		})
		require.NoError(t, err)
		require.Equal(t, strings.ToLower(walletAddress), material.Address)
		require.Equal(t, "flag --private-key-hex", material.Source)
	})

	t.Run("private key file", func(t *testing.T) {
		privateKeyFile := filepath.Join(t.TempDir(), "wallet.json")
		require.NoError(t, os.WriteFile(privateKeyFile, []byte(`{"address":"`+walletAddress+`","private_key":"`+privateKeyHex+`"}`), 0o600))

		material, err := resolveSoulSigningMaterial(context.Background(), soulTarget{}, soulENSPublishFlags{
			PrivateKeyFile: privateKeyFile,
		})
		require.NoError(t, err)
		require.Equal(t, strings.ToLower(walletAddress), material.Address)
		require.Equal(t, "file "+privateKeyFile, material.Source)
	})

	t.Run("bootstrap path", func(t *testing.T) {
		previousReadBootstrap := readBootstrapKeyMaterialFn
		previousUserHomeDir := userHomeDirFn
		t.Cleanup(func() {
			readBootstrapKeyMaterialFn = previousReadBootstrap
			userHomeDirFn = previousUserHomeDir
		})

		const mnemonic = "test test test test test test test test test test test junk"
		path, err := accounts.ParseDerivationPath(defaultBootstrapDerivationPath)
		require.NoError(t, err)
		derivedKey, err := deriveEthereumPrivateKey(bip39Seed(mnemonic), path)
		require.NoError(t, err)
		derivedAddress := strings.ToLower(crypto.PubkeyToAddress(derivedKey.PublicKey).Hex())

		homeDir := t.TempDir()
		userHomeDirFn = func() (string, error) { return homeDir, nil }
		expectedPath := defaultSoulBootstrapPath("sim", "example.com")
		readBootstrapKeyMaterialFn = func(path string) (bootstrapWallet, error) {
			require.Equal(t, expectedPath, path)
			return bootstrapWallet{
				Address:        derivedAddress,
				Mnemonic:       mnemonic,
				DerivationPath: defaultBootstrapDerivationPath,
			}, nil
		}

		material, err := resolveSoulSigningMaterial(context.Background(), soulTarget{
			App:        "sim",
			BaseDomain: "example.com",
		}, soulENSPublishFlags{})
		require.NoError(t, err)
		require.Equal(t, derivedAddress, material.Address)
		require.Equal(t, "bootstrap "+expectedPath, material.Source)
	})

	t.Run("missing input", func(t *testing.T) {
		previousUserHomeDir := userHomeDirFn
		t.Cleanup(func() { userHomeDirFn = previousUserHomeDir })

		userHomeDirFn = func() (string, error) { return "", errors.New("boom") }
		_, err := resolveSoulSigningMaterial(context.Background(), soulTarget{}, soulENSPublishFlags{})
		require.ErrorContains(t, err, "signing material is required")
	})

	t.Run("wallet secret", func(t *testing.T) {
		cfg := aws.Config{
			Region:      "us-east-1",
			Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		}
		cfg.HTTPClient = staticHTTPClient(func(request *http.Request) (*http.Response, error) {
			body := `{"SecretString":"` + privateKeyHex + `"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type":     []string{"application/x-amz-json-1.1"},
					"x-amzn-RequestId": []string{"req-wallet"},
				},
				Body:    io.NopCloser(strings.NewReader(body)),
				Request: request,
			}, nil
		})

		material, err := resolveSoulSigningMaterial(context.Background(), soulTarget{AWSConfig: cfg}, soulENSPublishFlags{
			WalletSecretARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:test",
		})
		require.NoError(t, err)
		require.Equal(t, strings.ToLower(walletAddress), material.Address)
		require.Contains(t, material.Source, "secret arn:aws:secretsmanager")
	})
}

func TestFetchSoulRegistration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/v1/soul/agents/0xabc/registration", request.URL.Path)
		_, _ = writer.Write([]byte(`{"version":"3"}`))
	}))
	defer server.Close()

	body, err := fetchSoulRegistration(context.Background(), server.URL+"/", "0xabc")
	require.NoError(t, err)
	require.JSONEq(t, `{"version":"3"}`, string(body))

	errorServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "nope", http.StatusBadRequest)
	}))
	defer errorServer.Close()

	_, err = fetchSoulRegistration(context.Background(), errorServer.URL, "0xabc")
	require.ErrorContains(t, err, "failed (400)")
}

func TestFetchLatestSoulVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/v1/soul/agents/0xabc/versions", request.URL.Path)
		require.Equal(t, "1", request.URL.Query().Get("limit"))
		require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{
			Versions: []struct {
				VersionNumber   int    `json:"version_number"`
				RegistrationURI string `json:"registration_uri"`
			}{
				{VersionNumber: 7, RegistrationURI: "s3://bucket/agent/7.json"},
			},
		}))
	}))
	defer server.Close()

	version, err := fetchLatestSoulVersion(context.Background(), server.URL, "0xabc")
	require.NoError(t, err)
	require.Equal(t, 7, version.VersionNumber)
	require.Equal(t, "s3://bucket/agent/7.json", version.RegistrationURI)

	emptyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{}))
	}))
	defer emptyServer.Close()

	_, err = fetchLatestSoulVersion(context.Background(), emptyServer.URL, "0xabc")
	require.ErrorContains(t, err, "no published versions")
}

func TestFetchLatestSoulVersion_Errors(t *testing.T) {
	invalidJSONServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("{"))
	}))
	defer invalidJSONServer.Close()

	_, err := fetchLatestSoulVersion(context.Background(), invalidJSONServer.URL, "0xabc")
	require.ErrorContains(t, err, "parse latest soul version")

	invalidVersionServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{
			Versions: []struct {
				VersionNumber   int    `json:"version_number"`
				RegistrationURI string `json:"registration_uri"`
			}{
				{VersionNumber: 0, RegistrationURI: "s3://bucket/agent/0.json"},
			},
		}))
	}))
	defer invalidVersionServer.Close()

	_, err = fetchLatestSoulVersion(context.Background(), invalidVersionServer.URL, "0xabc")
	require.ErrorContains(t, err, "invalid version number")

	missingURIServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{
			Versions: []struct {
				VersionNumber   int    `json:"version_number"`
				RegistrationURI string `json:"registration_uri"`
			}{
				{VersionNumber: 1},
			},
		}))
	}))
	defer missingURIServer.Close()

	_, err = fetchLatestSoulVersion(context.Background(), missingURIServer.URL, "0xabc")
	require.ErrorContains(t, err, "missing registration_uri")

	statusServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "nope", http.StatusBadRequest)
	}))
	defer statusServer.Close()

	_, err = fetchLatestSoulVersion(context.Background(), statusServer.URL, "0xabc")
	require.ErrorContains(t, err, "failed (400)")
}

func TestPublishSoulRegistration(t *testing.T) {
	requestBody := []byte(`{"registration":{"version":"3"}}`)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/api/v1/soul/agents/0xabc/update-registration", request.URL.Path)
		require.Equal(t, "Bearer instance-key", request.Header.Get("Authorization"))
		require.Equal(t, "application/json", request.Header.Get("Content-Type"))
		require.Equal(t, "application/json", request.Header.Get("Accept"))
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		require.JSONEq(t, string(requestBody), string(body))
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	require.NoError(t, publishSoulRegistration(context.Background(), server.URL, " instance-key ", "0xabc", requestBody))

	errorServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "bad publish", http.StatusConflict)
	}))
	defer errorServer.Close()

	err := publishSoulRegistration(context.Background(), errorServer.URL, "instance-key", "0xabc", requestBody)
	require.ErrorContains(t, err, "publish registration failed (409)")
	require.ErrorContains(t, err, "bad publish")
}

func TestVerifySoulENSResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/v1/soul/resolve/ens/agent.example.eth", request.URL.Path)
		require.NoError(t, json.NewEncoder(writer).Encode(soulResolveAgentResponse{
			Version: "3",
			Agent: struct {
				AgentID string `json:"agentId"`
				Domain  string `json:"domain"`
				LocalID string `json:"localId"`
				Wallet  string `json:"wallet"`
			}{
				AgentID: "0xabc",
			},
		}))
	}))
	defer server.Close()

	agentID, err := verifySoulENSResolution(context.Background(), server.URL, "agent.example.eth")
	require.NoError(t, err)
	require.Equal(t, "0xabc", agentID)

	emptyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.NoError(t, json.NewEncoder(writer).Encode(soulResolveAgentResponse{}))
	}))
	defer emptyServer.Close()

	_, err = verifySoulENSResolution(context.Background(), emptyServer.URL, "agent.example.eth")
	require.ErrorContains(t, err, "empty agent id")

	errorServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "nope", http.StatusBadRequest)
	}))
	defer errorServer.Close()

	_, err = verifySoulENSResolution(context.Background(), errorServer.URL, "agent.example.eth")
	require.ErrorContains(t, err, "failed (400)")
}

func TestVerifySoulENSResolution_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("{"))
	}))
	defer server.Close()

	_, err := verifySoulENSResolution(context.Background(), server.URL, "agent.example.eth")
	require.Error(t, err)
}

func TestSoulHTTPHelpers_RequestCreationErrors(t *testing.T) {
	_, err := fetchSoulRegistration(context.Background(), "://bad", "0xabc")
	require.Error(t, err)

	_, err = fetchLatestSoulVersion(context.Background(), "://bad", "0xabc")
	require.Error(t, err)

	err = publishSoulRegistration(context.Background(), "://bad", "instance-key", "0xabc", []byte(`{}`))
	require.Error(t, err)

	_, err = verifySoulENSResolution(context.Background(), "://bad", "agent.example.eth")
	require.Error(t, err)
}

func TestResolveCLISecretValue(t *testing.T) {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
	}

	cfg.HTTPClient = staticHTTPClient(func(request *http.Request) (*http.Response, error) {
		body := `{"SecretString":"{\"secret\":\"instance-key\"}"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":     []string{"application/x-amz-json-1.1"},
				"x-amzn-RequestId": []string{"req-1"},
			},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: request,
		}, nil
	})

	value, err := resolveCLISecretValue(context.Background(), cfg, "arn:aws:secretsmanager:us-east-1:123456789012:secret:test")
	require.NoError(t, err)
	require.Equal(t, "instance-key", value)

	cfg.HTTPClient = staticHTTPClient(func(request *http.Request) (*http.Response, error) {
		body := `{}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":     []string{"application/x-amz-json-1.1"},
				"x-amzn-RequestId": []string{"req-2"},
			},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: request,
		}, nil
	})

	_, err = resolveCLISecretValue(context.Background(), cfg, "arn:aws:secretsmanager:us-east-1:123456789012:secret:test")
	require.ErrorContains(t, err, "SecretString")

	cfg.HTTPClient = staticHTTPClient(func(request *http.Request) (*http.Response, error) {
		body := `{"SecretString":"{not-json}"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":     []string{"application/x-amz-json-1.1"},
				"x-amzn-RequestId": []string{"req-3"},
			},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: request,
		}, nil
	})

	value, err = resolveCLISecretValue(context.Background(), cfg, "arn:aws:secretsmanager:us-east-1:123456789012:secret:test")
	require.NoError(t, err)
	require.Equal(t, "{not-json}", value)
}

func TestResolveSoulTarget(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWSConfig })

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "sim", nil
	}

	target, err := resolveSoulTarget(context.Background(), soulTargetFlags{
		App:        " Sim ",
		Stage:      "dev",
		AWSProfile: "ignored",
		BaseDomain: " Example.COM ",
	})
	require.NoError(t, err)
	require.Equal(t, "sim", target.App)
	require.Equal(t, "sim", target.AWSProfile)
	require.Equal(t, "example.com", target.BaseDomain)
	require.Equal(t, stageMainTableName("sim", naming.StageDev), target.TableName)

	_, err = resolveSoulTarget(context.Background(), soulTargetFlags{App: "sim", Stage: "bad"})
	require.ErrorContains(t, err, "invalid stage")

	_, err = resolveSoulTarget(context.Background(), soulTargetFlags{App: "sim", Stage: "dev", BaseDomain: "-bad"})
	require.Error(t, err)
}

func TestRunSoulDispatch(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWSConfig })

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "sim", nil
	}

	require.NoError(t, runSoul(nil))
	require.NoError(t, runSoul([]string{"help"}))
	require.ErrorContains(t, runSoul([]string{"bad"}), "unknown soul command")
	require.NoError(t, runSoulENS(nil))
	require.NoError(t, runSoul([]string{"ens", "help"}))
	require.ErrorContains(t, runSoul([]string{"ens", "bad"}), "unknown soul ens command")
	require.ErrorContains(
		t,
		runSoul([]string{"ens", "set", "--app", "sim", "--base-domain", "example.com", "--agent-id", "bad", "--name", "agent.example.eth"}),
		"invalid agent id",
	)
	require.ErrorContains(
		t,
		runSoul([]string{"ens", "publish", "--app", "sim", "--base-domain", "example.com", "--agent-id", "bad"}),
		"invalid agent id",
	)
}

func TestRunSoulENSPreview_InvalidStage(t *testing.T) {
	err := runSoulENSPreview([]string{"--stage", "bad"})
	require.ErrorContains(t, err, "invalid stage")
}

func TestRunSoulENSSet_InvalidName(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWSConfig })

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "sim", nil
	}

	err := runSoulENSSet([]string{
		"--app", "sim",
		"--base-domain", "example.com",
		"--agent-id", "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab",
		"--name", "not-ens",
	})
	require.ErrorContains(t, err, "invalid ENS name")
}

func TestRunSoulENSPublish_InvalidStage(t *testing.T) {
	err := runSoulENSPublish([]string{
		"--stage", "bad",
		"--agent-id", "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab",
	})
	require.ErrorContains(t, err, "invalid stage")
}

func TestRunSoulENSPreview_InvalidBaseDomain(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	t.Cleanup(func() { loadAWSConfigForCLIFn = previousLoadAWSConfig })

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-east-1"}, "sim", nil
	}

	err := runSoulENSPreview([]string{"--base-domain", "-bad"})
	require.Error(t, err)
}

func TestRunSoulENS_CommandParsingErrors(t *testing.T) {
	require.Error(t, runSoulENSSet([]string{"--bad-flag"}))
	require.Error(t, runSoulENSPreview([]string{"--bad-flag"}))
	require.Error(t, runSoulENSPublish([]string{"--bad-flag"}))
}

func TestRunSoulENS_SubcommandDispatch(t *testing.T) {
	require.NoError(t, runSoul([]string{"ens"}))
	require.Error(t, runSoulENS([]string{"set", "--bad-flag"}))
	require.ErrorContains(t, runSoulENS([]string{"preview", "--stage", "bad"}), "invalid stage")
	require.Error(t, runSoulENS([]string{"publish", "--bad-flag"}))
}

func TestValidateSoulAgentID_Errors(t *testing.T) {
	_, err := validateSoulAgentID("")
	require.ErrorContains(t, err, "required")

	_, err = validateSoulAgentID("0xzzzz")
	require.ErrorContains(t, err, "invalid agent id")

	_, err = validateSoulAgentID("0xgggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg")
	require.ErrorContains(t, err, "invalid agent id")
}

func TestSoulHelperErrorBranches(t *testing.T) {
	_, err := normalizeSoulENSName("")
	require.ErrorContains(t, err, "required")

	normalizedResolver, err := normalizeSoulENSResolverAddress("")
	require.NoError(t, err)
	require.Empty(t, normalizedResolver)

	_, err = parseSoulSigningPrivateKey("not-a-private-key", "test")
	require.ErrorContains(t, err, "parse wallet private key")

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
	}
	cfg.HTTPClient = staticHTTPClient(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":     []string{"application/x-amz-json-1.1"},
				"x-amzn-RequestId": []string{"req-empty"},
			},
			Body:    io.NopCloser(strings.NewReader(`{"SecretString":"  "}`)),
			Request: request,
		}, nil
	})

	_, err = resolveCLISecretValue(context.Background(), cfg, "arn:aws:secretsmanager:us-east-1:123456789012:secret:test")
	require.ErrorContains(t, err, "empty")

	_, err = resolveSoulSigningMaterial(context.Background(), soulTarget{}, soulENSPublishFlags{
		PrivateKeyFile: filepath.Join(t.TempDir(), "missing.key"),
	})
	require.ErrorContains(t, err, "read private key file")

	previousReadBootstrap := readBootstrapKeyMaterialFn
	previousUserHomeDir := userHomeDirFn
	t.Cleanup(func() {
		readBootstrapKeyMaterialFn = previousReadBootstrap
		userHomeDirFn = previousUserHomeDir
	})
	userHomeDirFn = func() (string, error) { return t.TempDir(), nil }
	readBootstrapKeyMaterialFn = func(string) (bootstrapWallet, error) { return bootstrapWallet{}, errors.New("boom") }

	_, err = resolveSoulSigningMaterial(context.Background(), soulTarget{App: "sim", BaseDomain: "example.com"}, soulENSPublishFlags{})
	require.ErrorContains(t, err, "read bootstrap signing material")

	require.Empty(t, extractSoulStringField(nil, "missing"))
}

func TestPublishSoulENSChannel(t *testing.T) {
	const agentID = "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab"
	trustBaseURL := ""

	db := new(dynamormmocks.MockDB)
	query := new(dynamormmocks.MockQuery)
	setupSoulMockDB(db, query)

	query.On("First", mock.AnythingOfType("*models.InstanceSoulENSChannel")).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.InstanceSoulENSChannel)
		*dest = models.InstanceSoulENSChannel{
			PK:              "INSTANCE#CONFIG",
			SK:              models.SoulENSChannelSortKey(agentID),
			AgentID:         agentID,
			Name:            "agent.example.eth",
			ResolverAddress: "0x000000000000000000000000000000000000cAFe",
			Chain:           "sepolia",
		}
	}).Once()
	query.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.InstanceTrustConfig)
		*dest = models.InstanceTrustConfig{
			PK: "INSTANCE#CONFIG",
			SK: models.SKTrustConfig,
			Managed: &models.InstanceTrustConfigManaged{
				BaseURL:              trustBaseURL,
				InstanceKeySecretARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:test",
			},
		}
	}).Once()

	repo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	privateKey, walletAddress := mustSoulSigningKey(t)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	trustServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/soul/agents/"+agentID+"/registration":
			_, _ = writer.Write(mustSoulJSON(t, map[string]any{
				"version":      "3",
				"agentId":      agentID,
				"wallet":       walletAddress,
				"channels":     map[string]any{},
				"attestations": map[string]any{},
			}))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/soul/agents/"+agentID+"/versions":
			require.Equal(t, "1", request.URL.Query().Get("limit"))
			require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{
				Versions: []struct {
					VersionNumber   int    `json:"version_number"`
					RegistrationURI string `json:"registration_uri"`
				}{
					{VersionNumber: 4, RegistrationURI: "s3://bucket/agent/4.json"},
				},
			}))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/soul/agents/"+agentID+"/update-registration":
			require.Equal(t, "Bearer lesser-host-instance-key", request.Header.Get("Authorization"))
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			var parsed struct {
				Registration    map[string]any `json:"registration"`
				ExpectedVersion int            `json:"expected_version"`
			}
			require.NoError(t, json.Unmarshal(body, &parsed))
			require.Equal(t, 4, parsed.ExpectedVersion)
			require.Equal(t, "3", parsed.Registration["version"])
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/soul/resolve/ens/agent.example.eth":
			require.NoError(t, json.NewEncoder(writer).Encode(soulResolveAgentResponse{
				Version: "3",
				Agent: struct {
					AgentID string `json:"agentId"`
					Domain  string `json:"domain"`
					LocalID string `json:"localId"`
					Wallet  string `json:"wallet"`
				}{
					AgentID: agentID,
				},
			}))
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
	}))
	defer trustServer.Close()
	trustBaseURL = trustServer.URL

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
	}
	cfg.HTTPClient = staticHTTPClient(func(request *http.Request) (*http.Response, error) {
		body := `{"SecretString":"lesser-host-instance-key"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":     []string{"application/x-amz-json-1.1"},
				"x-amzn-RequestId": []string{"req-publish"},
			},
			Body:    io.NopCloser(strings.NewReader(body)),
			Request: request,
		}, nil
	})

	err := publishSoulENSChannel(context.Background(), soulTarget{AWSConfig: cfg}, repo, agentID, soulENSPublishFlags{
		PrivateKeyHex: privateKeyHex,
		ChangeSummary: "publish ens",
	})
	require.NoError(t, err)
}

func TestPublishSoulENSChannel_RejectsMissingConfigAndTrust(t *testing.T) {
	const agentID = "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab"

	t.Run("missing config", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		query := new(dynamormmocks.MockQuery)
		setupSoulMockDB(db, query)
		query.On("First", mock.AnythingOfType("*models.InstanceSoulENSChannel")).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		err := publishSoulENSChannel(context.Background(), soulTarget{}, repo, agentID, soulENSPublishFlags{})
		require.ErrorContains(t, err, "no soul ENS config stored")
	})

	t.Run("missing trust base url", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		query := new(dynamormmocks.MockQuery)
		setupSoulMockDB(db, query)
		query.On("First", mock.AnythingOfType("*models.InstanceSoulENSChannel")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceSoulENSChannel)
			*dest = models.InstanceSoulENSChannel{
				PK:      "INSTANCE#CONFIG",
				SK:      models.SoulENSChannelSortKey(agentID),
				AgentID: agentID,
				Name:    "agent.example.eth",
				Chain:   "sepolia",
			}
		}).Once()
		query.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceTrustConfig)
			*dest = models.InstanceTrustConfig{
				PK:      "INSTANCE#CONFIG",
				SK:      models.SKTrustConfig,
				Managed: &models.InstanceTrustConfigManaged{},
			}
		}).Once()

		repo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		err := publishSoulENSChannel(context.Background(), soulTarget{}, repo, agentID, soulENSPublishFlags{})
		require.ErrorContains(t, err, "trust config is not available")
	})

	t.Run("missing instance key secret", func(t *testing.T) {
		db := new(dynamormmocks.MockDB)
		query := new(dynamormmocks.MockQuery)
		setupSoulMockDB(db, query)
		query.On("First", mock.AnythingOfType("*models.InstanceSoulENSChannel")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceSoulENSChannel)
			*dest = models.InstanceSoulENSChannel{
				PK:      "INSTANCE#CONFIG",
				SK:      models.SoulENSChannelSortKey(agentID),
				AgentID: agentID,
				Name:    "agent.example.eth",
				Chain:   "sepolia",
			}
		}).Once()
		query.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceTrustConfig)
			*dest = models.InstanceTrustConfig{
				PK: "INSTANCE#CONFIG",
				SK: models.SKTrustConfig,
				Managed: &models.InstanceTrustConfigManaged{
					BaseURL: "https://trust.example.com",
				},
			}
		}).Once()

		repo := repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
		err := publishSoulENSChannel(context.Background(), soulTarget{}, repo, agentID, soulENSPublishFlags{})
		require.ErrorContains(t, err, "missing lesser-host instance key secret ARN")
	})
}

func TestPublishSoulENSChannel_FailurePaths(t *testing.T) {
	const agentID = "0x8db124b1d48e366002db4e61cc1501eeb8561e1ef06fd6f9abf9f984501d13ab"

	privateKey, walletAddress := mustSoulSigningKey(t)
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))

	newRepo := func(trustBaseURL string) *repositories.InstanceRepository {
		db := new(dynamormmocks.MockDB)
		query := new(dynamormmocks.MockQuery)
		setupSoulMockDB(db, query)
		query.On("First", mock.AnythingOfType("*models.InstanceSoulENSChannel")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceSoulENSChannel)
			*dest = models.InstanceSoulENSChannel{
				PK:      "INSTANCE#CONFIG",
				SK:      models.SoulENSChannelSortKey(agentID),
				AgentID: agentID,
				Name:    "agent.example.eth",
				Chain:   "sepolia",
			}
		}).Once()
		query.On("First", mock.AnythingOfType("*models.InstanceTrustConfig")).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.InstanceTrustConfig)
			*dest = models.InstanceTrustConfig{
				PK: "INSTANCE#CONFIG",
				SK: models.SKTrustConfig,
				Managed: &models.InstanceTrustConfigManaged{
					BaseURL:              trustBaseURL,
					InstanceKeySecretARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:test",
				},
			}
		}).Once()
		return repositories.NewInstanceRepository(db, "test-table", zap.NewNop())
	}

	newAWSConfig := func(secretBody string) aws.Config {
		cfg := aws.Config{
			Region:      "us-east-1",
			Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		}
		cfg.HTTPClient = staticHTTPClient(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type":     []string{"application/x-amz-json-1.1"},
					"x-amzn-RequestId": []string{"req-failure"},
				},
				Body:    io.NopCloser(strings.NewReader(secretBody)),
				Request: request,
			}, nil
		})
		return cfg
	}

	newRegistrationResponse := func() []byte {
		return mustSoulJSON(t, map[string]any{
			"version":      "3",
			"agentId":      agentID,
			"wallet":       walletAddress,
			"channels":     map[string]any{},
			"attestations": map[string]any{},
		})
	}

	t.Run("secret lookup failure", func(t *testing.T) {
		repo := newRepo("https://trust.example.com")
		err := publishSoulENSChannel(context.Background(), soulTarget{AWSConfig: newAWSConfig(`{}`)}, repo, agentID, soulENSPublishFlags{
			PrivateKeyHex: privateKeyHex,
		})
		require.ErrorContains(t, err, "resolve lesser-host instance key")
	})

	t.Run("registration fetch failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "no registration", http.StatusBadGateway)
		}))
		defer server.Close()

		repo := newRepo(server.URL)
		err := publishSoulENSChannel(context.Background(), soulTarget{AWSConfig: newAWSConfig(`{"SecretString":"lesser-host-instance-key"}`)}, repo, agentID, soulENSPublishFlags{
			PrivateKeyHex: privateKeyHex,
		})
		require.ErrorContains(t, err, "fetch current registration failed")
	})

	t.Run("latest version fetch failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/v1/soul/agents/" + agentID + "/registration":
				_, _ = writer.Write(newRegistrationResponse())
			default:
				http.Error(writer, "no versions", http.StatusBadGateway)
			}
		}))
		defer server.Close()

		repo := newRepo(server.URL)
		err := publishSoulENSChannel(context.Background(), soulTarget{AWSConfig: newAWSConfig(`{"SecretString":"lesser-host-instance-key"}`)}, repo, agentID, soulENSPublishFlags{
			PrivateKeyHex: privateKeyHex,
		})
		require.ErrorContains(t, err, "fetch latest soul version failed")
	})

	t.Run("publish failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/v1/soul/agents/" + agentID + "/registration":
				_, _ = writer.Write(newRegistrationResponse())
			case "/api/v1/soul/agents/" + agentID + "/versions":
				require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{
					Versions: []struct {
						VersionNumber   int    `json:"version_number"`
						RegistrationURI string `json:"registration_uri"`
					}{
						{VersionNumber: 1, RegistrationURI: "s3://bucket/agent/1.json"},
					},
				}))
			case "/api/v1/soul/agents/" + agentID + "/update-registration":
				http.Error(writer, "cannot publish", http.StatusConflict)
			default:
				http.Error(writer, "unexpected", http.StatusNotFound)
			}
		}))
		defer server.Close()

		repo := newRepo(server.URL)
		err := publishSoulENSChannel(context.Background(), soulTarget{AWSConfig: newAWSConfig(`{"SecretString":"lesser-host-instance-key"}`)}, repo, agentID, soulENSPublishFlags{
			PrivateKeyHex: privateKeyHex,
		})
		require.ErrorContains(t, err, "publish registration failed")
	})

	t.Run("verification mismatch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/v1/soul/agents/" + agentID + "/registration":
				_, _ = writer.Write(newRegistrationResponse())
			case "/api/v1/soul/agents/" + agentID + "/versions":
				require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{
					Versions: []struct {
						VersionNumber   int    `json:"version_number"`
						RegistrationURI string `json:"registration_uri"`
					}{
						{VersionNumber: 1, RegistrationURI: "s3://bucket/agent/1.json"},
					},
				}))
			case "/api/v1/soul/agents/" + agentID + "/update-registration":
				writer.WriteHeader(http.StatusOK)
			case "/api/v1/soul/resolve/ens/agent.example.eth":
				require.NoError(t, json.NewEncoder(writer).Encode(soulResolveAgentResponse{
					Agent: struct {
						AgentID string `json:"agentId"`
						Domain  string `json:"domain"`
						LocalID string `json:"localId"`
						Wallet  string `json:"wallet"`
					}{
						AgentID: "0x0000000000000000000000000000000000000000000000000000000000000000",
					},
				}))
			default:
				http.Error(writer, "unexpected", http.StatusNotFound)
			}
		}))
		defer server.Close()

		repo := newRepo(server.URL)
		err := publishSoulENSChannel(context.Background(), soulTarget{AWSConfig: newAWSConfig(`{"SecretString":"lesser-host-instance-key"}`)}, repo, agentID, soulENSPublishFlags{
			PrivateKeyHex: privateKeyHex,
		})
		require.ErrorContains(t, err, "unexpected agent_id")
	})

	t.Run("skip verify", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/v1/soul/agents/" + agentID + "/registration":
				_, _ = writer.Write(newRegistrationResponse())
			case "/api/v1/soul/agents/" + agentID + "/versions":
				require.NoError(t, json.NewEncoder(writer).Encode(soulListVersionsResponse{
					Versions: []struct {
						VersionNumber   int    `json:"version_number"`
						RegistrationURI string `json:"registration_uri"`
					}{
						{VersionNumber: 1, RegistrationURI: "s3://bucket/agent/1.json"},
					},
				}))
			case "/api/v1/soul/agents/" + agentID + "/update-registration":
				writer.WriteHeader(http.StatusOK)
			default:
				http.Error(writer, "unexpected", http.StatusNotFound)
			}
		}))
		defer server.Close()

		repo := newRepo(server.URL)
		err := publishSoulENSChannel(context.Background(), soulTarget{AWSConfig: newAWSConfig(`{"SecretString":"lesser-host-instance-key"}`)}, repo, agentID, soulENSPublishFlags{
			PrivateKeyHex: privateKeyHex,
			SkipVerify:    true,
		})
		require.NoError(t, err)
	})
}

func TestSoulHTTPHelpers_TransportErrors(t *testing.T) {
	previousDefaultClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previousDefaultClient })

	http.DefaultClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}),
	}

	err := publishSoulRegistration(context.Background(), "https://trust.example.com", "instance-key", "agent-id", []byte(`{}`))
	require.ErrorContains(t, err, "boom")

	_, err = verifySoulENSResolution(context.Background(), "https://trust.example.com", "agent.example.eth")
	require.ErrorContains(t, err, "boom")
}

func TestWithSoulInstanceRepo(t *testing.T) {
	previousTableName := models.MainTableName
	t.Cleanup(func() { models.MainTableName = previousTableName })

	err := withSoulInstanceRepo(soulTarget{
		AWSConfig: aws.Config{
			Region:      "us-east-1",
			Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		},
		TableName: "lesser-test-table",
	}, func(repo *repositories.InstanceRepository) error {
		require.NotNil(t, repo)
		require.Equal(t, "lesser-test-table", models.MainTableName)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, previousTableName, models.MainTableName)
}

func mustNormalizeSoulENSName(t *testing.T, value string) string {
	t.Helper()

	normalized, err := normalizeSoulENSName(value)
	require.NoError(t, err)
	return normalized
}

func mustNormalizeSoulENSResolverAddress(t *testing.T, value string) string {
	t.Helper()

	normalized, err := normalizeSoulENSResolverAddress(value)
	require.NoError(t, err)
	return normalized
}

func mustNormalizeSoulENSChain(t *testing.T, value string) string {
	t.Helper()

	normalized, err := normalizeSoulENSChain(value)
	require.NoError(t, err)
	return normalized
}

func mustNormalizeSoulRegistrationVersion(t *testing.T, registration map[string]any) string {
	t.Helper()

	version, err := normalizeSoulCurrentRegistrationVersion(registration)
	require.NoError(t, err)
	return version
}

type staticHTTPClient func(*http.Request) (*http.Response, error)

func (client staticHTTPClient) Do(request *http.Request) (*http.Response, error) {
	return client(request)
}

func setupSoulMockDB(db *dynamormmocks.MockDB, query *dynamormmocks.MockQuery) {
	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(query).Maybe()
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query).Maybe()
}
