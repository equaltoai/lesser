package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestViewerRole_Round12(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := resolver.Query()

	adminRole, err := q.ViewerRole(round12AuthContext("admin"))
	require.NoError(t, err)
	require.True(t, adminRole.IsAdmin)
	require.NotEmpty(t, adminRole.Role)

	userRole, err := q.ViewerRole(round12AuthContext("alice"))
	require.NoError(t, err)
	require.False(t, userRole.IsAdmin)
	require.NotEmpty(t, userRole.Role)
}

func TestAdminInstanceConfigRequiresAdmin_Round12(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := resolver.Query()

	_, err := q.AdminInstanceConfig(round12AuthContext("alice"))
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
	require.Equal(t, 403, apperrors.GetHTTPStatus(err))
}

func TestUpdateAdminInstanceManagedDefaults_ValidatesTrustURL_Round12(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()

	badURL := "https://abc.lambda-url.us-east-1.on.aws"
	_, err := mut.UpdateAdminInstanceManagedDefaults(round12AuthContext("admin"), model.UpdateAdminInstanceManagedDefaultsInput{
		Trust: &model.AdminTrustConfigPatchInput{
			BaseURL: &badURL,
		},
	})
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeBadRequest))
}

func TestUpdateAdminInstanceOverrides_ValidatesTipsEnablement_Round12(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()

	enabled := true
	_, err := mut.UpdateAdminInstanceOverrides(round12AuthContext("admin"), model.UpdateAdminInstanceOverridesInput{
		Tips: &model.AdminTipsConfigPatchInput{
			Enabled: &enabled,
		},
	})
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeValidationFailed))
}

func TestUpdateAdminInstanceManagedDefaults_SucceedsForAdmin_Round12(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()

	enabled := true
	out, err := mut.UpdateAdminInstanceManagedDefaults(round12AuthContext("admin"), model.UpdateAdminInstanceManagedDefaultsInput{
		Translation: &model.AdminTranslationConfigPatchInput{
			Enabled: &enabled,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.Translation)
}
