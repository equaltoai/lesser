package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestDetermineBootstrapWallet_ExistingAddress(t *testing.T) {
	wallet, err := determineBootstrapWallet("0xAbC")
	require.NoError(t, err)
	require.Equal(t, "0xabc", wallet.Address)
	require.Empty(t, wallet.Mnemonic)
	require.Equal(t, defaultBootstrapDerivationPath, wallet.DerivationPath)
	require.Equal(t, 1, wallet.ChainID)
}

func TestGenerateBootstrapWalletAndKeyMaterialRoundTrip(t *testing.T) {
	wallet, err := generateBootstrapWallet()
	require.NoError(t, err)
	require.NotEmpty(t, wallet.Address)
	require.NotEmpty(t, wallet.Mnemonic)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "bootstrap.json")

	require.Error(t, writeBootstrapKeyMaterial(path, bootstrapWallet{Address: wallet.Address}))
	require.NoError(t, writeBootstrapKeyMaterial(path, wallet))

	loaded, err := readBootstrapKeyMaterial(path)
	require.NoError(t, err)
	require.Equal(t, wallet.Address, loaded.Address)
	require.Equal(t, wallet.Mnemonic, loaded.Mnemonic)
	require.Equal(t, wallet.DerivationPath, loaded.DerivationPath)
	require.Equal(t, wallet.ChainID, loaded.ChainID)
}

