package main

import (
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestActivityProcessorLocalityAcceptsCaseVariantHTTPScheme(t *testing.T) {
	domain := config.Get().Domain
	require.NotEmpty(t, domain)

	h := &ActivityHandler{}
	require.True(t, h.isLocalActor("HTTP://"+strings.ToUpper(domain)+"/users/alice"))
}
