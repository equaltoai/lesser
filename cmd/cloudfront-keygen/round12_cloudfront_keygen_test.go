package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/require"
)

type fakeSecretsManager struct {
	createCalls   int
	putCalls      int
	describeCalls int

	lastCreate   *secretsmanager.CreateSecretInput
	lastPut      *secretsmanager.PutSecretValueInput
	lastDescribe *secretsmanager.DescribeSecretInput

	createOut   *secretsmanager.CreateSecretOutput
	describeOut *secretsmanager.DescribeSecretOutput

	createErr   error
	putErr      error
	describeErr error
}

func (f *fakeSecretsManager) CreateSecret(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	f.createCalls++
	f.lastCreate = params
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createOut != nil {
		return f.createOut, nil
	}
	arn := "arn:aws:secretsmanager:us-east-1:123:secret:test"
	return &secretsmanager.CreateSecretOutput{ARN: &arn}, nil
}

func (f *fakeSecretsManager) PutSecretValue(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	f.putCalls++
	f.lastPut = params
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &secretsmanager.PutSecretValueOutput{}, nil
}

func (f *fakeSecretsManager) DescribeSecret(_ context.Context, params *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	f.describeCalls++
	f.lastDescribe = params
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	if f.describeOut != nil {
		return f.describeOut, nil
	}
	arn := "arn:aws:secretsmanager:us-east-1:123:secret:existing"
	return &secretsmanager.DescribeSecretOutput{ARN: &arn}, nil
}

type fakeCloudFront struct {
	listPublicKeysCalls int
	listKeyGroupsCalls  int

	getPublicKeyConfigCalls int
	getKeyGroupConfigCalls  int
	updatePublicKeyCalls    int
	updateKeyGroupCalls     int
	createPublicKeyCalls    int
	createKeyGroupCalls     int

	lastUpdatePublicKey *cloudfront.UpdatePublicKeyInput
	lastUpdateKeyGroup  *cloudfront.UpdateKeyGroupInput
	lastCreatePublicKey *cloudfront.CreatePublicKeyInput
	lastCreateKeyGroup  *cloudfront.CreateKeyGroupInput

	listPublicKeysOutputs []*cloudfront.ListPublicKeysOutput
	listKeyGroupsOutputs  []*cloudfront.ListKeyGroupsOutput

	getPublicKeyConfigOut *cloudfront.GetPublicKeyConfigOutput
	getKeyGroupConfigOut  *cloudfront.GetKeyGroupConfigOutput

	createPublicKeyOut *cloudfront.CreatePublicKeyOutput
	createKeyGroupOut  *cloudfront.CreateKeyGroupOutput

	listPublicKeysErr     error
	listKeyGroupsErr      error
	getPublicKeyConfigErr error
	getKeyGroupConfigErr  error
	updatePublicKeyErr    error
	updateKeyGroupErr     error
	createPublicKeyErr    error
	createKeyGroupErr     error
}

func (f *fakeCloudFront) ListPublicKeys(_ context.Context, _ *cloudfront.ListPublicKeysInput, _ ...func(*cloudfront.Options)) (*cloudfront.ListPublicKeysOutput, error) {
	f.listPublicKeysCalls++
	if f.listPublicKeysErr != nil {
		return nil, f.listPublicKeysErr
	}
	if len(f.listPublicKeysOutputs) > 0 {
		out := f.listPublicKeysOutputs[0]
		f.listPublicKeysOutputs = f.listPublicKeysOutputs[1:]
		return out, nil
	}
	zero := int32(0)
	return &cloudfront.ListPublicKeysOutput{PublicKeyList: &cftypes.PublicKeyList{MaxItems: &zero, Quantity: &zero}}, nil
}

func (f *fakeCloudFront) GetPublicKeyConfig(_ context.Context, _ *cloudfront.GetPublicKeyConfigInput, _ ...func(*cloudfront.Options)) (*cloudfront.GetPublicKeyConfigOutput, error) {
	f.getPublicKeyConfigCalls++
	if f.getPublicKeyConfigErr != nil {
		return nil, f.getPublicKeyConfigErr
	}
	return f.getPublicKeyConfigOut, nil
}

