package stream

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
)

// UnmarshalItem unmarshals a single DynamoDB stream record into a struct
func UnmarshalItem(record events.DynamoDBEventRecord, out any) error {
	// Check if we're dealing with a NewImage (INSERT/MODIFY) or OldImage (REMOVE)
	var image map[string]events.DynamoDBAttributeValue
	switch record.EventName {
	case eventNameInsert, eventNameModify:
		image = record.Change.NewImage
	case eventNameRemove:
		image = record.Change.OldImage
	default:
		return fmt.Errorf("unknown event type: %s", record.EventName)
	}

	// Convert DynamoDB stream attribute values to DynamORM compatible format
	item, err := convertStreamImageToItem(image)
	if err != nil {
		return fmt.Errorf("failed to convert stream image: %w", err)
	}

	// Production-ready field mapping using DynamORM-compatible approach
	return unmarshalWithDynamORMCompat(item, out)
}

// UnmarshalItems unmarshals multiple DynamoDB stream records into a slice of structs
func UnmarshalItems(records []events.DynamoDBEventRecord, outType any) (any, error) {
	// Create a slice of the correct type to hold results
	sliceType := reflect.SliceOf(reflect.TypeOf(outType))
	results := reflect.MakeSlice(sliceType, 0, len(records))

	// Process each record
	for _, record := range records {
		// Create a new instance of the output type
		item := reflect.New(reflect.TypeOf(outType)).Interface()

		// Unmarshal the record
		if err := UnmarshalItem(record, item); err != nil {
			common.Logger().Warn("Failed to unmarshal stream record",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err),
			)
			continue
		}

		// Add to results
		results = reflect.Append(results, reflect.ValueOf(item).Elem())
	}

	return results.Interface(), nil
}

// ProcessStreamRecords processes DynamoDB stream records with the provided handler function
func ProcessStreamRecords(ctx context.Context, records []events.DynamoDBEventRecord, handler func(ctx context.Context, record events.DynamoDBEventRecord) error) error {
	for _, record := range records {
		// Skip records we don't care about
		if record.EventName != eventNameInsert && record.EventName != eventNameModify && record.EventName != eventNameRemove {
			continue
		}

		// Process the record with the handler (handler is responsible for unmarshaling)
		if err := handler(ctx, record); err != nil {
			common.Logger().Error("Failed to process stream record",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err),
			)
			// Continue processing other records even if one fails
			continue
		}
	}

	return nil
}

// Helper function to convert DynamoDB stream attribute values to DynamORM compatible format
func convertStreamImageToItem(image map[string]events.DynamoDBAttributeValue) (map[string]any, error) {
	item := make(map[string]any)

	for key, value := range image {
		converted, err := convertAttributeValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to convert attribute %s: %w", key, err)
		}
		item[key] = converted
	}

	return item, nil
}


// Helper function to convert a single DynamoDB attribute value to a Go type
func convertAttributeValue(attr events.DynamoDBAttributeValue) (any, error) {
	switch attr.DataType() {
	case events.DataTypeString:
		return attr.String(), nil
	case events.DataTypeNumber:
		return attr.Number(), nil
	case events.DataTypeBoolean:
		return attr.Boolean(), nil
	case events.DataTypeMap:
		m := make(map[string]any)
		for k, v := range attr.Map() {
			converted, err := convertAttributeValue(v)
			if err != nil {
				return nil, err
			}
			m[k] = converted
		}
		return m, nil
	case events.DataTypeList:
		list := make([]any, len(attr.List()))
		for i, v := range attr.List() {
			converted, err := convertAttributeValue(v)
			if err != nil {
				return nil, err
			}
			list[i] = converted
		}
		return list, nil
	case events.DataTypeNull:
		return nil, nil
	case events.DataTypeBinary:
		return attr.Binary(), nil
	default:
		return nil, fmt.Errorf("unsupported attribute type: %v", attr.DataType())
	}
}

