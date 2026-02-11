package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	cliAuthSessionVersion = 1
)

var (
	authSessionMagic = []byte("lesser-auth-session:v1\n")
)

type cliAuthSession struct {
	Version int `json:"version"`

	BaseURL  string `json:"base_url"`
	ClientID string `json:"client_id"`

	RefreshToken string   `json:"refresh_token"`
	Username     string   `json:"username"`
	Scopes       []string `json:"scopes,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func readAuthSecret(secretFile string) (string, error) {
	if value := strings.TrimSpace(os.Getenv("LESSER_AUTH_SECRET")); value != "" {
		return value, nil
	}

	secretFile = strings.TrimSpace(secretFile)
	if secretFile == "" {
		return "", nil
	}

	data, err := os.ReadFile(secretFile) // #nosec G304 -- CLI reads an operator-provided local secret path
	if err != nil {
		return "", fmt.Errorf("read secret file %s: %w", secretFile, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func deriveAuthKey(baseURL, secret string) []byte {
	baseURL = strings.TrimSpace(baseURL)
	secret = strings.TrimSpace(secret)

	material := ""
	if secret != "" {
		material = secret
	} else {
		if keyringEnabled() {
			if krSecret, err := getOrCreateKeyringSecret(baseURL); err == nil && strings.TrimSpace(krSecret) != "" {
				material = krSecret
			} else {
				material = machineDerivedSecret()
			}
		} else {
			material = machineDerivedSecret()
		}
	}

	sum := sha256.Sum256([]byte(material + "|" + baseURL))
	return sum[:]
}

func machineDerivedSecret() string {
	host, _ := os.Hostname()
	user := strings.TrimSpace(os.Getenv("USER"))
	home, _ := os.UserHomeDir()
	machineID := readMachineID()

	return strings.Join([]string{
		strings.TrimSpace(host),
		user,
		machineID,
		strings.TrimSpace(home),
	}, "|")
}

func readMachineID() string {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		data, err := os.ReadFile(path) // #nosec G304 -- reads known system paths
		if err != nil {
			continue
		}
		if value := strings.TrimSpace(string(data)); value != "" {
			return value
		}
	}
	return ""
}

func authBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lesser", "auth"), nil
}

func authBaseURLHash(baseURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(baseURL)))
	return hex.EncodeToString(sum[:])
}

func authSessionFile(baseURL string) (string, error) {
	baseDir, err := authBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, authBaseURLHash(baseURL), "session.enc"), nil
}

func readAuthSession(baseURL string, key []byte) (*cliAuthSession, error) {
	path, err := authSessionFile(baseURL)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path) // #nosec G304 -- local cli session store
	if err != nil {
		return nil, err
	}

	plaintext, err := decryptAuthBlob(raw, key, baseURL)
	if err != nil {
		return nil, err
	}

	var session cliAuthSession
	if err := json.Unmarshal(plaintext, &session); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &session, nil
}

func writeAuthSession(baseURL string, key []byte, session *cliAuthSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	path, err := authSessionFile(baseURL)
	if err != nil {
		return err
	}

	session.BaseURL = strings.TrimSpace(baseURL)

	plaintext, err := json.Marshal(session)
	if err != nil {
		return err
	}

	blob, err := encryptAuthBlob(plaintext, key, baseURL)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func deleteAuthSession(baseURL string) (bool, error) {
	path, err := authSessionFile(baseURL)
	if err != nil {
		return false, err
	}

	err = os.Remove(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func encryptAuthBlob(plaintext, key []byte, baseURL string) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("auth key must be 32 bytes (got %d)", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	aad := []byte("base_url:" + strings.TrimSpace(baseURL))
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	out := make([]byte, 0, len(authSessionMagic)+len(nonce)+len(ciphertext))
	out = append(out, authSessionMagic...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

func decryptAuthBlob(blob, key []byte, baseURL string) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("auth key must be 32 bytes (got %d)", len(key))
	}

	if len(blob) < len(authSessionMagic) || !strings.HasPrefix(string(blob[:len(authSessionMagic)]), string(authSessionMagic)) {
		return nil, errors.New("unsupported session file format")
	}

	payload := blob[len(authSessionMagic):]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("corrupt session file")
	}

	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]

	aad := []byte("base_url:" + strings.TrimSpace(baseURL))
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func requireNonEmpty(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("base-url is required (or set LESSER_BASE_URL)")
	}
	raw = strings.TrimRight(raw, "/")

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "", fmt.Errorf("base-url must start with http:// or https:// (got %q)", raw)
	}
	return raw, nil
}