func (f *fakeCloudFront) UpdatePublicKey(_ context.Context, params *cloudfront.UpdatePublicKeyInput, _ ...func(*cloudfront.Options)) (*cloudfront.UpdatePublicKeyOutput, error) {
	f.updatePublicKeyCalls++
	f.lastUpdatePublicKey = params
	if f.updatePublicKeyErr != nil {
		return nil, f.updatePublicKeyErr
	}
	return &cloudfront.UpdatePublicKeyOutput{}, nil
}

func (f *fakeCloudFront) CreatePublicKey(_ context.Context, params *cloudfront.CreatePublicKeyInput, _ ...func(*cloudfront.Options)) (*cloudfront.CreatePublicKeyOutput, error) {
	f.createPublicKeyCalls++
	f.lastCreatePublicKey = params
	if f.createPublicKeyErr != nil {
		return nil, f.createPublicKeyErr
	}
	if f.createPublicKeyOut != nil {
		return f.createPublicKeyOut, nil
	}
	id := "pkid"
	return &cloudfront.CreatePublicKeyOutput{PublicKey: &cftypes.PublicKey{Id: &id}}, nil
}

func (f *fakeCloudFront) ListKeyGroups(_ context.Context, _ *cloudfront.ListKeyGroupsInput, _ ...func(*cloudfront.Options)) (*cloudfront.ListKeyGroupsOutput, error) {
	f.listKeyGroupsCalls++
	if f.listKeyGroupsErr != nil {
		return nil, f.listKeyGroupsErr
	}
	if len(f.listKeyGroupsOutputs) > 0 {
		out := f.listKeyGroupsOutputs[0]
		f.listKeyGroupsOutputs = f.listKeyGroupsOutputs[1:]
		return out, nil
	}
	zero := int32(0)
	return &cloudfront.ListKeyGroupsOutput{KeyGroupList: &cftypes.KeyGroupList{MaxItems: &zero, Quantity: &zero}}, nil
}

func (f *fakeCloudFront) GetKeyGroupConfig(_ context.Context, _ *cloudfront.GetKeyGroupConfigInput, _ ...func(*cloudfront.Options)) (*cloudfront.GetKeyGroupConfigOutput, error) {
	f.getKeyGroupConfigCalls++
	if f.getKeyGroupConfigErr != nil {
		return nil, f.getKeyGroupConfigErr
	}
	return f.getKeyGroupConfigOut, nil
}

func (f *fakeCloudFront) UpdateKeyGroup(_ context.Context, params *cloudfront.UpdateKeyGroupInput, _ ...func(*cloudfront.Options)) (*cloudfront.UpdateKeyGroupOutput, error) {
	f.updateKeyGroupCalls++
	f.lastUpdateKeyGroup = params
	if f.updateKeyGroupErr != nil {
		return nil, f.updateKeyGroupErr
	}
	return &cloudfront.UpdateKeyGroupOutput{}, nil
}

func (f *fakeCloudFront) CreateKeyGroup(_ context.Context, params *cloudfront.CreateKeyGroupInput, _ ...func(*cloudfront.Options)) (*cloudfront.CreateKeyGroupOutput, error) {
	f.createKeyGroupCalls++
	f.lastCreateKeyGroup = params
	if f.createKeyGroupErr != nil {
		return nil, f.createKeyGroupErr
	}
	if f.createKeyGroupOut != nil {
		return f.createKeyGroupOut, nil
	}
	id := "kgid"
	return &cloudfront.CreateKeyGroupOutput{KeyGroup: &cftypes.KeyGroup{Id: &id}}, nil
}

func TestHandler_MissingSecretName_Round12(t *testing.T) {
	_, _, err := handler(context.Background(), cfn.Event{
		RequestType:        cfn.RequestCreate,
		LogicalResourceID:  "Res",
		ResourceProperties: map[string]interface{}{},
	})
	require.Error(t, err)
}