// Production enhancements for DynamoDB stream processing

// ProcessingConfig holds the configuration for stream processing
type ProcessingConfig struct {
	EnableMetrics          bool
	EnableErrorRecovery    bool
	MaxRetryAttempts      int
	RetryBackoffInitial   time.Duration
	RetryBackoffMax       time.Duration
	EnableDLQ             bool
	ParallelProcessing    bool
	MaxConcurrentRecords  int
}

// DefaultProcessingConfig returns production-ready default configuration
func DefaultProcessingConfig() *ProcessingConfig {
	return &ProcessingConfig{
		EnableMetrics:         true,
		EnableErrorRecovery:   true,
		MaxRetryAttempts:     3,
		RetryBackoffInitial:  time.Millisecond * 100,
		RetryBackoffMax:      time.Second * 5,
		EnableDLQ:           true,
		ParallelProcessing:  true,
		MaxConcurrentRecords: 10,
	}
}

// Processor handles stream processing with enhanced error handling and metrics
type Processor struct {
	config *ProcessingConfig
	logger *zap.Logger
}

// NewProcessor creates a new enhanced stream processor
func NewProcessor(config *ProcessingConfig, logger *zap.Logger) *Processor {
	if config == nil {
		config = DefaultProcessingConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Processor{
		config: config,
		logger: logger,
	}
}

// ProcessStreamRecordsWithRetry processes DynamoDB stream records with production features
func (sp *Processor) ProcessStreamRecordsWithRetry(
	ctx context.Context, 
	records []events.DynamoDBEventRecord, 
	handler func(ctx context.Context, record events.DynamoDBEventRecord) error,
) error {
	// Metrics tracking
	startTime := time.Now()
	totalRecords := len(records)
	processedRecords := 0
	failedRecords := 0

	defer func() {
		if sp.config.EnableMetrics {
			sp.logger.Info("stream_processing_completed",
				zap.Int("total_records", totalRecords),
				zap.Int("processed_records", processedRecords),
				zap.Int("failed_records", failedRecords),
				zap.Duration("processing_duration", time.Since(startTime)),
			)
		}
	}()

	// Process records with parallel processing if enabled
	if sp.config.ParallelProcessing {
		return sp.processRecordsParallel(ctx, records, handler, &processedRecords, &failedRecords)
	}

	// Sequential processing with retry logic
	return sp.processRecordsSequential(ctx, records, handler, &processedRecords, &failedRecords)
}

// processRecordsParallel processes records concurrently
func (sp *Processor) processRecordsParallel(
	ctx context.Context,
	records []events.DynamoDBEventRecord,
	handler func(ctx context.Context, record events.DynamoDBEventRecord) error,
	processedCount, failedCount *int,
) error {
	semaphore := make(chan struct{}, sp.config.MaxConcurrentRecords)
	errChan := make(chan error, len(records))
	
	for _, record := range records {
		go func(r events.DynamoDBEventRecord) {
			semaphore <- struct{}{} // Acquire
			defer func() { <-semaphore }() // Release

			if err := sp.processRecordWithRetry(ctx, r, handler); err != nil {
				errChan <- err
				*failedCount++
			} else {
				*processedCount++
			}
		}(record)
	}

	// Wait for all goroutines and collect errors
	var errors []error
	for i := 0; i < len(records); i++ {
		select {
		case err := <-errChan:
			if err != nil {
				errors = append(errors, err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to process %d records: %v", len(errors), errors)
	}

	return nil
}

// processRecordsSequential processes records one by one
func (sp *Processor) processRecordsSequential(
	ctx context.Context,
	records []events.DynamoDBEventRecord,
	handler func(ctx context.Context, record events.DynamoDBEventRecord) error,
	processedCount, failedCount *int,
) error {
	for _, record := range records {
		if err := sp.processRecordWithRetry(ctx, record, handler); err != nil {
			*failedCount++
			// Continue processing other records even if one fails
			sp.logger.Error("failed to process stream record after retries",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err),
			)
			continue
		}
		*processedCount++
	}

	return nil
}

// processRecordWithRetry processes a single record with retry logic
func (sp *Processor) processRecordWithRetry(
	ctx context.Context,
	record events.DynamoDBEventRecord,
	handler func(ctx context.Context, record events.DynamoDBEventRecord) error,
) error {
	var lastErr error

	for attempt := 0; attempt <= sp.config.MaxRetryAttempts; attempt++ {
		// Skip records we don't care about
		if record.EventName != eventNameInsert && record.EventName != eventNameModify && record.EventName != eventNameRemove {
			return nil
		}

		// Process the record with the handler (handler handles unmarshaling)
		if err := handler(ctx, record); err != nil {
			lastErr = err
			if !sp.isRetryableError(err) {
				break
			}

			sp.logger.Warn("stream_record_processing_failed_retrying",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Int("attempt", attempt+1),
				zap.Int("max_attempts", sp.config.MaxRetryAttempts+1),
				zap.Error(err),
			)

			sp.applyBackoff(attempt)
			continue
		}

		// Success
		return nil
	}

	// All retries exhausted
	return fmt.Errorf("failed to process record after %d attempts: %w", sp.config.MaxRetryAttempts+1, lastErr)
}

// isRetryableError determines if an error should trigger a retry
func (sp *Processor) isRetryableError(err error) bool {
	if !sp.config.EnableErrorRecovery {
		return false
	}

	errStr := err.Error()
	retryableErrors := []string{
		"ProvisionedThroughputExceededException",
		"ThrottlingException",
		"ServiceUnavailableException",
		"RequestLimitExceeded",
		"InternalServerError",
		"connection",
		"timeout",
		"temporary",
	}

	for _, retryableErr := range retryableErrors {
		if contains(errStr, retryableErr) {
			return true
		}
	}

	return false
}

// applyBackoff applies exponential backoff with jitter
func (sp *Processor) applyBackoff(attempt int) {
	if attempt == 0 {
		return
	}

	// Clamp attempt to prevent integer overflow in bit shifting
	safeAttempt := attempt - 1
	if safeAttempt > 30 { // 2^30 is already very large, prevent overflow
		safeAttempt = 30
	}
	// #nosec G115 - attempt is clamped to prevent overflow
	backoff := sp.config.RetryBackoffInitial * time.Duration(1<<uint(safeAttempt))
	if backoff > sp.config.RetryBackoffMax {
		backoff = sp.config.RetryBackoffMax
	}

	// Add jitter (up to 25% of backoff time)
	// #nosec G404 - Using math/rand for jitter is acceptable for backoff timing
	jitter := time.Duration(float64(backoff) * 0.25 * (rand.Float64() - 0.5)) // Add random jitter
	
	sp.logger.Debug("applying_backoff",
		zap.Duration("backoff_duration", backoff+jitter),
		zap.Int("attempt", attempt),
	)

	time.Sleep(backoff + jitter)
}

// unmarshalWithDynamORMCompat provides DynamORM-compatible unmarshaling with enhanced error handling
func unmarshalWithDynamORMCompat(item map[string]any, out any) error {
	outVal := reflect.ValueOf(out)
	if outVal.Kind() != reflect.Ptr || outVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("out must be a pointer to struct")
	}

	outVal = outVal.Elem()
	outType := outVal.Type()

	// Enhanced field mapping with DynamORM tag support
	for i := 0; i < outType.NumField(); i++ {
		field := outType.Field(i)
		fieldVal := outVal.Field(i)

		if !fieldVal.CanSet() {
			continue
		}

		// Get field name from DynamORM tag or JSON tag or field name
		fieldName := getFieldName(field)
		
		// Check if the field exists in the item map
		if val, ok := item[fieldName]; ok {
			if err := setFieldValueEnhanced(fieldVal, val, field); err != nil {
				common.Logger().Warn("Failed to set field value",
					zap.String("field", fieldName),
					zap.String("field_type", field.Type.String()),
					zap.Error(err),
				)
				// Continue processing other fields
			}
		}
	}

	return nil
}

// getFieldName extracts field name from struct tags (dynamorm, json) or uses field name
func getFieldName(field reflect.StructField) string {
	// Check for DynamORM tag first
	if tag, ok := field.Tag.Lookup("dynamorm"); ok {
		// Skip special DynamORM tags like "pk", "sk", etc.
		if tag != "pk" && tag != "sk" && !contains(tag, "index:") && tag != "ttl" && tag != "version" {
			return tag
		}
	}

	// Check for JSON tag
	if tag, ok := field.Tag.Lookup("json"); ok {
		// Extract the actual field name before any options like "omitempty"
		if idx := strings.Index(tag, ","); idx != -1 {
			return tag[:idx]
		}
		return tag
	}

	// Use struct field name as fallback
	return field.Name
}

// setFieldValueEnhanced provides enhanced field value setting with better type handling
func setFieldValueEnhanced(field reflect.Value, value any, structField reflect.StructField) error {
	if value == nil {
		return nil
	}

	// Try direct assignment first
	if tryDirectAssignmentEnhanced(field, value) {
		return nil
	}

	// Handle type conversions with enhanced support
	switch field.Kind() {
	case reflect.String:
		return setStringFieldEnhanced(field, value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return setIntFieldEnhanced(field, value)
	case reflect.Float32, reflect.Float64:
		return setFloatFieldEnhanced(field, value)
	case reflect.Bool:
		return setBoolFieldEnhanced(field, value)
	case reflect.Slice:
		return setSliceFieldEnhanced(field, value)
	case reflect.Map:
		return setMapFieldEnhanced(field, value)
	case reflect.Struct:
		return setStructFieldEnhanced(field, value, structField)
	case reflect.Ptr:
		return setPtrFieldEnhanced(field, value, structField)
	default:
		return fmt.Errorf("unsupported field type: %v", field.Type())
	}
}

// Enhanced field setters with better error handling and type conversion

func tryDirectAssignmentEnhanced(field reflect.Value, value any) bool {
	valueType := reflect.TypeOf(value)
	if valueType == nil {
		return false
	}
	if valueType.AssignableTo(field.Type()) {
		field.Set(reflect.ValueOf(value))
		return true
	}
	return false
}

func setStringFieldEnhanced(field reflect.Value, value any) error {
	switch v := value.(type) {
	case string:
		field.SetString(v)
		return nil
	case []byte:
		field.SetString(string(v))
		return nil
	case fmt.Stringer:
		field.SetString(v.String())
		return nil
	default:
		return fmt.Errorf("cannot convert %T to string", value)
	}
}

func setIntFieldEnhanced(field reflect.Value, value any) error {
	switch v := value.(type) {
	case string:
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			if field.OverflowInt(num) {
				return fmt.Errorf("integer overflow: %d", num)
			}
			field.SetInt(num)
			return nil
		}
		return fmt.Errorf("invalid integer string: %s", v)
	case float64:
		if field.OverflowInt(int64(v)) {
			return fmt.Errorf("integer overflow: %f", v)
		}
		field.SetInt(int64(v))
		return nil
	case int, int8, int16, int32, int64:
		val := reflect.ValueOf(v).Int()
		if field.OverflowInt(val) {
			return fmt.Errorf("integer overflow: %d", val)
		}
		field.SetInt(val)
		return nil
	default:
		return fmt.Errorf("cannot convert %T to int", value)
	}
}

func setFloatFieldEnhanced(field reflect.Value, value any) error {
	switch v := value.(type) {
	case string:
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			if field.OverflowFloat(num) {
				return fmt.Errorf("float overflow: %f", num)
			}
			field.SetFloat(num)
			return nil
		}
		return fmt.Errorf("invalid float string: %s", v)
	case float64:
		if field.OverflowFloat(v) {
			return fmt.Errorf("float overflow: %f", v)
		}
		field.SetFloat(v)
		return nil
	case float32:
		if field.OverflowFloat(float64(v)) {
			return fmt.Errorf("float overflow: %f", v)
		}
		field.SetFloat(float64(v))
		return nil
	default:
		return fmt.Errorf("cannot convert %T to float", value)
	}
}

func setBoolFieldEnhanced(field reflect.Value, value any) error {
	switch v := value.(type) {
	case bool:
		field.SetBool(v)
		return nil
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			field.SetBool(b)
			return nil
		}
		return fmt.Errorf("invalid bool string: %s", v)
	default:
		return fmt.Errorf("cannot convert %T to bool", value)
	}
}

