package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

const defaultBootstrapDerivationPath = "m/44'/60'/0'/0/0"
const instanceConfigKeyPK = "INSTANCE#CONFIG"

type bootstrapWallet struct {
	Address        string `json:"address"`
	Mnemonic       string `json:"mnemonic,omitempty"`
	DerivationPath string `json:"derivation_path"`
	ChainID        int    `json:"chain_id"`
}

var (
	determineBootstrapWalletFn     = determineBootstrapWallet
	writeBootstrapKeyMaterialFn    = writeBootstrapKeyMaterial
	readBootstrapKeyMaterialFn     = readBootstrapKeyMaterial
	inspectBootstrapRequirementsFn = inspectBootstrapRequirements
	ensureStageBootstrapStateFn    = ensureStageBootstrapState
)

type dynamodbAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

func determineBootstrapWallet(existingAddress string) (bootstrapWallet, error) {
	if strings.TrimSpace(existingAddress) != "" {
		return bootstrapWallet{
			Address:        strings.ToLower(strings.TrimSpace(existingAddress)),
			DerivationPath: defaultBootstrapDerivationPath,
			ChainID:        1,
		}, nil
	}

	return generateBootstrapWallet()
}

func generateBootstrapWallet() (bootstrapWallet, error) {
	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return bootstrapWallet{}, fmt.Errorf("generate entropy: %w", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return bootstrapWallet{}, fmt.Errorf("generate mnemonic: %w", err)
	}

	seed := bip39.NewSeed(mnemonic, "")
	path, err := accounts.ParseDerivationPath(defaultBootstrapDerivationPath)
	if err != nil {
		return bootstrapWallet{}, fmt.Errorf("parse derivation path: %w", err)
	}

	key, err := deriveEthereumPrivateKey(seed, path)
	if err != nil {
		return bootstrapWallet{}, err
	}
	addr := crypto.PubkeyToAddress(key.PublicKey).Hex()

	return bootstrapWallet{
		Address:        strings.ToLower(addr),
		Mnemonic:       mnemonic,
		DerivationPath: defaultBootstrapDerivationPath,
		ChainID:        1,
	}, nil
}

func deriveEthereumPrivateKey(seed []byte, path accounts.DerivationPath) (*ecdsa.PrivateKey, error) {
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		return nil, fmt.Errorf("derive master key: %w", err)
	}

	key := masterKey
	for _, component := range path {
		key, err = key.NewChildKey(component)
		if err != nil {
			return nil, fmt.Errorf("derive child key: %w", err)
		}
	}

	privBytes := key.Key
	if len(privBytes) == 33 && privBytes[0] == 0x00 {
		privBytes = privBytes[1:]
	}
	if len(privBytes) != 32 {
		return nil, fmt.Errorf("unexpected derived private key length %d", len(privBytes))
	}

	priv, err := crypto.ToECDSA(privBytes)
	if err != nil {
		return nil, fmt.Errorf("convert derived key: %w", err)
	}
	return priv, nil
}

