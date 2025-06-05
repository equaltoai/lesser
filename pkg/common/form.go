package common

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"
)

// ParseMultipartForm parses multipart form data from a request body
func ParseMultipartForm(body string, contentType string) (map[string]string, error) {
	// Parse the content type to extract the boundary
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("invalid content type: %w", err)
	}

	boundary, ok := params["boundary"]
	if !ok {
		return nil, fmt.Errorf("no boundary found in content type")
	}

	// Create a multipart reader
	reader := multipart.NewReader(strings.NewReader(body), boundary)

	// Parse form fields
	values := make(map[string]string)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading multipart: %w", err)
		}

		// Read the part content
		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, part); err != nil {
			return nil, fmt.Errorf("error reading part: %w", err)
		}

		// Get the form field name
		fieldName := part.FormName()
		if fieldName != "" {
			values[fieldName] = buf.String()
		}

		part.Close()
	}

	return values, nil
}

// ParseFormURLEncoded parses URL-encoded form data
func ParseFormURLEncoded(body string) (map[string]string, error) {
	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, fmt.Errorf("error parsing form data: %w", err)
	}

	// Convert to simple map (taking first value for each key)
	result := make(map[string]string)
	for key, vals := range values {
		if len(vals) > 0 {
			result[key] = vals[0]
		}
	}

	return result, nil
}
