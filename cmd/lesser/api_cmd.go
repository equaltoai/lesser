package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func runAPI(argv []string) error {
	if len(argv) == 0 {
		printUsage()
		return nil
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "request":
		return runAPIRequest(argv[1:])
	default:
		if argv[0] != "" && argv[0][0] == '-' {
			printUsage()
			return nil
		}
		return fmt.Errorf("unknown api command %q", argv[0])
	}
}

type apiRequestFlags struct {
	BaseURL    string
	SecretFile string

	Method string
	Path   string

	Data     string
	DataFile string

	Headers multiValueFlag

	ContentType string
	Accept      string

	MaxConcurrency int
	RPS            float64
	Burst          int
	Retries        int
	TimeoutSeconds int
}

func runAPIRequest(argv []string) error {
	fs := flag.NewFlagSet("lesser api request", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args apiRequestFlags
	fs.StringVar(&args.BaseURL, "base-url", os.Getenv("LESSER_BASE_URL"), "instance base url (env: LESSER_BASE_URL)")
	fs.StringVar(&args.SecretFile, "secret-file", os.Getenv("LESSER_AUTH_SECRET_FILE"), "path to auth secret file (env: LESSER_AUTH_SECRET_FILE)")

	fs.StringVar(&args.Method, "method", http.MethodGet, "http method (GET, POST, ...)")
	fs.StringVar(&args.Path, "path", "", "request path (example: /api/v1/accounts/verify_credentials)")

	fs.StringVar(&args.Data, "data", "", "request body string")
	fs.StringVar(&args.DataFile, "data-file", "", "request body file path")
	fs.Var(&args.Headers, "header", "header (repeatable): 'Name: value'")

	fs.StringVar(&args.ContentType, "content-type", "", "content type header (default: application/json when body is set)")
	fs.StringVar(&args.Accept, "accept", "application/json", "accept header")

	fs.IntVar(&args.MaxConcurrency, "max-concurrency", envOrDefaultInt("LESSER_CLI_MAX_CONCURRENCY", 2), "max concurrent in-flight http requests (env: LESSER_CLI_MAX_CONCURRENCY)")
	fs.Float64Var(&args.RPS, "rps", envOrDefaultFloat("LESSER_CLI_RPS", 2.0), "max requests per second (env: LESSER_CLI_RPS)")
	fs.IntVar(&args.Burst, "burst", envOrDefaultInt("LESSER_CLI_BURST", 4), "token bucket burst size (env: LESSER_CLI_BURST)")
	fs.IntVar(&args.Retries, "retries", envOrDefaultInt("LESSER_CLI_RETRIES", 3), "max retries for 429/5xx/transient errors (env: LESSER_CLI_RETRIES)")
	fs.IntVar(&args.TimeoutSeconds, "timeout", envOrDefaultInt("LESSER_CLI_TIMEOUT", 30), "overall request timeout seconds (env: LESSER_CLI_TIMEOUT)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	baseURL, err := normalizeBaseURL(args.BaseURL)
	if err != nil {
		return err
	}

	method := strings.ToUpper(strings.TrimSpace(args.Method))
	if method == "" {
		return fmt.Errorf("method is required")
	}

	path := strings.TrimSpace(args.Path)
	if err := requireNonEmpty("path", path); err != nil {
		return err
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with '/' (got %q)", path)
	}

	body, err := readBodyArg(args.Data, args.DataFile)
	if err != nil {
		return err
	}

	headers := http.Header{}
	for _, raw := range args.Headers.Values() {
		name, value, err := splitHeader(raw)
		if err != nil {
			return err
		}
		headers.Add(name, value)
	}
	if accept := strings.TrimSpace(args.Accept); accept != "" && headers.Get("Accept") == "" {
		headers.Set("Accept", accept)
	}
	if len(body) > 0 && headers.Get("Content-Type") == "" {
		contentType := strings.TrimSpace(args.ContentType)
		if contentType == "" {
			contentType = "application/json"
		}
		headers.Set("Content-Type", contentType)
	}

	secret, err := readAuthSecret(args.SecretFile)
	if err != nil {
		return err
	}
	key := deriveAuthKey(baseURL, secret)

	client, err := newCLIAPIClient(baseURL, key, cliAPIClientOptions{
		MaxConcurrency: args.MaxConcurrency,
		RPS:            args.RPS,
		Burst:          args.Burst,
		Retries:        args.Retries,
		Timeout:        time.Duration(args.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(args.TimeoutSeconds)*time.Second)
	defer cancel()

	status, respHeaders, respBody, err := client.Request(ctx, method, path, headers, body)
	if len(respBody) > 0 {
		_, _ = os.Stdout.Write(respBody)
	}

	if err != nil {
		return err
	}

	if status >= 400 {
		retryAfter := strings.TrimSpace(respHeaders.Get("Retry-After"))
		if retryAfter != "" {
			return fmt.Errorf("api request failed (%d); retry-after=%s", status, retryAfter)
		}
		return fmt.Errorf("api request failed (%d)", status)
	}

	return nil
}

type multiValueFlag []string

func (m *multiValueFlag) String() string {
	if m == nil {
		return ""
	}
	return strings.Join(*m, ",")
}

func (m *multiValueFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func (m *multiValueFlag) Values() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), *m...)
}

func readBodyArg(data, dataFile string) ([]byte, error) {
	if strings.TrimSpace(data) != "" && strings.TrimSpace(dataFile) != "" {
		return nil, fmt.Errorf("only one of --data or --data-file may be set")
	}
	if strings.TrimSpace(dataFile) != "" {
		body, err := os.ReadFile(dataFile) // #nosec G304 -- CLI reads an operator-provided local path
		if err != nil {
			return nil, fmt.Errorf("read data-file %s: %w", dataFile, err)
		}
		return body, nil
	}
	if strings.TrimSpace(data) != "" {
		return []byte(data), nil
	}
	return nil, nil
}

func splitHeader(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("header is empty")
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("header must be in 'Name: value' format (got %q)", raw)
	}
	name := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if name == "" {
		return "", "", fmt.Errorf("header name is empty (got %q)", raw)
	}
	return name, value, nil
}

func envOrDefaultInt(key string, value int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return value
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return value
	}
	return parsed
}

func envOrDefaultFloat(key string, value float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return value
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return value
	}
	return parsed
}