func TestReadBootstrapKeyMaterial_RejectsInvalid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bootstrap.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"wallet":{"address":"","mnemonic":""}}`), 0o644))

	_, err := readBootstrapKeyMaterial(path)
	require.Error(t, err)
}

func TestStageMainTableName(t *testing.T) {
	name := stageMainTableName("app", naming.StageDev)
	require.NotEmpty(t, name)
	require.Contains(t, name, "app")
}

func TestTableNotFoundError(t *testing.T) {
	err := tableNotFoundError{TableName: "tbl"}
	require.Contains(t, err.Error(), "tbl")
}

type fakeDynamoDB struct {
	getItemFn    func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error)
	updateItemFn func(context.Context, *dynamodb.UpdateItemInput) (*dynamodb.UpdateItemOutput, error)

	updateCalls int
	lastUpdate  *dynamodb.UpdateItemInput
}

func (f *fakeDynamoDB) GetItem(ctx context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if f.getItemFn == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	return f.getItemFn(ctx, input)
}

func (f *fakeDynamoDB) UpdateItem(ctx context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.updateCalls++
	f.lastUpdate = input
	if f.updateItemFn == nil {
		return &dynamodb.UpdateItemOutput{}, nil
	}
	return f.updateItemFn(ctx, input)
}

func TestGetInstanceStateItem_ParsesAndHandlesNotFound(t *testing.T) {
	t.Run("table not found maps to typed error", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return nil, &dynamotypes.ResourceNotFoundException{}
			},
		}

		_, err := getInstanceStateItem(context.Background(), fake, "tbl")
		require.Error(t, err)
		var tnf tableNotFoundError
		require.ErrorAs(t, err, &tnf)
	})

	t.Run("missing item locks stage", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
			},
		}

		item, err := getInstanceStateItem(context.Background(), fake, "tbl")
		require.NoError(t, err)
		require.False(t, item.Exists)
		require.True(t, item.Locked)
	})

	t.Run("parses address and unlocked", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{
					Item: map[string]dynamotypes.AttributeValue{
						"locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: false},
						"bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: "0xAbC"},
					},
				}, nil
			},
		}

		item, err := getInstanceStateItem(context.Background(), fake, "tbl")
		require.NoError(t, err)
		require.True(t, item.Exists)
		require.False(t, item.Locked)
		require.Equal(t, "0xabc", item.BootstrapWalletAddress)
	})
}

func TestEnsureStageBootstrapState_HandlesExistingAndUpsert(t *testing.T) {
	app := "app"
	stage := naming.StageDev
	table := stageMainTableName(app, stage)

	t.Run("returns existing unlocked", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{
					Item: map[string]dynamotypes.AttributeValue{
						"locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: false},
						"bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: "0xAbC"},
					},
				}, nil
			},
		}

		state, err := ensureStageBootstrapState(context.Background(), fake, app, stage, "")
		require.NoError(t, err)
		require.False(t, state.Locked)
		require.Equal(t, "0xabc", state.Address)
		require.False(t, state.Updated)
		require.Equal(t, 0, fake.updateCalls)
	})

	t.Run("locked stage refuses overwrite", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{
					Item: map[string]dynamotypes.AttributeValue{
						"locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: true},
						"bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: "0xAbC"},
					},
				}, nil
			},
		}

		_, err := ensureStageBootstrapState(context.Background(), fake, app, stage, "0xdef")
		require.Error(t, err)
		require.Contains(t, err.Error(), "refusing to overwrite")
		require.Equal(t, 0, fake.updateCalls)
	})

	t.Run("requires desired address when missing", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(_ context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				require.Equal(t, table, aws.ToString(input.TableName))
				return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
			},
		}

		_, err := ensureStageBootstrapState(context.Background(), fake, app, stage, "   ")
		require.Error(t, err)
		require.Contains(t, err.Error(), "bootstrap address is empty")
	})

	t.Run("upserts when not configured", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
			},
			updateItemFn: func(_ context.Context, input *dynamodb.UpdateItemInput) (*dynamodb.UpdateItemOutput, error) {
				require.Equal(t, table, aws.ToString(input.TableName))
				return &dynamodb.UpdateItemOutput{}, nil
			},
		}

		state, err := ensureStageBootstrapState(context.Background(), fake, app, stage, "0xAbC")
		require.NoError(t, err)
		require.True(t, state.Locked)
		require.True(t, state.Updated)
		require.Equal(t, "0xabc", state.Address)
		require.Equal(t, 1, fake.updateCalls)
	})

	t.Run("upsert failure surfaces error", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
			},
			updateItemFn: func(context.Context, *dynamodb.UpdateItemInput) (*dynamodb.UpdateItemOutput, error) {
				return nil, errors.New("update failed")
			},
		}

		_, err := ensureStageBootstrapState(context.Background(), fake, app, stage, "0xabc")
		require.Error(t, err)
		require.Contains(t, err.Error(), "update instance state")
		require.Equal(t, 1, fake.updateCalls)
	})
}

func TestInspectBootstrapRequirements_CombinesStages(t *testing.T) {
	app := "app"
	devTable := stageMainTableName(app, naming.StageDev)
	liveTable := stageMainTableName(app, naming.StageLive)

	t.Run("marks required when table missing", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(_ context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				switch aws.ToString(input.TableName) {
				case devTable:
					return nil, &dynamotypes.ResourceNotFoundException{}
				case liveTable:
					return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
				default:
					return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
				}
			},
		}

		addr, required, err := inspectBootstrapRequirements(context.Background(), fake, app, []naming.Stage{naming.StageDev, naming.StageLive})
		require.NoError(t, err)
		require.True(t, required)
		require.Empty(t, addr)
	})

	t.Run("returns address and still requires when any stage locked without address", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(_ context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				switch aws.ToString(input.TableName) {
				case devTable:
					return &dynamodb.GetItemOutput{
						Item: map[string]dynamotypes.AttributeValue{
							"locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: true},
							"bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: ""},
						},
					}, nil
				case liveTable:
					return &dynamodb.GetItemOutput{
						Item: map[string]dynamotypes.AttributeValue{
							"locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: true},
							"bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: "0xAbC"},
						},
					}, nil
				default:
					return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
				}
			},
		}

		addr, required, err := inspectBootstrapRequirements(context.Background(), fake, app, []naming.Stage{naming.StageDev, naming.StageLive})
		require.NoError(t, err)
		require.True(t, required)
		require.Equal(t, "0xabc", addr)
	})

	t.Run("errors when multiple addresses", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(_ context.Context, input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				switch aws.ToString(input.TableName) {
				case devTable:
					return &dynamodb.GetItemOutput{
						Item: map[string]dynamotypes.AttributeValue{
							"locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: true},
							"bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: "0x1"},
						},
					}, nil
				case liveTable:
					return &dynamodb.GetItemOutput{
						Item: map[string]dynamotypes.AttributeValue{
							"locked":                 &dynamotypes.AttributeValueMemberBOOL{Value: true},
							"bootstrapWalletAddress": &dynamotypes.AttributeValueMemberS{Value: "0x2"},
						},
					}, nil
				default:
					return &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}, nil
				}
			},
		}

		_, _, err := inspectBootstrapRequirements(context.Background(), fake, app, []naming.Stage{naming.StageDev, naming.StageLive})
		require.Error(t, err)
		require.Contains(t, err.Error(), "multiple bootstrap wallet addresses")
	})

	t.Run("returns error for unexpected GetItem failure", func(t *testing.T) {
		fake := &fakeDynamoDB{
			getItemFn: func(context.Context, *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
				return nil, errors.New("boom")
			},
		}

		_, _, err := inspectBootstrapRequirements(context.Background(), fake, app, []naming.Stage{naming.StageDev})
		require.Error(t, err)
	})
}
