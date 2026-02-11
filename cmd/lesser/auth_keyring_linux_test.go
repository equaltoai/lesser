//go:build linux
// +build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyringLinux_SecretToolImplementation(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))

	dbDir := filepath.Join(tmp, "db")
	require.NoError(t, os.MkdirAll(dbDir, 0o700))

	scriptPath := filepath.Join(binDir, "secret-tool")
	script := `#!/bin/sh
set -eu

cmd="$1"
shift

dir="${SECRET_TOOL_DIR:-}"
if [ -z "$dir" ]; then
  echo "SECRET_TOOL_DIR required" >&2
  exit 2
fi

account=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    account)
      account="$2"
      shift 2
      ;;
    --label=*)
      shift 1
      ;;
    *)
      shift 1
      ;;
  esac
done

if [ -z "$account" ]; then
  echo "account missing" >&2
  exit 2
fi

path="$dir/$account"

case "$cmd" in
  lookup)
    if [ -f "$path" ]; then
      cat "$path"
      exit 0
    fi
    exit 1
    ;;
  store)
    cat - > "$path"
    exit 0
    ;;
  clear)
    rm -f "$path"
    exit 0
    ;;
  *)
    echo "unknown command" >&2
    exit 2
    ;;
esac
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o700))

	t.Setenv("SECRET_TOOL_DIR", dbDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	require.True(t, keyringIsAvailable())

	account := "test-account"
	_, err := keyringLoadSecret(account)
	require.ErrorIs(t, err, errKeyringNotFound)

	require.NoError(t, keyringSaveSecret(account, "secret-123"))
	secret, err := keyringLoadSecret(account)
	require.NoError(t, err)
	require.Equal(t, "secret-123", secret)
}