func setSliceFieldEnhanced(field reflect.Value, value any) error {
	list, ok := value.([]any)
	if !ok {
		return fmt.Errorf("expected slice, got %T", value)
	}

	sliceVal := reflect.MakeSlice(field.Type(), len(list), len(list))
	for i, item := range list {
		if err := setFieldValueEnhanced(sliceVal.Index(i), item, reflect.StructField{}); err != nil {
			return fmt.Errorf("failed to set slice element %d: %w", i, err)
		}
	}
	field.Set(sliceVal)
	return nil
}

func setMapFieldEnhanced(field reflect.Value, value any) error {
	m, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map[string]any, got %T", value)
	}

	mapVal := reflect.MakeMap(field.Type())
	for k, v := range m {
		keyVal := reflect.ValueOf(k)
		valVal := reflect.New(field.Type().Elem()).Elem()
		if err := setFieldValueEnhanced(valVal, v, reflect.StructField{}); err != nil {
			return fmt.Errorf("failed to set map value for key %s: %w", k, err)
		}
		mapVal.SetMapIndex(keyVal, valVal)
	}
	field.Set(mapVal)
	return nil
}

func setStructFieldEnhanced(field reflect.Value, value any, _ reflect.StructField) error {
	// Handle time.Time specially
	if field.Type() == reflect.TypeOf(time.Time{}) {
		return setTimeField(field, value)
	}

	// Handle other struct types by attempting to unmarshal into them
	if mapVal, ok := value.(map[string]any); ok {
		return unmarshalWithDynamORMCompat(mapVal, field.Addr().Interface())
	}

	return fmt.Errorf("cannot convert %T to struct %v", value, field.Type())
}

func setPtrFieldEnhanced(field reflect.Value, value any, structField reflect.StructField) error {
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	// Create new instance of the pointed type
	elemType := field.Type().Elem()
	elemVal := reflect.New(elemType).Elem()

	if err := setFieldValueEnhanced(elemVal, value, structField); err != nil {
		return err
	}

	field.Set(elemVal.Addr())
	return nil
}

func setTimeField(field reflect.Value, value any) error {
	switch v := value.(type) {
	case string:
		// Try multiple time formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				field.Set(reflect.ValueOf(t))
				return nil
			}
		}
		// Try Unix timestamp as string
		if timestamp, err := strconv.ParseInt(v, 10, 64); err == nil {
			t := time.Unix(timestamp, 0)
			field.Set(reflect.ValueOf(t))
			return nil
		}
		return fmt.Errorf("unable to parse time string: %s", v)
	case float64:
		// Unix timestamp
		t := time.Unix(int64(v), 0)
		field.Set(reflect.ValueOf(t))
		return nil
	case int64:
		t := time.Unix(v, 0)
		field.Set(reflect.ValueOf(t))
		return nil
	default:
		return fmt.Errorf("cannot convert %T to time.Time", value)
	}
}

// Utility functions

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// Event name constants are defined in handler.go
