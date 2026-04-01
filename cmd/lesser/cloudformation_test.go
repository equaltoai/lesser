package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestDeployCloudFormationStack_RequiresInputs(t *testing.T) {
	_, err := deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{})
	require.ErrorContains(t, err, "cloudformation stack name is required")

	_, err = deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{
		StackName: "demo",
	})
	require.ErrorContains(t, err, "requires template body or template URL")

	_, err = deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{
		StackName:    "demo",
		TemplateBody: "{}",
		TemplateURL:  "https://example.com/template.json",
	})
	require.ErrorContains(t, err, "must use either template body or template URL")
}

func TestDeployCloudFormationStack_CreateFlow(t *testing.T) {
	restore := stubCloudFormationFns(t)

	var createInput *cloudformation.CreateStackInput
	var waitedFor string

	newCloudFormationClientFn = func(aws.Config, ...func(*cloudformation.Options)) *cloudformation.Client { return nil }
	cloudFormationStackExistsFn = func(context.Context, *cloudformation.Client, string) (bool, error) { return false, nil }
	createCloudFormationStackFn = func(_ context.Context, _ *cloudformation.Client, input *cloudformation.CreateStackInput) (*cloudformation.CreateStackOutput, error) {
		createInput = input
		return &cloudformation.CreateStackOutput{}, nil
	}
	waitCloudFormationStackCreateFn = func(_ context.Context, _ *cloudformation.Client, stackName string) error {
		waitedFor = stackName
		return nil
	}
	describeCloudFormationOutputsFn = func(context.Context, *cloudformation.Client, string) (map[string]string, error) {
		return map[string]string{"Bucket": "demo"}, nil
	}

	outputs, err := deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{
		StackName:    "demo",
		TemplateBody: `{"Resources":{}}`,
		Parameters: map[string]string{
			"HostedZoneId": "Z123",
			"AppSlug":      "demo",
		},
	})
	restore()

	require.NoError(t, err)
	require.Equal(t, map[string]string{"Bucket": "demo"}, outputs)
	require.NotNil(t, createInput)
	require.Equal(t, "demo", aws.ToString(createInput.StackName))
	require.Equal(t, `{"Resources":{}}`, aws.ToString(createInput.TemplateBody))
	require.Nil(t, createInput.TemplateURL)
	require.Equal(t, []string{"AppSlug", "HostedZoneId"}, []string{
		aws.ToString(createInput.Parameters[0].ParameterKey),
		aws.ToString(createInput.Parameters[1].ParameterKey),
	})
	require.Equal(t, "demo", aws.ToString(createInput.Parameters[0].ParameterValue))
	require.Equal(t, "Z123", aws.ToString(createInput.Parameters[1].ParameterValue))
	require.Equal(t, "demo", waitedFor)
}

func TestDeployCloudFormationStack_CreateFlowWaitError(t *testing.T) {
	restore := stubCloudFormationFns(t)

	newCloudFormationClientFn = func(aws.Config, ...func(*cloudformation.Options)) *cloudformation.Client { return nil }
	cloudFormationStackExistsFn = func(context.Context, *cloudformation.Client, string) (bool, error) { return false, nil }
	createCloudFormationStackFn = func(context.Context, *cloudformation.Client, *cloudformation.CreateStackInput) (*cloudformation.CreateStackOutput, error) {
		return &cloudformation.CreateStackOutput{}, nil
	}
	waitCloudFormationStackCreateFn = func(context.Context, *cloudformation.Client, string) error {
		return errors.New("wait failed")
	}

	_, err := deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{
		StackName:    "demo",
		TemplateBody: "{}",
	})
	restore()

	require.ErrorContains(t, err, "wait for cloudformation stack create demo")
}

