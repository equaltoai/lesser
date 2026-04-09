package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/require"
)

type captureHTTPDoer struct {
	req  *http.Request
	body []byte
}

func (d *captureHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))

	d.req = cloned
	d.body = append([]byte(nil), body...)

	return &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte("accepted"))),
	}, nil
}

func TestActivityPubDeliverySigningParity(t *testing.T) {
	ctx := context.Background()
	privateKeyPEM, privateKey := rsaPrivateKeyPEM(t)

	signingActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
		PublicKey: &activitypub.PublicKey{
			ID:    "https://example.com/users/alice#main-key",
			Owner: "https://example.com/users/alice",
		},
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			ID:      "https://example.com/activities/follow-1",
			Type:    activitypub.FollowType,
			To:      []string{"https://remote.example/users/bob"},
		},
		Actor:  signingActor.ID,
		Object: "https://remote.example/users/bob",
	}

	store := fakeFederationStore{
		privateKeyFn: func(context.Context, string) (string, error) {
			return privateKeyPEM, nil
		},
	}

	classicBody, err := json.Marshal(activity)
	require.NoError(t, err)
	classicReq, err := federation.BuildSignedActivityPubRequest(
		ctx,
		http.MethodPost,
		"https://remote.example/inbox?shared=true",
		"Lesser/1.0",
		classicBody,
		privateKey,
		signingActor.PublicKey.ID,
	)
	require.NoError(t, err)

	managerCapture := &captureHTTPDoer{}
	m, _, _ := newTestManager(t)
	m.federationStore = store
	m.httpClient = managerCapture

	result := &fedTypes.DeliveryResult{}
	err = m.performHTTPDelivery(ctx, activity, "https://remote.example/inbox?shared=true", signingActor, result)
	require.NoError(t, err)
	require.NotNil(t, managerCapture.req)

	classicSig, err := federation.ParseSignatureHeader(classicReq.Header.Get(federation.SignatureHeader))
	require.NoError(t, err)
	managerSig, err := federation.ParseSignatureHeader(managerCapture.req.Header.Get(federation.SignatureHeader))
	require.NoError(t, err)

	classicCanonical, err := federation.BuildHTTPSignatureString(classicReq, classicSig.Headers)
	require.NoError(t, err)
	managerCanonical, err := federation.BuildHTTPSignatureString(managerCapture.req, managerSig.Headers)
	require.NoError(t, err)

	require.Equal(t, classicSig.Algorithm, managerSig.Algorithm)
	require.Equal(t, classicSig.Headers, managerSig.Headers)
	require.Equal(t, classicCanonical, managerCanonical)
	require.Equal(t, classicBody, managerCapture.body)
	require.NoError(t, federation.VerifyHTTPSignature(classicReq, &privateKey.PublicKey))
	require.NoError(t, federation.VerifyHTTPSignature(managerCapture.req, &privateKey.PublicKey))
}
