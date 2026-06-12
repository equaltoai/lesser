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

func TestSoulBootstrapErrorStateAndCheckpointMetadata(t *testing.T) {
	now := time.Date(2026, 6, 12, 17, 0, 0, 0, time.UTC)
	state := NewSoulBootstrapErrorState(
		"drone-gamma",
		&SoulBootstrapCorrelationState{CorrelationKey: " corr-3 "},
		SoulBootstrapErrorHostInstanceKeyMissing,
		" missing key ",
		" lesser ",
		0,
		" host-req ",
		now,
	)
	require.Equal(t, SoulBootstrapPhaseError, state.Phase)
	require.Equal(t, SoulBootstrapStateHostInstanceKeyMissing, state.State)
	require.Equal(t, SoulBootstrapErrorHostInstanceKeyMissing, state.Error.Code)
	require.Equal(t, "missing key", state.Error.Message)
	require.Equal(t, "lesser", state.Error.Source)
	require.Equal(t, "host-req", state.Error.HostRequestID)
	require.Equal(t, "corr-3", state.Correlation.CorrelationKey)

	normalized := NormalizeSoulBootstrap(&SoulBootstrapState{
		Username: "drone-gamma",
		SigningCheckpoints: []SoulBootstrapSigningCheckpoint{{
			Version:         " 1 ",
			Name:            " wallet ",
			MessageEncoding: " utf8 ",
			Message:         " sign me ",
		}},
	}, "")
	require.Len(t, normalized.SigningCheckpoints, 1)
	require.Equal(t, "1", normalized.SigningCheckpoints[0].Version)
	require.Equal(t, "wallet", normalized.SigningCheckpoints[0].Name)
	require.Equal(t, "utf8", normalized.SigningCheckpoints[0].MessageEncoding)
	require.Equal(t, "sign me", normalized.SigningCheckpoints[0].Message)
}

func TestSoulBootstrapM23PublicationStateCloneAndTrim(t *testing.T) {
	publishedAt := time.Date(2026, 6, 12, 22, 30, 0, 0, time.UTC)
	updatedAt := publishedAt.Add(time.Minute)

	state := NormalizeSoulBootstrap(&SoulBootstrapState{
		Username:           " drone-delta ",
		BodyID:             " body-delta ",
		HostRegistrationID: " reg_123 ",
		HostConversationID: " conv_123 ",
		HostSoulAgentID:    " 0xsoul ",
		Phase:              SoulBootstrapPhaseFinalize,
		SigningCheckpoints: []SoulBootstrapSigningCheckpoint{{
			Version:                     " 1 ",
			Name:                        " finalize ",
			Status:                      " ready ",
			ExpectedVersion:             0,
			NextVersion:                 1,
			BoundaryRequirementsJSON:    ` [{"boundary_id":"b1"}] `,
			FinalizeRequestTemplateJSON: ` {"self_attestation":""} `,
			RegistrationPreviewJSON:     ` {"agent_id":"0xsoul"} `,
			HostRequestID:               " host-req-finalize ",
		}},
		Publication: &SoulBootstrapPublicationEvidence{
			AgentID:                    " 0xsoul ",
			PublishedVersion:           1,
			RegistrationURI:            " s3://bucket/registration.json ",
			RegistrationS3Key:          " registry/registration.json ",
			VersionedRegistrationURI:   " s3://bucket/versions/1/registration.json ",
			VersionedRegistrationS3Key: " registry/versions/1/registration.json ",
			AnchorState:                " hosted_offchain ",
			PublishedAt:                &publishedAt,
		},
		Correlation: &SoulBootstrapCorrelationState{
			WalletVerificationIdempotencyKey:   " wallet-1 ",
			PrincipalDeclarationIdempotencyKey: " principal-1 ",
			ConversationIdempotencyKey:         " conversation-1 ",
			FinalizeIdempotencyKey:             " finalize-1 ",
		},
		UpdatedAt: &updatedAt,
	}, "")

	require.Equal(t, "drone-delta", state.Username)
	require.Equal(t, "body-delta", state.BodyID)
	require.Equal(t, SoulBootstrapPhaseFinalize, state.Phase)
	require.Equal(t, "finalize.pending", state.State)
	require.Equal(t, "0xsoul", state.Publication.AgentID)
	require.Equal(t, "hosted_offchain", state.Publication.AnchorState)
	require.Equal(t, publishedAt, *state.Publication.PublishedAt)
	require.Equal(t, "wallet-1", state.Correlation.WalletVerificationIdempotencyKey)
	require.Equal(t, "conversation-1", state.Correlation.ConversationIdempotencyKey)
	require.Equal(t, "finalize-1", state.Correlation.FinalizeIdempotencyKey)
	require.Equal(t, `[{"boundary_id":"b1"}]`, state.SigningCheckpoints[0].BoundaryRequirementsJSON)
	require.Equal(t, `{"self_attestation":""}`, state.SigningCheckpoints[0].FinalizeRequestTemplateJSON)
	require.Equal(t, `{"agent_id":"0xsoul"}`, state.SigningCheckpoints[0].RegistrationPreviewJSON)

	cloned := state.Clone()
	require.NotSame(t, state.Publication, cloned.Publication)
	require.NotSame(t, state.Publication.PublishedAt, cloned.Publication.PublishedAt)
	cloned.Publication.AgentID = "changed"
	require.Equal(t, "0xsoul", state.Publication.AgentID)
	require.Nil(t, (*SoulBootstrapState)(nil).Clone())
	require.Nil(t, cloneSoulBootstrapPublication(nil))
}