func TestDeployCloudFormationStack_UpdateFlow(t *testing.T) {
	restore := stubCloudFormationFns(t)

	var updateInput *cloudformation.UpdateStackInput
	var waitedFor string

	newCloudFormationClientFn = func(aws.Config, ...func(*cloudformation.Options)) *cloudformation.Client { return nil }
	cloudFormationStackExistsFn = func(context.Context, *cloudformation.Client, string) (bool, error) { return true, nil }
	updateCloudFormationStackFn = func(_ context.Context, _ *cloudformation.Client, input *cloudformation.UpdateStackInput) (*cloudformation.UpdateStackOutput, error) {
		updateInput = input
		return &cloudformation.UpdateStackOutput{}, nil
	}
	waitCloudFormationStackUpdateFn = func(_ context.Context, _ *cloudformation.Client, stackName string) error {
		waitedFor = stackName
		return nil
	}
	describeCloudFormationOutputsFn = func(context.Context, *cloudformation.Client, string) (map[string]string, error) {
		return map[string]string{"URL": "https://example.com"}, nil
	}

	outputs, err := deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{
		StackName:   "demo",
		TemplateURL: "https://example.com/template.json",
		Parameters: map[string]string{
			"AppSlug": "demo",
		},
	})
	restore()

	require.NoError(t, err)
	require.Equal(t, map[string]string{"URL": "https://example.com"}, outputs)
	require.NotNil(t, updateInput)
	require.Equal(t, "demo", aws.ToString(updateInput.StackName))
	require.Equal(t, "https://example.com/template.json", aws.ToString(updateInput.TemplateURL))
	require.Nil(t, updateInput.TemplateBody)
	require.Equal(t, "demo", waitedFor)
}

func TestDeployCloudFormationStack_UpdateNoOpReturnsOutputs(t *testing.T) {
	restore := stubCloudFormationFns(t)

	newCloudFormationClientFn = func(aws.Config, ...func(*cloudformation.Options)) *cloudformation.Client { return nil }
	cloudFormationStackExistsFn = func(context.Context, *cloudformation.Client, string) (bool, error) { return true, nil }
	updateCloudFormationStackFn = func(context.Context, *cloudformation.Client, *cloudformation.UpdateStackInput) (*cloudformation.UpdateStackOutput, error) {
		return nil, errors.New("ValidationError: No updates are to be performed")
	}
	describeCloudFormationOutputsFn = func(context.Context, *cloudformation.Client, string) (map[string]string, error) {
		return map[string]string{"Version": "1"}, nil
	}

	outputs, err := deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{
		StackName:    "demo",
		TemplateBody: "{}",
	})
	restore()

	require.NoError(t, err)
	require.Equal(t, map[string]string{"Version": "1"}, outputs)
}

func TestDeployCloudFormationStack_PropagatesLookupErrors(t *testing.T) {
	restore := stubCloudFormationFns(t)

	newCloudFormationClientFn = func(aws.Config, ...func(*cloudformation.Options)) *cloudformation.Client { return nil }
	cloudFormationStackExistsFn = func(context.Context, *cloudformation.Client, string) (bool, error) {
		return false, errors.New("describe failed")
	}

	_, err := deployCloudFormationStack(context.Background(), aws.Config{}, cloudFormationDeployRequest{
		StackName:    "demo",
		TemplateBody: "{}",
	})
	restore()

	require.ErrorContains(t, err, "describe failed")
}

func TestCloudFormationHelpers(t *testing.T) {
	params := makeCloudFormationParameters(map[string]string{
		"HostedZoneId": "Z123",
		"AppSlug":      "demo",
	})
	require.Equal(t, []string{"AppSlug", "HostedZoneId"}, []string{
		aws.ToString(params[0].ParameterKey),
		aws.ToString(params[1].ParameterKey),
	})

	require.False(t, isCloudFormationStackNotFoundError(nil))
	require.True(t, isCloudFormationStackNotFoundError(errors.New("stack demo does not exist")))
	require.False(t, isCloudFormationStackNotFoundError(errors.New("boom")))

	require.False(t, isNoCloudFormationUpdatesError(nil))
	require.True(t, isNoCloudFormationUpdatesError(errors.New("No updates are to be performed")))
	require.False(t, isNoCloudFormationUpdatesError(errors.New("boom")))

	require.Equal(t, map[string]string{}, extractCloudFormationOutputs(nil))
	require.Equal(t, map[string]string{"Key": "Value"}, extractCloudFormationOutputs(map[string]string{"Key": "Value"}))

	require.Equal(t, "bucket", resolveReleaseAssetBucket(map[string]string{"ReleaseAssetBucketName": "bucket"}, "app", "123", "us-east-1"))
	require.Equal(t, "app-shared-release-assets-123-us-east-1", resolveReleaseAssetBucket(map[string]string{}, "app", "123", "us-east-1"))
}

