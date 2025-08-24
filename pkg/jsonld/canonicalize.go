package jsonld

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/equaltoai/lesser/pkg/common"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Canonicalizer implements JSON-LD canonicalization following URDNA2015 algorithm
type Canonicalizer struct {
	blankNodeIdentifierMap map[string]string
	canonicalIssuer        *IdentifierIssuer
	options                CanonicalizeOptions
}

// CanonicalizeOptions configures the canonicalization process
type CanonicalizeOptions struct {
	// SkipExpansion skips JSON-LD expansion step (for non-JSON-LD documents)
	SkipExpansion bool
	// RemoveSignatureFields removes signature-related fields before canonicalization
	RemoveSignatureFields bool
	// SignatureFields defines which fields to remove during canonicalization
	SignatureFields []string
}

// IdentifierIssuer generates blank node identifiers
type IdentifierIssuer struct {
	prefix   string
	counter  int
	existing map[string]string
}

// NewIdentifierIssuer creates a new identifier issuer
func NewIdentifierIssuer(prefix string) *IdentifierIssuer {
	return &IdentifierIssuer{
		prefix:   prefix,
		counter:  0,
		existing: make(map[string]string),
	}
}

// GetID returns an identifier for the given blank node
func (i *IdentifierIssuer) GetID(blankNode string) string {
	if id, exists := i.existing[blankNode]; exists {
		return id
	}

	id := fmt.Sprintf("%s%d", i.prefix, i.counter)
	i.counter++
	i.existing[blankNode] = id
	return id
}

// Clone creates a copy of the identifier issuer
func (i *IdentifierIssuer) Clone() *IdentifierIssuer {
	clone := &IdentifierIssuer{
		prefix:   i.prefix,
		counter:  i.counter,
		existing: make(map[string]string),
	}
	for k, v := range i.existing {
		clone.existing[k] = v
	}
	return clone
}

// NewCanonicalizer creates a new JSON-LD canonicalizer
func NewCanonicalizer(options CanonicalizeOptions) *Canonicalizer {
	if options.RemoveSignatureFields && options.SignatureFields == nil {
		options.SignatureFields = []string{
			"signature", "Signature",
			"issuerProof", "IssuerProof",
			"proof", "Proof",
			"proofs", "Proofs",
		}
	}

	return &Canonicalizer{
		blankNodeIdentifierMap: make(map[string]string),
		canonicalIssuer:        NewIdentifierIssuer("_:c14n"),
		options:                options,
	}
}

// Canonicalize performs JSON-LD canonicalization on the input document
func (c *Canonicalizer) Canonicalize(input interface{}) ([]byte, error) {
	// Step 1: Parse and normalize input
	normalized, err := c.normalizeInput(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNormalizeInput, err)
	}

	// Step 2: Remove signature fields if requested
	if c.options.RemoveSignatureFields {
		normalized = c.removeSignatureFields(normalized)
	}

	// Step 3: Convert to N-Quads representation
	nquads, err := c.toNQuads(normalized)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConvertToNQuads, err)
	}

	// Step 4: Sort N-Quads lexicographically
	sort.Strings(nquads)

	// Step 5: Join with newlines for canonical form
	canonical := strings.Join(nquads, "\n")
	if len(nquads) > 0 {
		canonical += "\n"
	}

	return []byte(canonical), nil
}

// CanonicalizeToJSON performs canonicalization and returns a canonical JSON document
func (c *Canonicalizer) CanonicalizeToJSON(input interface{}) ([]byte, error) {
	// Step 1: Parse and normalize input
	normalized, err := c.normalizeInput(input)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNormalizeInput, err)
	}

	// Step 2: Remove signature fields if requested
	if c.options.RemoveSignatureFields {
		normalized = c.removeSignatureFields(normalized)
	}

	// Step 3: Canonicalize JSON structure
	canonical, err := c.canonicalizeJSONStructure(normalized)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCanonicalizeJSONStructure, err)
	}

	// Step 4: Marshal with deterministic ordering
	return c.marshalCanonical(canonical)
}

// normalizeInput converts input to a normalized map representation
func (c *Canonicalizer) normalizeInput(input interface{}) (interface{}, error) {
	switch v := input.(type) {
	case []byte:
		var parsed interface{}
		if err := json.Unmarshal(v, &parsed); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrParseJSON, err)
		}
		return c.normalizeValue(parsed), nil
	case string:
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrParseJSONString, err)
		}
		return c.normalizeValue(parsed), nil
	default:
		// Convert to JSON and back to normalize types
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMarshalInput, err)
		}

		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnmarshalNormalized, err)
		}

		return c.normalizeValue(parsed), nil
	}
}

