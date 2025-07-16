package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

func TestCreateActor_MissingUsername(t *testing.T) {
	repo := &ActorRepository{}

	actor := &activitypub.Actor{
		Name: "Test User",
	}
	privateKey := "test-private-key"

	err := repo.CreateActor(context.Background(), actor, privateKey)

	assert.Error(t, err)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestUpdateActor_MissingUsername(t *testing.T) {
	repo := &ActorRepository{}

	actor := &activitypub.Actor{
		Name: "Updated User",
	}

	err := repo.UpdateActor(context.Background(), actor)

	assert.Error(t, err)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestSearchAccounts_EmptyQuery(t *testing.T) {
	repo := &ActorRepository{}

	results, err := repo.SearchAccounts(context.Background(), "", 10, false, 0)

	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestGetSearchSuggestions_ShortPrefix(t *testing.T) {
	repo := &ActorRepository{}

	results, err := repo.GetSearchSuggestions(context.Background(), "a")

	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestConvertActorFields(t *testing.T) {
	now := time.Now()
	modelFields := []models.ActorField{
		{
			Name:       "Website",
			Value:      "https://example.com",
			VerifiedAt: &now,
		},
	}

	storageFields := convertActorFields(modelFields)

	assert.Len(t, storageFields, 1)
	assert.Equal(t, "Website", storageFields[0].Name)
	assert.Equal(t, "https://example.com", storageFields[0].Value)
	assert.Equal(t, &now, storageFields[0].VerifiedAt)
}

func TestConvertStorageActorFields(t *testing.T) {
	now := time.Now()
	storageFields := []storage.ActorField{
		{
			Name:       "Website",
			Value:      "https://example.com",
			VerifiedAt: &now,
		},
	}

	modelFields := convertStorageActorFields(storageFields)

	assert.Len(t, modelFields, 1)
	assert.Equal(t, "Website", modelFields[0].Name)
	assert.Equal(t, "https://example.com", modelFields[0].Value)
	assert.Equal(t, &now, modelFields[0].VerifiedAt)
}

func TestNewActorRepository(t *testing.T) {
	repo := NewActorRepository(nil)

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
}