func writeBootstrapKeyMaterial(outPath string, wallet bootstrapWallet) error {
	if wallet.Mnemonic == "" {
		return errors.New("mnemonic is empty")
	}

	payload := struct {
		CreatedAt time.Time       `json:"created_at"`
		Wallet    bootstrapWallet `json:"wallet"`
	}{
		CreatedAt: time.Now().UTC(),
		Wallet:    wallet,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	tmp := outPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	if err := os.Rename(tmp, outPath); err != nil {
		return fmt.Errorf("finalize output file: %w", err)
	}
	return nil
}

func readBootstrapKeyMaterial(path string) (bootstrapWallet, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- file path is user-supplied/local state
	if err != nil {
		return bootstrapWallet{}, fmt.Errorf("read bootstrap key material: %w", err)
	}

	var payload struct {
		Wallet bootstrapWallet `json:"wallet"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return bootstrapWallet{}, fmt.Errorf("parse bootstrap key material: %w", err)
	}

	wallet := payload.Wallet
	wallet.Address = strings.ToLower(strings.TrimSpace(wallet.Address))
	wallet.Mnemonic = strings.TrimSpace(wallet.Mnemonic)
	wallet.DerivationPath = strings.TrimSpace(wallet.DerivationPath)
	if wallet.Mnemonic == "" || wallet.Address == "" {
		return bootstrapWallet{}, errors.New("invalid bootstrap key material (missing address or mnemonic)")
	}
	return wallet, nil
}

type stageBootstrapState struct {
	Locked  bool
	Address string
	Updated bool
}

func stageMainTableName(app string, stage naming.Stage) string {
	return naming.ResourceNameWithApp(app, "main-table", string(stage))
}

func ensureStageBootstrapState(ctx context.Context, ddb dynamodbAPI, app string, stage naming.Stage, desiredAddress string) (stageBootstrapState, error) {
	tableName := stageMainTableName(app, stage)
	current, err := getInstanceStateItem(ctx, ddb, tableName)
	if err != nil {
		return stageBootstrapState{}, err
	}

	if current.Exists && !current.Locked {
		return stageBootstrapState{Locked: false, Address: strings.ToLower(current.BootstrapWalletAddress)}, nil
	}

	currentAddress := strings.ToLower(strings.TrimSpace(current.BootstrapWalletAddress))
	if currentAddress != "" {
		desiredAddress = strings.ToLower(strings.TrimSpace(desiredAddress))
		if desiredAddress != "" && !strings.EqualFold(currentAddress, desiredAddress) {
			return stageBootstrapState{}, fmt.Errorf("stage %s already has bootstrap wallet %s configured; refusing to overwrite", stage, currentAddress)
		}
		return stageBootstrapState{Locked: true, Address: currentAddress}, nil
	}

	desiredAddress = strings.ToLower(strings.TrimSpace(desiredAddress))
	if desiredAddress == "" {
		return stageBootstrapState{}, errors.New("bootstrap address is empty")
	}

	now := time.Now().UTC()
	if err := upsertInstanceState(ctx, ddb, tableName, now, desiredAddress); err != nil {
		return stageBootstrapState{}, err
	}
	return stageBootstrapState{Locked: true, Address: desiredAddress, Updated: true}, nil
}

type instanceStateItem struct {
	Exists                 bool
	Locked                 bool
	BootstrapWalletAddress string
}

type tableNotFoundError struct {
	TableName string
}

func (e tableNotFoundError) Error() string {
	return fmt.Sprintf("DynamoDB table %q not found", e.TableName)
}

func getInstanceStateItem(ctx context.Context, ddb dynamodbAPI, tableName string) (instanceStateItem, error) {
	out, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(tableName),
		ConsistentRead: aws.Bool(true),
		Key: map[string]dynamotypes.AttributeValue{
			"PK": &dynamotypes.AttributeValueMemberS{Value: instanceConfigKeyPK},
			"SK": &dynamotypes.AttributeValueMemberS{Value: "STATE"},
		},
	})
	if err != nil {
		var rnfe *dynamotypes.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return instanceStateItem{}, tableNotFoundError{TableName: tableName}
		}
		return instanceStateItem{}, fmt.Errorf("get instance state from %q: %w", tableName, err)
	}
	if len(out.Item) == 0 {
		return instanceStateItem{Exists: false, Locked: true}, nil
	}

	locked := true
	if v, ok := out.Item["locked"].(*dynamotypes.AttributeValueMemberBOOL); ok {
		locked = v.Value
	}

	addr := ""
	if v, ok := out.Item["bootstrapWalletAddress"].(*dynamotypes.AttributeValueMemberS); ok {
		addr = v.Value
	}

	return instanceStateItem{
		Exists:                 true,
		Locked:                 locked,
		BootstrapWalletAddress: strings.ToLower(strings.TrimSpace(addr)),
	}, nil
}

func upsertInstanceState(ctx context.Context, ddb dynamodbAPI, tableName string, now time.Time, bootstrapAddress string) error {
	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]dynamotypes.AttributeValue{
			"PK": &dynamotypes.AttributeValueMemberS{Value: instanceConfigKeyPK},
			"SK": &dynamotypes.AttributeValueMemberS{Value: "STATE"},
		},
		UpdateExpression: aws.String("SET #locked = :locked, #bootstrapUsername = :bootstrapUsername, #bootstrapWalletAddress = :bootstrapWalletAddress, #updatedAt = :updatedAt, #createdAt = if_not_exists(#createdAt, :createdAt) REMOVE #activatedAt, #primaryAdminUsername"),
		ExpressionAttributeNames: map[string]string{
			"#locked":                 "locked",
			"#bootstrapUsername":      "bootstrapUsername",
			"#bootstrapWalletAddress": "bootstrapWalletAddress",
			"#updatedAt":              "updatedAt",
			"#createdAt":              "createdAt",
			"#activatedAt":            "activatedAt",
			"#primaryAdminUsername":   "primaryAdminUsername",
		},
		ExpressionAttributeValues: map[string]dynamotypes.AttributeValue{
			":locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: true},
			":bootstrapUsername":      &dynamotypes.AttributeValueMemberS{Value: "bootstrap"},
			":bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: strings.ToLower(strings.TrimSpace(bootstrapAddress))},
			":updatedAt":              &dynamotypes.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
			":createdAt":              &dynamotypes.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
		},
	})
	if err != nil {
		return fmt.Errorf("update instance state in %q: %w", tableName, err)
	}
	return nil
}

func inspectBootstrapRequirements(ctx context.Context, ddb dynamodbAPI, app string, stages []naming.Stage) (existingBootstrapAddress string, bootstrapRequired bool, err error) {
	addrs := map[string]struct{}{}

	for _, stage := range stages {
		tableName := stageMainTableName(app, stage)
		item, getErr := getInstanceStateItem(ctx, ddb, tableName)
		if getErr != nil {
			var tnf tableNotFoundError
			if errors.As(getErr, &tnf) {
				bootstrapRequired = true
				continue
			}
			return "", false, getErr
		}
		if !item.Exists {
			bootstrapRequired = true
			continue
		}

		addr := strings.ToLower(strings.TrimSpace(item.BootstrapWalletAddress))
		if addr != "" {
			addrs[addr] = struct{}{}
		}

		if item.Locked && addr == "" {
			bootstrapRequired = true
		}
	}

	if len(addrs) == 0 {
		return "", bootstrapRequired, nil
	}
	if len(addrs) > 1 {
		return "", false, errors.New("multiple bootstrap wallet addresses detected across stages; refusing to proceed")
	}
	for addr := range addrs {
		return addr, bootstrapRequired, nil
	}
	return "", bootstrapRequired, nil
}