func TestHandler_DeleteEvent_Round12(t *testing.T) {
	physical, data, err := handler(context.Background(), cfn.Event{
		RequestType:        cfn.RequestDelete,
		LogicalResourceID:  "Res",
		PhysicalResourceID: "existing-physical",
		ResourceProperties: map[string]interface{}{
			"SecretName": "secret",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "existing-physical", physical)
	require.Nil(t, data)
}

func TestHandler_CreateSecret_Success_Round12(t *testing.T) {
	origLoad := loadAWSConfigFn
	origNewSecrets := newSecretsManagerClientFn
	origNewCF := newCloudFrontClientFn
	origRSA := rsaGenerateKeyFn
	origEnsure := ensureCloudFrontResourcesFn
	t.Cleanup(func() {
		loadAWSConfigFn = origLoad
		newSecretsManagerClientFn = origNewSecrets
		newCloudFrontClientFn = origNewCF
		rsaGenerateKeyFn = origRSA
		ensureCloudFrontResourcesFn = origEnsure
	})

	secrets := &fakeSecretsManager{createOut: &secretsmanager.CreateSecretOutput{ARN: aws.String("arn:secret")}}
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return secrets }
	newCloudFrontClientFn = func(aws.Config) cloudFrontAPI { return nil }
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	rsaGenerateKeyFn = func(r io.Reader, _ int) (*rsa.PrivateKey, error) { return rsa.GenerateKey(r, 1024) }

	var gotKeyName, gotKeyGroupName, gotKey string
	ensureCloudFrontResourcesFn = func(_ context.Context, _ cloudFrontAPI, keyName, keyGroupName, encodedKey string) (string, string, error) {
		gotKeyName = keyName
		gotKeyGroupName = keyGroupName
		gotKey = encodedKey
		return "pk", "kg", nil
	}

	physical, data, err := handler(context.Background(), cfn.Event{
		RequestType:       cfn.RequestCreate,
		LogicalResourceID: "Res",
		ResourceProperties: map[string]interface{}{
			"SecretName": "secret",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "arn:secret", physical)
	require.Equal(t, "lesser-Res-key", gotKeyName)
	require.Equal(t, "lesser-Res-keygroup", gotKeyGroupName)
	require.NotEmpty(t, gotKey)
	require.Equal(t, 1, secrets.createCalls)
	require.Contains(t, data, "PublicKeyId")
	require.Contains(t, data, "KeyGroupId")
}

func TestHandler_CreateSecret_ResourceExists_Round12(t *testing.T) {
	origLoad := loadAWSConfigFn
	origNewSecrets := newSecretsManagerClientFn
	origNewCF := newCloudFrontClientFn
	origRSA := rsaGenerateKeyFn
	origEnsure := ensureCloudFrontResourcesFn
	t.Cleanup(func() {
		loadAWSConfigFn = origLoad
		newSecretsManagerClientFn = origNewSecrets
		newCloudFrontClientFn = origNewCF
		rsaGenerateKeyFn = origRSA
		ensureCloudFrontResourcesFn = origEnsure
	})

	secrets := &fakeSecretsManager{
		createErr:   &smtypes.ResourceExistsException{Message: aws.String("exists")},
		describeOut: &secretsmanager.DescribeSecretOutput{ARN: aws.String("arn:existing")},
	}
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return secrets }
	newCloudFrontClientFn = func(aws.Config) cloudFrontAPI { return nil }
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	rsaGenerateKeyFn = func(r io.Reader, _ int) (*rsa.PrivateKey, error) { return rsa.GenerateKey(r, 1024) }
	ensureCloudFrontResourcesFn = func(context.Context, cloudFrontAPI, string, string, string) (string, string, error) {
		return "pk", "kg", nil
	}

	physical, data, err := handler(context.Background(), cfn.Event{
		RequestType:       cfn.RequestUpdate,
		LogicalResourceID: "Res",
		ResourceProperties: map[string]interface{}{
			"SecretName": "secret",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "arn:existing", physical)
	require.Equal(t, 1, secrets.putCalls)
	require.Equal(t, 1, secrets.describeCalls)
	require.Contains(t, data, "SecretArn")
}

func TestHandler_CreateSecret_CreateFails_Round12(t *testing.T) {
	origLoad := loadAWSConfigFn
	origNewSecrets := newSecretsManagerClientFn
	origNewCF := newCloudFrontClientFn
	origRSA := rsaGenerateKeyFn
	origEnsure := ensureCloudFrontResourcesFn
	t.Cleanup(func() {
		loadAWSConfigFn = origLoad
		newSecretsManagerClientFn = origNewSecrets
		newCloudFrontClientFn = origNewCF
		rsaGenerateKeyFn = origRSA
		ensureCloudFrontResourcesFn = origEnsure
	})

	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI {
		return &fakeSecretsManager{createErr: errors.New("boom")}
	}
	newCloudFrontClientFn = func(aws.Config) cloudFrontAPI { return nil }
	rsaGenerateKeyFn = func(r io.Reader, _ int) (*rsa.PrivateKey, error) { return rsa.GenerateKey(r, 1024) }
	ensureCloudFrontResourcesFn = func(context.Context, cloudFrontAPI, string, string, string) (string, string, error) {
		return "pk", "kg", nil
	}
	_, _, err := handler(context.Background(), cfn.Event{
		RequestType:       cfn.RequestCreate,
		LogicalResourceID: "Res",
		ResourceProperties: map[string]interface{}{
			"SecretName": "secret",
		},
	})
	require.Error(t, err)
}

func TestCloudFront_Upserts_Round12(t *testing.T) {
	client := &fakeCloudFront{}
	publicKeyID, keyGroupID, err := ensureCloudFrontResources(context.Background(), client, "key", "group", "encoded")
	require.NoError(t, err)
	require.Equal(t, "pkid", publicKeyID)
	require.Equal(t, "kgid", keyGroupID)
	require.Equal(t, 1, client.createPublicKeyCalls)
	require.Equal(t, 1, client.createKeyGroupCalls)
}

func TestCloudFront_UpdatePaths_Round12(t *testing.T) {
	now := time.Now().UTC()
	max := int32(1)
	qty := int32(1)
	keyID := "existing-pk"
	keyName := "key"
	etag := "etag"
	encoded := "old"

	client := &fakeCloudFront{
		listPublicKeysOutputs: []*cloudfront.ListPublicKeysOutput{
			{
				PublicKeyList: &cftypes.PublicKeyList{
					MaxItems: &max,
					Quantity: &qty,
					Items: []cftypes.PublicKeySummary{
						{Id: &keyID, Name: &keyName, EncodedKey: &encoded, CreatedTime: &now},
					},
				},
			},
		},
		getPublicKeyConfigOut: &cloudfront.GetPublicKeyConfigOutput{
			ETag: &etag,
			PublicKeyConfig: &cftypes.PublicKeyConfig{
				CallerReference: aws.String("ref"),
				Name:            aws.String(keyName),
				EncodedKey:      aws.String(encoded),
			},
		},
		listKeyGroupsOutputs: []*cloudfront.ListKeyGroupsOutput{
			{
				KeyGroupList: &cftypes.KeyGroupList{
					MaxItems: &max,
					Quantity: &qty,
					Items: []cftypes.KeyGroupSummary{
						{
							KeyGroup: &cftypes.KeyGroup{
								Id: aws.String("existing-kg"),
								KeyGroupConfig: &cftypes.KeyGroupConfig{
									Name:  aws.String("group"),
									Items: []string{"old"},
								},
								LastModifiedTime: &now,
							},
						},
					},
				},
			},
		},
		getKeyGroupConfigOut: &cloudfront.GetKeyGroupConfigOutput{
			ETag: &etag,
			KeyGroupConfig: &cftypes.KeyGroupConfig{
				Name:  aws.String("group"),
				Items: []string{"old"},
			},
		},
	}

	publicKeyID, err := upsertPublicKey(context.Background(), client, "key", "new-encoded")
	require.NoError(t, err)
	require.Equal(t, "existing-pk", publicKeyID)
	require.Equal(t, 1, client.updatePublicKeyCalls)
	require.NotNil(t, client.lastUpdatePublicKey)

	keyGroupID, err := upsertKeyGroup(context.Background(), client, "group", "existing-pk")
	require.NoError(t, err)
	require.Equal(t, "existing-kg", keyGroupID)
	require.Equal(t, 1, client.updateKeyGroupCalls)
	require.NotNil(t, client.lastUpdateKeyGroup)
}

func TestStringPtr_Round12(t *testing.T) {
	require.Equal(t, "x", *stringPtr("x"))
}

func TestFindPublicKeyByName_PaginatesAndFinds_Round12(t *testing.T) {
	now := time.Now().UTC()
	max := int32(1)
	zero := int32(0)

	keyID := "pk"
	keyName := "key"
	etag := "etag"

	client := &fakeCloudFront{
		listPublicKeysOutputs: []*cloudfront.ListPublicKeysOutput{
			{PublicKeyList: &cftypes.PublicKeyList{MaxItems: &max, Quantity: &zero, NextMarker: aws.String("next")}},
			{
				PublicKeyList: &cftypes.PublicKeyList{
					MaxItems: &max,
					Quantity: &max,
					Items: []cftypes.PublicKeySummary{
						{Id: &keyID, Name: &keyName, EncodedKey: aws.String("k"), CreatedTime: &now},
					},
				},
			},
		},
		getPublicKeyConfigOut: &cloudfront.GetPublicKeyConfigOutput{
			ETag: &etag,
			PublicKeyConfig: &cftypes.PublicKeyConfig{
				Name:       aws.String(keyName),
				EncodedKey: aws.String("k"),
			},
		},
	}

	id, cfg, gotETag, err := findPublicKeyByName(context.Background(), client, "key")
	require.NoError(t, err)
	require.Equal(t, "pk", id)
	require.NotNil(t, cfg)
	require.Equal(t, "etag", aws.ToString(gotETag))
	require.Equal(t, 2, client.listPublicKeysCalls)
	require.Equal(t, 1, client.getPublicKeyConfigCalls)
}

func TestFindPublicKeyByName_Errors_Round12(t *testing.T) {
	client := &fakeCloudFront{listPublicKeysErr: errors.New("boom")}
	_, _, _, err := findPublicKeyByName(context.Background(), client, "key")
	require.Error(t, err)

	now := time.Now().UTC()
	max := int32(1)
	qty := int32(1)
	keyID := "pk"
	keyName := "key"
	client = &fakeCloudFront{
		listPublicKeysOutputs: []*cloudfront.ListPublicKeysOutput{
			{
				PublicKeyList: &cftypes.PublicKeyList{
					MaxItems: &max,
					Quantity: &qty,
					Items: []cftypes.PublicKeySummary{
						{Id: &keyID, Name: &keyName, EncodedKey: aws.String("k"), CreatedTime: &now},
					},
				},
			},
		},
		getPublicKeyConfigErr: errors.New("boom"),
	}
	_, _, _, err = findPublicKeyByName(context.Background(), client, "key")
	require.Error(t, err)
}

func TestFindKeyGroupByName_PaginatesAndFinds_Round12(t *testing.T) {
	now := time.Now().UTC()
	max := int32(1)
	zero := int32(0)

	keyGroupName := "group"
	keyGroupID := "kg"
	etag := "etag"

	client := &fakeCloudFront{
		listKeyGroupsOutputs: []*cloudfront.ListKeyGroupsOutput{
			{KeyGroupList: &cftypes.KeyGroupList{MaxItems: &max, Quantity: &zero, NextMarker: aws.String("next")}},
			{
				KeyGroupList: &cftypes.KeyGroupList{
					MaxItems: &max,
					Quantity: &max,
					Items: []cftypes.KeyGroupSummary{
						{
							KeyGroup: &cftypes.KeyGroup{
								Id: aws.String(keyGroupID),
								KeyGroupConfig: &cftypes.KeyGroupConfig{
									Name:  aws.String(keyGroupName),
									Items: []string{"pk"},
								},
								LastModifiedTime: &now,
							},
						},
					},
				},
			},
		},
		getKeyGroupConfigOut: &cloudfront.GetKeyGroupConfigOutput{
			ETag: &etag,
			KeyGroupConfig: &cftypes.KeyGroupConfig{
				Name:  aws.String(keyGroupName),
				Items: []string{"pk"},
			},
		},
	}

	id, cfg, gotETag, err := findKeyGroupByName(context.Background(), client, "group")
	require.NoError(t, err)
	require.Equal(t, "kg", id)
	require.NotNil(t, cfg)
	require.Equal(t, "etag", aws.ToString(gotETag))
	require.Equal(t, 2, client.listKeyGroupsCalls)
	require.Equal(t, 1, client.getKeyGroupConfigCalls)
}

func TestFindKeyGroupByName_Errors_Round12(t *testing.T) {
	client := &fakeCloudFront{listKeyGroupsErr: errors.New("boom")}
	_, _, _, err := findKeyGroupByName(context.Background(), client, "group")
	require.Error(t, err)

	now := time.Now().UTC()
	max := int32(1)
	qty := int32(1)
	client = &fakeCloudFront{
		listKeyGroupsOutputs: []*cloudfront.ListKeyGroupsOutput{
			{
				KeyGroupList: &cftypes.KeyGroupList{
					MaxItems: &max,
					Quantity: &qty,
					Items: []cftypes.KeyGroupSummary{
						{
							KeyGroup: &cftypes.KeyGroup{
								Id: aws.String("kg"),
								KeyGroupConfig: &cftypes.KeyGroupConfig{
									Name:  aws.String("group"),
									Items: []string{"pk"},
								},
								LastModifiedTime: &now,
							},
						},
					},
				},
			},
		},
		getKeyGroupConfigErr: errors.New("boom"),
	}
	_, _, _, err = findKeyGroupByName(context.Background(), client, "group")
	require.Error(t, err)
}

func TestEnsureCloudFrontResources_ErrorPaths_Round12(t *testing.T) {
	_, _, err := ensureCloudFrontResources(context.Background(), &fakeCloudFront{listPublicKeysErr: errors.New("boom")}, "key", "group", "encoded")
	require.Error(t, err)

	client := &fakeCloudFront{listKeyGroupsErr: errors.New("boom")}
	_, _, err = ensureCloudFrontResources(context.Background(), client, "key", "group", "encoded")
	require.Error(t, err)
}

func TestUpsertPublicKey_Errors_Round12(t *testing.T) {
	now := time.Now().UTC()
	max := int32(1)
	qty := int32(1)
	keyID := "existing-pk"
	keyName := "key"
	etag := "etag"

	client := &fakeCloudFront{
		listPublicKeysOutputs: []*cloudfront.ListPublicKeysOutput{
			{
				PublicKeyList: &cftypes.PublicKeyList{
					MaxItems: &max,
					Quantity: &qty,
					Items: []cftypes.PublicKeySummary{
						{Id: &keyID, Name: &keyName, EncodedKey: aws.String("k"), CreatedTime: &now},
					},
				},
			},
		},
		getPublicKeyConfigOut: &cloudfront.GetPublicKeyConfigOutput{
			ETag: &etag,
			PublicKeyConfig: &cftypes.PublicKeyConfig{
				Name:       aws.String(keyName),
				EncodedKey: aws.String("k"),
			},
		},
		updatePublicKeyErr: errors.New("boom"),
	}
	_, err := upsertPublicKey(context.Background(), client, "key", "new")
	require.Error(t, err)

	client = &fakeCloudFront{createPublicKeyErr: errors.New("boom")}
	_, err = upsertPublicKey(context.Background(), client, "key", "new")
	require.Error(t, err)
}

func TestUpsertKeyGroup_Errors_Round12(t *testing.T) {
	now := time.Now().UTC()
	max := int32(1)
	qty := int32(1)
	etag := "etag"

	client := &fakeCloudFront{
		listKeyGroupsOutputs: []*cloudfront.ListKeyGroupsOutput{
			{
				KeyGroupList: &cftypes.KeyGroupList{
					MaxItems: &max,
					Quantity: &qty,
					Items: []cftypes.KeyGroupSummary{
						{
							KeyGroup: &cftypes.KeyGroup{
								Id: aws.String("existing-kg"),
								KeyGroupConfig: &cftypes.KeyGroupConfig{
									Name:  aws.String("group"),
									Items: []string{"old"},
								},
								LastModifiedTime: &now,
							},
						},
					},
				},
			},
		},
		getKeyGroupConfigOut: &cloudfront.GetKeyGroupConfigOutput{
			ETag: &etag,
			KeyGroupConfig: &cftypes.KeyGroupConfig{
				Name:  aws.String("group"),
				Items: []string{"old"},
			},
		},
		updateKeyGroupErr: errors.New("boom"),
	}
	_, err := upsertKeyGroup(context.Background(), client, "group", "pk")
	require.Error(t, err)

	client = &fakeCloudFront{createKeyGroupErr: errors.New("boom")}
	_, err = upsertKeyGroup(context.Background(), client, "group", "pk")
	require.Error(t, err)
}