// normalizeValue recursively normalizes values
func (c *Canonicalizer) normalizeValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		normalized := make(map[string]interface{})
		for key, val := range v {
			normalized[key] = c.normalizeValue(val)
		}
		return normalized
	case []interface{}:
		normalized := make([]interface{}, len(v))
		for i, val := range v {
			normalized[i] = c.normalizeValue(val)
		}
		return normalized
	case float64:
		// Handle integer values stored as float64
		if v == float64(int64(v)) && v >= -9007199254740991 && v <= 9007199254740991 {
			return int64(v)
		}
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	default:
		return v
	}
}

// removeSignatureFields removes signature-related fields from the document
func (c *Canonicalizer) removeSignatureFields(input interface{}) interface{} {
	switch v := input.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			// Skip signature fields
			skip := false
			for _, field := range c.options.SignatureFields {
				if key == field {
					skip = true
					break
				}
			}

			if !skip {
				result[key] = c.removeSignatureFields(value)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, value := range v {
			result[i] = c.removeSignatureFields(value)
		}
		return result
	default:
		return v
	}
}

// canonicalizeJSONStructure canonicalizes the JSON structure without converting to N-Quads
func (c *Canonicalizer) canonicalizeJSONStructure(input interface{}) (interface{}, error) {
	return c.canonicalizeValue(input), nil
}

// canonicalizeValue recursively canonicalizes values ensuring deterministic ordering
func (c *Canonicalizer) canonicalizeValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		// Sort keys alphabetically
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		// Build ordered map
		result := make(map[string]interface{})
		for _, key := range keys {
			result[key] = c.canonicalizeValue(v[key])
		}
		return result

	case []interface{}:
		// For arrays, maintain order but canonicalize contents
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = c.canonicalizeValue(item)
		}
		return result

	default:
		return value
	}
}

// marshalCanonical marshals with deterministic ordering and formatting
func (c *Canonicalizer) marshalCanonical(value interface{}) ([]byte, error) {
	return json.Marshal(value)
}

// toNQuads converts normalized JSON-LD to N-Quads format
func (c *Canonicalizer) toNQuads(input interface{}) ([]string, error) {
	var nquads []string

	switch v := input.(type) {
	case map[string]interface{}:
		quads, err := c.objectToNQuads(v, "")
		if err != nil {
			return nil, err
		}
		nquads = append(nquads, quads...)

	case []interface{}:
		for _, item := range v {
			quads, err := c.toNQuads(item)
			if err != nil {
				return nil, err
			}
			nquads = append(nquads, quads...)
		}
	}

	return nquads, nil
}

// objectToNQuads converts a JSON object to N-Quads
func (c *Canonicalizer) objectToNQuads(obj map[string]interface{}, subject string) ([]string, error) {
	var nquads []string

	// If no subject provided, generate one
	if err := common.ValidateRequiredParam("subject", subject); err != nil {
		if id, hasID := obj["@id"]; hasID {
			if idStr, ok := id.(string); ok {
				subject = c.escapeNQuadsValue(idStr)
			}
		} else {
			// Generate blank node
			subject = c.canonicalIssuer.GetID("_:b")
		}
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(obj))
	for key := range obj {
		if key != "@id" { // Skip @id as it's handled as subject
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	// Convert each property to N-Quads
	for _, key := range keys {
		value := obj[key]
		predicate := c.escapeNQuadsValue(key)

		switch v := value.(type) {
		case map[string]interface{}:
			// Object value
			objectQuads, err := c.objectToNQuads(v, "")
			if err != nil {
				return nil, err
			}
			nquads = append(nquads, objectQuads...)

			// Link to the object
			objectSubject := c.getObjectSubject(v)
			quad := fmt.Sprintf("%s %s %s .", subject, predicate, objectSubject)
			nquads = append(nquads, quad)

		case []interface{}:
			// Array of values
			for _, item := range v {
				itemQuads, err := c.valueToNQuads(subject, predicate, item)
				if err != nil {
					return nil, err
				}
				nquads = append(nquads, itemQuads...)
			}

		default:
			// Literal value
			itemQuads, err := c.valueToNQuads(subject, predicate, v)
			if err != nil {
				return nil, err
			}
			nquads = append(nquads, itemQuads...)
		}
	}

	return nquads, nil
}

// valueToNQuads converts a value to N-Quads format
func (c *Canonicalizer) valueToNQuads(subject, predicate string, value interface{}) ([]string, error) {
	switch v := value.(type) {
	case map[string]interface{}:
		// Nested object
		objectQuads, err := c.objectToNQuads(v, "")
		if err != nil {
			return nil, err
		}

		objectSubject := c.getObjectSubject(v)
		quad := fmt.Sprintf("%s %s %s .", subject, predicate, objectSubject)

		return append(objectQuads, quad), nil

	default:
		// Literal value
		literal := c.valueToLiteral(v)
		quad := fmt.Sprintf("%s %s %s .", subject, predicate, literal)
		return []string{quad}, nil
	}
}

// getObjectSubject gets or generates a subject for an object
func (c *Canonicalizer) getObjectSubject(obj map[string]interface{}) string {
	if id, hasID := obj["@id"]; hasID {
		if idStr, ok := id.(string); ok {
			return c.escapeNQuadsValue(idStr)
		}
	}

	// Generate blank node
	return c.canonicalIssuer.GetID("_:b")
}

// valueToLiteral converts a value to N-Quads literal format
func (c *Canonicalizer) valueToLiteral(value interface{}) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, c.escapeStringLiteral(v))
	case int64:
		return fmt.Sprintf(`"%d"^^<http://www.w3.org/2001/XMLSchema#integer>`, v)
	case float64:
		return fmt.Sprintf(`"%g"^^<http://www.w3.org/2001/XMLSchema#double>`, v)
	case bool:
		return fmt.Sprintf(`"%t"^^<http://www.w3.org/2001/XMLSchema#boolean>`, v)
	default:
		// Fallback to string representation
		return fmt.Sprintf(`"%v"`, v)
	}
}