func TestCreateAndUpdateCloudFormationStack(t *testing.T) {
	client := newStubCloudFormationClient(xmlHTTPResponse(200, `<CreateStackResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <CreateStackResult>
    <StackId>arn:aws:cloudformation:us-east-1:123456789012:stack/demo/1</StackId>
  </CreateStackResult>
</CreateStackResponse>`))

	createOut, err := createCloudFormationStack(context.Background(), client, &cloudformation.CreateStackInput{
		StackName: aws.String("demo"),
	})
	require.NoError(t, err)
	require.Equal(t, "arn:aws:cloudformation:us-east-1:123456789012:stack/demo/1", aws.ToString(createOut.StackId))

	client = newStubCloudFormationClient(xmlHTTPResponse(200, `<UpdateStackResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <UpdateStackResult>
    <StackId>arn:aws:cloudformation:us-east-1:123456789012:stack/demo/2</StackId>
  </UpdateStackResult>
</UpdateStackResponse>`))

	updateOut, err := updateCloudFormationStack(context.Background(), client, &cloudformation.UpdateStackInput{
		StackName: aws.String("demo"),
	})
	require.NoError(t, err)
	require.Equal(t, "arn:aws:cloudformation:us-east-1:123456789012:stack/demo/2", aws.ToString(updateOut.StackId))
}

func TestWaitCloudFormationStackCreateAndUpdate(t *testing.T) {
	createClient := newStubCloudFormationClient(xmlHTTPResponse(200, `<DescribeStacksResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <DescribeStacksResult>
    <Stacks>
      <member>
        <StackStatus>CREATE_COMPLETE</StackStatus>
      </member>
    </Stacks>
  </DescribeStacksResult>
</DescribeStacksResponse>`))
	require.NoError(t, waitCloudFormationStackCreate(context.Background(), createClient, "demo"))

	updateClient := newStubCloudFormationClient(xmlHTTPResponse(200, `<DescribeStacksResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <DescribeStacksResult>
    <Stacks>
      <member>
        <StackStatus>UPDATE_COMPLETE</StackStatus>
      </member>
    </Stacks>
  </DescribeStacksResult>
</DescribeStacksResponse>`))
	require.NoError(t, waitCloudFormationStackUpdate(context.Background(), updateClient, "demo"))
}

func TestDescribeCloudFormationOutputs_ReadsOutputs(t *testing.T) {
	restore := stubCloudFormationFns(t)
	t.Cleanup(restore)

	describeCloudFormationOutputsFn = describeCloudFormationOutputs
	newCloudFormationClientFn = func(cfg aws.Config, optFns ...func(*cloudformation.Options)) *cloudformation.Client {
		return cloudformation.NewFromConfig(aws.Config{
			Region:      "us-east-1",
			Credentials: aws.AnonymousCredentials{},
			HTTPClient: httpDoFunc(func(*http.Request) (*http.Response, error) {
				body := `<DescribeStacksResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <DescribeStacksResult>
    <Stacks>
      <member>
        <Outputs>
          <member>
            <OutputKey>Bucket</OutputKey>
            <OutputValue>demo</OutputValue>
          </member>
          <member>
            <OutputKey></OutputKey>
            <OutputValue>skip</OutputValue>
          </member>
        </Outputs>
      </member>
    </Stacks>
  </DescribeStacksResult>
</DescribeStacksResponse>`
				return xmlHTTPResponse(200, body), nil
			}),
		}, optFns...)
	}

	client := newCloudFormationClientFn(aws.Config{})
	outputs, err := describeCloudFormationOutputs(context.Background(), client, "demo")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"Bucket": "demo"}, outputs)
}

func TestDescribeCloudFormationOutputs_PropagatesDescribeError(t *testing.T) {
	client := newStubCloudFormationClient(xmlHTTPResponse(500, `<ErrorResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <Error>
    <Type>Receiver</Type>
    <Code>InternalFailure</Code>
    <Message>boom</Message>
  </Error>
</ErrorResponse>`))

	_, err := describeCloudFormationOutputs(context.Background(), client, "demo")
	require.ErrorContains(t, err, "describe cloudformation stack demo")
}