func TestSoulBootstrapM23PhaseAndErrorMappings(t *testing.T) {
	require.Equal(t, SoulBootstrapPhaseConversation, normalizeBootstrapPhase("", SoulBootstrapStateConversationCompleted))
	require.Equal(t, SoulBootstrapPhaseNotStarted, normalizeBootstrapPhase("", ""))
	require.Equal(t, "custom", normalizeBootstrapPhase("custom", "conversation.completed"))

	for phase, want := range map[string]string{
		SoulBootstrapPhaseBegin:                "begin.ready",
		SoulBootstrapPhaseWalletVerification:   "wallet_verification.pending",
		SoulBootstrapPhasePrincipalDeclaration: "principal_declaration.pending",
		SoulBootstrapPhaseConversation:         "conversation.pending",
		SoulBootstrapPhaseFinalize:             "finalize.pending",
		SoulBootstrapPhaseError:                SoulBootstrapStateHostBridgeUnavailable,
		SoulBootstrapPhaseComplete:             SoulBootstrapStateCompleteBound,
		"unknown":                              SoulBootstrapStateNotStarted,
	} {
		require.Equal(t, want, defaultBootstrapStateForPhase(phase))
	}

	for code, want := range map[string]string{
		SoulBootstrapErrorHostBridgeUnavailable:         SoulBootstrapStateHostBridgeUnavailable,
		SoulBootstrapErrorHostTrustNotConfigured:        SoulBootstrapStateHostTrustNotConfigured,
		SoulBootstrapErrorHostInstanceKeyMissing:        SoulBootstrapStateHostInstanceKeyMissing,
		SoulBootstrapErrorHostInstanceKeyUnavailable:    SoulBootstrapStateHostInstanceKeyUnavailable,
		SoulBootstrapErrorHostSigningPayloadUnsupported: SoulBootstrapStateHostSigningPayloadUnsupported,
		SoulBootstrapErrorHostRegistrationIDRequired:    SoulBootstrapStateCorrelationMismatch,
		SoulBootstrapErrorHostConversationIDRequired:    SoulBootstrapStateCorrelationMismatch,
		SoulBootstrapErrorHostBootstrapReplayRejected:   SoulBootstrapStateCorrelationMismatch,
		SoulBootstrapErrorSoulBindingConflict:           SoulBootstrapStateBindingConflict,
		SoulBootstrapErrorSoulNotAvailable:              SoulBootstrapStateSoulNotAvailable,
		SoulBootstrapErrorHostUnavailable:               SoulBootstrapStateHostUnavailable,
		"unexpected":                                    SoulBootstrapStateHostUnavailable,
	} {
		require.Equal(t, want, errorStateForBootstrapCode(code))
	}

	now := time.Date(2026, 6, 12, 23, 0, 0, 0, time.UTC)
	defaulted := NewSoulBootstrapErrorState("drone-epsilon", nil, "", "", "", 0, " ", now)
	require.Equal(t, SoulBootstrapErrorHostUnavailable, defaulted.Error.Code)
	require.Equal(t, "Soul bootstrap could not complete.", defaulted.Error.Message)
	require.Equal(t, "lesser", defaulted.Error.Source)
	require.Equal(t, SoulBootstrapStateHostUnavailable, defaulted.State)
}
