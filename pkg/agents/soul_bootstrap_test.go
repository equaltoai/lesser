package agents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSoulBootstrapMetadataEncodeDecodeDefaulting(t *testing.T) {
	issuedAt := time.Date(2026, 6, 12, 15, 30, 0, 0, time.UTC)
	updatedAt := issuedAt.Add(2 * time.Minute)

	metadata, err := SetDroneWorkflowMetadata(nil, &DroneWorkflowState{
		CurrentPhase: DroneWorkflowPhaseSigning,
		CurrentState: DroneWorkflowStateSigningPending,
		SoulBootstrap: &SoulBootstrapState{
			Username:           " drone-alpha ",
			BodyID:             " body-alpha ",
			HostRegistrationID: " reg_123 ",
			HostConversationID: " conv_456 ",
			HostSoulAgentID:    " 0xsoul ",
			WalletAddress:      " 0xwallet ",
			PrincipalAddress:   " 0xprincipal ",
			Phase:              SoulBootstrapPhasePrincipalDeclaration,
			SigningCheckpoints: []SoulBootstrapSigningCheckpoint{
				{
					Name:             " principal_declaration ",
					Status:           " ready ",
					PrincipalAddress: " 0xprincipal ",
					SignerAddress:    " 0xprincipal ",
					SigningMethod:    " eip191_personal_sign ",
					MessageEncoding:  " hex_bytes ",
					MessageHex:       " 0xabc ",
					DigestHex:        " 0xabc ",
					CanonicalJSON:    `{"agent":"drone-alpha"}`,
					HostRequestID:    " req-host ",
					IssuedAt:         &issuedAt,
				},
			},
			Error: &SoulBootstrapErrorState{
				Code:          " soul_instance.conflict ",
				Message:       " already registered ",
				Source:        " host ",
				StatusCode:    409,
				HostRequestID: " req-host ",
				At:            &updatedAt,
			},
			Correlation: &SoulBootstrapCorrelationState{
				CorrelationKey:                     " corr-1 ",
				BeginIdempotencyKey:                " begin-1 ",
				PrincipalDeclarationIdempotencyKey: " principal-1 ",
				LastHostRequestID:                  " req-host ",
			},
			UpdatedAt: &updatedAt,
		},
	})
	require.NoError(t, err)

	decoded, err := ParseDroneWorkflowMetadata(metadata)
	require.NoError(t, err)
	require.NotNil(t, decoded)
	require.NotNil(t, decoded.SoulBootstrap)
	require.Equal(t, "drone-alpha", decoded.SoulBootstrap.Username)
	require.Equal(t, "body-alpha", decoded.SoulBootstrap.BodyID)
	require.Equal(t, "reg_123", decoded.SoulBootstrap.HostRegistrationID)
	require.Equal(t, "conv_456", decoded.SoulBootstrap.HostConversationID)
	require.Equal(t, "0xsoul", decoded.SoulBootstrap.HostSoulAgentID)
	require.Equal(t, SoulBootstrapPhasePrincipalDeclaration, decoded.SoulBootstrap.Phase)
	require.Equal(t, "principal_declaration.pending", decoded.SoulBootstrap.State)
	require.Len(t, decoded.SoulBootstrap.SigningCheckpoints, 1)
	require.Equal(t, "principal_declaration", decoded.SoulBootstrap.SigningCheckpoints[0].Name)
	require.Equal(t, "0xabc", decoded.SoulBootstrap.SigningCheckpoints[0].DigestHex)
	require.Equal(t, "soul_instance.conflict", decoded.SoulBootstrap.Error.Code)
	require.Equal(t, "corr-1", decoded.SoulBootstrap.Correlation.CorrelationKey)
	require.Equal(t, updatedAt, *decoded.SoulBootstrap.UpdatedAt)
}

func TestSoulBootstrapDefaultAndUnavailableState(t *testing.T) {
	defaultState := NormalizeSoulBootstrap(nil, "drone-beta")
	require.NotNil(t, defaultState)
	require.Equal(t, "drone-beta", defaultState.Username)
	require.Equal(t, "drone-beta", defaultState.BodyID)
	require.Equal(t, SoulBootstrapPhaseNotStarted, defaultState.Phase)
	require.Equal(t, SoulBootstrapStateNotStarted, defaultState.State)

	now := time.Date(2026, 6, 12, 16, 0, 0, 0, time.UTC)
	unavailable := NewSoulBootstrapHostBridgeUnavailableState(
		"drone-beta",
		&SoulBootstrapCorrelationState{CorrelationKey: " corr-2 ", BeginIdempotencyKey: " begin-2 "},
		now,
	)
	require.Equal(t, SoulBootstrapPhaseError, unavailable.Phase)
	require.Equal(t, SoulBootstrapStateHostBridgeUnavailable, unavailable.State)
	require.NotNil(t, unavailable.Error)
	require.Equal(t, SoulBootstrapErrorHostBridgeUnavailable, unavailable.Error.Code)
	require.Equal(t, "lesser", unavailable.Error.Source)
	require.Equal(t, "corr-2", unavailable.Correlation.CorrelationKey)
	require.Equal(t, "begin-2", unavailable.Correlation.BeginIdempotencyKey)
	require.Equal(t, now, *unavailable.UpdatedAt)
}
