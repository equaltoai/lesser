// Package factories provides test data factories for consistent test data generation
package factories

import (
	"fmt"
	"time"
)

// Builder is a generic builder interface for test data
type Builder[T any] interface {
	Build() T
	Reset() Builder[T]
}

// BaseBuilder provides common functionality for all builders
type BaseBuilder struct {
	domain   string
	sequence int
	baseTime time.Time
}

// NewBaseBuilder creates a new base builder
func NewBaseBuilder(domain string) *BaseBuilder {
	return &BaseBuilder{
		domain:   domain,
		sequence: 1,
		baseTime: time.Now().Truncate(time.Hour),
	}
}

// NextSequence increments and returns the sequence number
func (b *BaseBuilder) NextSequence() int {
	seq := b.sequence
	b.sequence++
	return seq
}

// GenerateID generates a unique ID with the given prefix
func (b *BaseBuilder) GenerateID(prefix string) string {
	return fmt.Sprintf("https://%s/%s/%d", b.domain, prefix, b.NextSequence())
}

// GenerateTimestamp generates a timestamp based on sequence
func (b *BaseBuilder) GenerateTimestamp() time.Time {
	return b.baseTime.Add(time.Duration(b.sequence) * time.Minute)
}

// WithDefaults applies default values to a map of options
func WithDefaults(values map[string]interface{}, defaults map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	// Copy defaults first
	for k, v := range defaults {
		result[k] = v
	}
	
	// Override with provided values
	for k, v := range values {
		if v != nil && v != "" {
			result[k] = v
		}
	}
	
	return result
}

// FluentSetter provides a fluent interface for setting values
type FluentSetter[T any] struct {
	target *T
}

// NewFluentSetter creates a new fluent setter
func NewFluentSetter[T any](target *T) *FluentSetter[T] {
	return &FluentSetter[T]{target: target}
}

// Set sets a value and returns the setter for chaining
func (f *FluentSetter[T]) Set(setter func(*T)) *FluentSetter[T] {
	setter(f.target)
	return f
}

// Build returns the target object
func (f *FluentSetter[T]) Build() *T {
	return f.target
}