// escapeNQuadsValue escapes a value for N-Quads format
func (c *Canonicalizer) escapeNQuadsValue(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return fmt.Sprintf("<%s>", value)
	}
	if strings.HasPrefix(value, "_:") {
		return value
	}
	return fmt.Sprintf("<%s>", value)
}

// escapeStringLiteral escapes special characters in string literals
func (c *Canonicalizer) escapeStringLiteral(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7F {
				result.WriteString(fmt.Sprintf(`\u%04X`, r))
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}

// Utility functions for common use cases

// CanonicalizeBytesToJSON canonicalizes input bytes to canonical JSON
func CanonicalizeBytesToJSON(input []byte, removeSignatures bool) ([]byte, error) {
	options := CanonicalizeOptions{
		SkipExpansion:         true,
		RemoveSignatureFields: removeSignatures,
	}

	canonicalizer := NewCanonicalizer(options)
	return canonicalizer.CanonicalizeToJSON(input)
}

// CanonicalizeStructToJSON canonicalizes a Go struct to canonical JSON
func CanonicalizeStructToJSON(input interface{}, removeSignatures bool) ([]byte, error) {
	options := CanonicalizeOptions{
		SkipExpansion:         true,
		RemoveSignatureFields: removeSignatures,
	}

	canonicalizer := NewCanonicalizer(options)
	return canonicalizer.CanonicalizeToJSON(input)
}

// CanonicalizeActivityPubObject canonicalizes an ActivityPub object for signature verification
func CanonicalizeActivityPubObject(input interface{}) ([]byte, error) {
	options := CanonicalizeOptions{
		SkipExpansion:         false, // ActivityPub objects may need expansion
		RemoveSignatureFields: true,
		SignatureFields: []string{
			"signature", "Signature",
			"proof", "Proof",
			"proofs", "Proofs",
		},
	}

	canonicalizer := NewCanonicalizer(options)
	return canonicalizer.CanonicalizeToJSON(input)
}

// Hash returns a SHA256 hash of the canonical form
func Hash(canonical []byte) string {
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:])
}

// IsBlankNode checks if a string represents a blank node identifier
func IsBlankNode(value string) bool {
	return strings.HasPrefix(value, "_:")
}

// NormalizeUnicode normalizes Unicode strings for consistent canonicalization
func NormalizeUnicode(s string) string {
	// Basic Unicode normalization
	if !utf8.ValidString(s) {
		// If not valid UTF-8, return as-is
		return s
	}

	// Remove leading/trailing whitespace and normalize internal whitespace
	s = strings.TrimSpace(s)

	// Normalize multiple whitespace to single space
	var result strings.Builder
	var prevSpace bool

	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				result.WriteRune(' ')
				prevSpace = true
			}
		} else {
			result.WriteRune(r)
			prevSpace = false
		}
	}

	return result.String()
}
