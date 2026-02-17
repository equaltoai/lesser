package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActorActivityJSON_Round24_MarshalError(t *testing.T) {
	resp, err := actorActivityJSON(http.StatusOK, make(chan int))
	require.Nil(t, resp)
	require.Error(t, err)
}

func TestConvertAppTheoryRequest_Round24_NilContext(t *testing.T) {
	h := &Handler{}
	req, err := h.convertAppTheoryRequest(nil)
	require.Nil(t, req)
	require.Error(t, err)
}
