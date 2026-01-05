package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSS3StorageClient_getContentType(t *testing.T) {
	client := &AWSS3StorageClient{}

	assert.Equal(t, "application/gzip", client.getContentType("backup.tar.gz"))
	assert.Equal(t, "application/zip", client.getContentType("backup.ZIP"))
	assert.Equal(t, "application/x-tar", client.getContentType("backup.tar"))
	assert.Equal(t, "text/csv", client.getContentType("export.csv"))
	assert.Equal(t, "application/json", client.getContentType("report.json"))
	assert.Equal(t, "application/octet-stream", client.getContentType("unknown.bin"))
}

func TestAWSS3StorageClient_UploadFile_InputValidation(t *testing.T) {
	client := &AWSS3StorageClient{}

	err := client.UploadFile(context.Background(), "", []byte("data"))
	require.Error(t, err)

	err = client.UploadFile(context.Background(), "key", []byte{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotUploadEmptyData)
}

func TestAWSS3StorageClient_GetFile_InputValidation(t *testing.T) {
	client := &AWSS3StorageClient{}

	_, err := client.GetFile(context.Background(), "")
	require.Error(t, err)
}