func TestCloudFormationStackExists(t *testing.T) {
	client := newStubCloudFormationClient(xmlHTTPResponse(200, `<DescribeStacksResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <DescribeStacksResult>
    <Stacks><member/></Stacks>
  </DescribeStacksResult>
</DescribeStacksResponse>`))

	exists, err := cloudFormationStackExists(context.Background(), client, "demo")
	require.NoError(t, err)
	require.True(t, exists)
}

func TestCloudFormationStackExists_NotFound(t *testing.T) {
	client := newStubCloudFormationClient(xmlHTTPResponse(400, `<ErrorResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <Error>
    <Type>Sender</Type>
    <Code>ValidationError</Code>
    <Message>Stack with id demo does not exist</Message>
  </Error>
</ErrorResponse>`))

	exists, err := cloudFormationStackExists(context.Background(), client, "demo")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestCloudFormationStackExists_PropagatesDescribeError(t *testing.T) {
	client := newStubCloudFormationClient(xmlHTTPResponse(500, `<ErrorResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <Error>
    <Type>Receiver</Type>
    <Code>InternalFailure</Code>
    <Message>boom</Message>
  </Error>
</ErrorResponse>`))

	exists, err := cloudFormationStackExists(context.Background(), client, "demo")
	require.ErrorContains(t, err, "describe cloudformation stack demo")
	require.False(t, exists)
}

func TestDescribeCloudFormationOutputs_ErrorsWhenStackMissingAfterDeploy(t *testing.T) {
	client := newStubCloudFormationClient(xmlHTTPResponse(200, `<DescribeStacksResponse xmlns="http://cloudformation.amazonaws.com/doc/2010-05-15/">
  <DescribeStacksResult>
    <Stacks></Stacks>
  </DescribeStacksResult>
</DescribeStacksResponse>`))

	_, err := describeCloudFormationOutputs(context.Background(), client, "demo")
	require.ErrorContains(t, err, "not found after deploy")
}

func TestPresignReleaseAssemblyURL(t *testing.T) {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("akid", "secret", ""),
	}
	client := s3.NewPresignClient(s3.NewFromConfig(cfg))

	url, err := presignReleaseAssemblyURL(context.Background(), client, "bucket", "templates/demo.json")
	require.NoError(t, err)
	require.Contains(t, url, "bucket")
	require.Contains(t, url, "templates/demo.json")
}

func TestPutS3Object(t *testing.T) {
	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("akid", "secret", ""),
		HTTPClient: httpDoFunc(func(req *http.Request) (*http.Response, error) {
			return xmlHTTPResponse(200, ``), nil
		}),
	})

	_, err := putS3Object(context.Background(), client, &s3.PutObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("key"),
		Body:   strings.NewReader("body"),
	})
	require.NoError(t, err)
}

func stubCloudFormationFns(t *testing.T) func() {
	t.Helper()

	previousNewClient := newCloudFormationClientFn
	previousExists := cloudFormationStackExistsFn
	previousDescribe := describeCloudFormationOutputsFn
	previousCreate := createCloudFormationStackFn
	previousUpdate := updateCloudFormationStackFn
	previousWaitCreate := waitCloudFormationStackCreateFn
	previousWaitUpdate := waitCloudFormationStackUpdateFn

	return func() {
		newCloudFormationClientFn = previousNewClient
		cloudFormationStackExistsFn = previousExists
		describeCloudFormationOutputsFn = previousDescribe
		createCloudFormationStackFn = previousCreate
		updateCloudFormationStackFn = previousUpdate
		waitCloudFormationStackCreateFn = previousWaitCreate
		waitCloudFormationStackUpdateFn = previousWaitUpdate
	}
}

type httpDoFunc func(*http.Request) (*http.Response, error)

func (fn httpDoFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newStubCloudFormationClient(resp *http.Response) *cloudformation.Client {
	return cloudformation.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.AnonymousCredentials{},
		HTTPClient: httpDoFunc(func(*http.Request) (*http.Response, error) {
			return resp, nil
		}),
	})
}

func xmlHTTPResponse(status int, body string) *http.Response {
	headers := make(http.Header)
	headers.Set("Content-Type", "text/xml")
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
