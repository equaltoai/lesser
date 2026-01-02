package streaming

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
)

type fakeDynamormDB struct {
	mu sync.Mutex

	items map[string]reflect.Value

	forceFirstErr  error
	forceCreateErr error
	forceUpdateErr error
}

func newFakeDynamormDB() *fakeDynamormDB {
	return &fakeDynamormDB{
		items: make(map[string]reflect.Value),
	}
}

func (db *fakeDynamormDB) Model(model any) dynamormCore.Query {
	return &fakeDynamormQuery{
		db:     db,
		model:  model,
		wheres: make(map[string]any),
	}
}

func (db *fakeDynamormDB) Transaction(fn func(tx *dynamormCore.Tx) error) error {
	tx := &dynamormCore.Tx{}
	tx.SetDB(db)
	return fn(tx)
}

func (db *fakeDynamormDB) Migrate() error                 { return nil }
func (db *fakeDynamormDB) AutoMigrate(models ...any) error { return nil }
func (db *fakeDynamormDB) Close() error                   { return nil }
func (db *fakeDynamormDB) WithContext(_ context.Context) dynamormCore.DB {
	return db
}

type fakeDynamormQuery struct {
	db     *fakeDynamormDB
	model  any
	wheres map[string]any
}

func (q *fakeDynamormQuery) Where(field string, _ string, value any) dynamormCore.Query {
	q.wheres[field] = value
	return q
}

func (q *fakeDynamormQuery) Index(_ string) dynamormCore.Query                                       { return q }
func (q *fakeDynamormQuery) Filter(_ string, _ string, _ any) dynamormCore.Query                     { return q }
func (q *fakeDynamormQuery) OrFilter(_ string, _ string, _ any) dynamormCore.Query                   { return q }
func (q *fakeDynamormQuery) FilterGroup(fn func(dynamormCore.Query)) dynamormCore.Query              { fn(q); return q }
func (q *fakeDynamormQuery) OrFilterGroup(fn func(dynamormCore.Query)) dynamormCore.Query            { fn(q); return q }
func (q *fakeDynamormQuery) IfNotExists() dynamormCore.Query                                         { return q }
func (q *fakeDynamormQuery) IfExists() dynamormCore.Query                                            { return q }
func (q *fakeDynamormQuery) WithCondition(_ string, _ string, _ any) dynamormCore.Query              { return q }
func (q *fakeDynamormQuery) WithConditionExpression(_ string, _ map[string]any) dynamormCore.Query    { return q }
func (q *fakeDynamormQuery) OrderBy(_ string, _ string) dynamormCore.Query                            { return q }
func (q *fakeDynamormQuery) Limit(_ int) dynamormCore.Query                                           { return q }
func (q *fakeDynamormQuery) Offset(_ int) dynamormCore.Query                                          { return q }
func (q *fakeDynamormQuery) Select(_ ...string) dynamormCore.Query                                    { return q }
func (q *fakeDynamormQuery) ConsistentRead() dynamormCore.Query                                       { return q }
func (q *fakeDynamormQuery) WithRetry(_ int, _ time.Duration) dynamormCore.Query                       { return q }
func (q *fakeDynamormQuery) All(_ any) error                                                          { return nil }
func (q *fakeDynamormQuery) AllPaginated(_ any) (*dynamormCore.PaginatedResult, error)                { return &dynamormCore.PaginatedResult{}, nil }
func (q *fakeDynamormQuery) Count() (int64, error)                                                     { return 0, nil }
func (q *fakeDynamormQuery) CreateOrUpdate() error                                                     { return q.Create() }
func (q *fakeDynamormQuery) UpdateBuilder() dynamormCore.UpdateBuilder                                 { return &fakeUpdateBuilder{} }
func (q *fakeDynamormQuery) Delete() error                                                             { return nil }
func (q *fakeDynamormQuery) Scan(_ any) error                                                          { return nil }
func (q *fakeDynamormQuery) ParallelScan(_ int32, _ int32) dynamormCore.Query                          { return q }
func (q *fakeDynamormQuery) ScanAllSegments(_ any, _ int32) error                                      { return nil }
func (q *fakeDynamormQuery) BatchGet(_ []any, _ any) error                                             { return nil }
func (q *fakeDynamormQuery) BatchGetWithOptions(_ []any, _ any, _ *dynamormCore.BatchGetOptions) error { return nil }
func (q *fakeDynamormQuery) BatchGetBuilder() dynamormCore.BatchGetBuilder                             { return &fakeBatchGetBuilder{} }
func (q *fakeDynamormQuery) BatchCreate(_ any) error                                                   { return nil }
func (q *fakeDynamormQuery) BatchDelete(_ []any) error                                                 { return nil }
func (q *fakeDynamormQuery) BatchWrite(_ []any, _ []any) error                                         { return nil }
func (q *fakeDynamormQuery) BatchUpdateWithOptions(_ []any, _ []string, _ ...any) error                { return nil }
func (q *fakeDynamormQuery) Cursor(_ string) dynamormCore.Query                                        { return q }
func (q *fakeDynamormQuery) SetCursor(_ string) error                                                  { return nil }
func (q *fakeDynamormQuery) WithContext(_ context.Context) dynamormCore.Query                          { return q }

func (q *fakeDynamormQuery) First(dest any) error {
	if q.db.forceFirstErr != nil {
		return q.db.forceFirstErr
	}

	key, ok := q.keyFromWheres()
	if !ok {
		return errors.New("missing PK/SK")
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()

	stored, ok := q.db.items[key]
	if !ok {
		return errors.New("item not found")
	}

	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Pointer {
		return errors.New("dest must be pointer")
	}

	if stored.Type().AssignableTo(destVal.Elem().Type()) {
		destVal.Elem().Set(stored)
		return nil
	}

	if stored.CanAddr() && stored.Addr().Type().AssignableTo(destVal.Elem().Type()) {
		destVal.Elem().Set(stored.Addr())
		return nil
	}

	return fmt.Errorf("type mismatch: stored=%s dest=%s", stored.Type(), destVal.Elem().Type())
}

func (q *fakeDynamormQuery) Create() error {
	if q.db.forceCreateErr != nil {
		return q.db.forceCreateErr
	}

	key, ok := keyFromModel(q.model)
	if !ok {
		return errors.New("model missing PK/SK")
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()
	q.db.items[key] = reflect.Indirect(reflect.ValueOf(q.model))
	return nil
}

func (q *fakeDynamormQuery) Update(_ ...string) error {
	if q.db.forceUpdateErr != nil {
		return q.db.forceUpdateErr
	}

	key, ok := q.keyFromWheres()
	if !ok {
		key, ok = keyFromModel(q.model)
	}
	if !ok {
		return errors.New("missing PK/SK")
	}

	q.db.mu.Lock()
	defer q.db.mu.Unlock()

	if _, exists := q.db.items[key]; !exists {
		return errors.New("item not found")
	}

	q.db.items[key] = reflect.Indirect(reflect.ValueOf(q.model))
	return nil
}

func (q *fakeDynamormQuery) keyFromWheres() (string, bool) {
	pk, okPK := q.wheres["PK"].(string)
	sk, okSK := q.wheres["SK"].(string)
	if !okPK || !okSK {
		return "", false
	}
	return pk + "|" + sk, true
}

func keyFromModel(model any) (string, bool) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}

	pkField := v.FieldByName("PK")
	skField := v.FieldByName("SK")
	if !pkField.IsValid() || !skField.IsValid() {
		return "", false
	}
	if pkField.Kind() != reflect.String || skField.Kind() != reflect.String {
		return "", false
	}
	return pkField.String() + "|" + skField.String(), true
}

type fakeUpdateBuilder struct{}

func (b *fakeUpdateBuilder) Set(_ string, _ any) dynamormCore.UpdateBuilder                      { return b }
func (b *fakeUpdateBuilder) SetIfNotExists(_ string, _ any, _ any) dynamormCore.UpdateBuilder    { return b }
func (b *fakeUpdateBuilder) Add(_ string, _ any) dynamormCore.UpdateBuilder                      { return b }
func (b *fakeUpdateBuilder) Increment(_ string) dynamormCore.UpdateBuilder                       { return b }
func (b *fakeUpdateBuilder) Decrement(_ string) dynamormCore.UpdateBuilder                       { return b }
func (b *fakeUpdateBuilder) Remove(_ string) dynamormCore.UpdateBuilder                          { return b }
func (b *fakeUpdateBuilder) Delete(_ string, _ any) dynamormCore.UpdateBuilder                   { return b }
func (b *fakeUpdateBuilder) AppendToList(_ string, _ any) dynamormCore.UpdateBuilder             { return b }
func (b *fakeUpdateBuilder) PrependToList(_ string, _ any) dynamormCore.UpdateBuilder            { return b }
func (b *fakeUpdateBuilder) RemoveFromListAt(_ string, _ int) dynamormCore.UpdateBuilder         { return b }
func (b *fakeUpdateBuilder) SetListElement(_ string, _ int, _ any) dynamormCore.UpdateBuilder    { return b }
func (b *fakeUpdateBuilder) Condition(_ string, _ string, _ any) dynamormCore.UpdateBuilder      { return b }
func (b *fakeUpdateBuilder) OrCondition(_ string, _ string, _ any) dynamormCore.UpdateBuilder    { return b }
func (b *fakeUpdateBuilder) ConditionExists(_ string) dynamormCore.UpdateBuilder                 { return b }
func (b *fakeUpdateBuilder) ConditionNotExists(_ string) dynamormCore.UpdateBuilder              { return b }
func (b *fakeUpdateBuilder) ConditionVersion(_ int64) dynamormCore.UpdateBuilder                 { return b }
func (b *fakeUpdateBuilder) ReturnValues(_ string) dynamormCore.UpdateBuilder                    { return b }
func (b *fakeUpdateBuilder) Execute() error                                                      { return nil }
func (b *fakeUpdateBuilder) ExecuteWithResult(_ any) error                                        { return nil }

type fakeBatchGetBuilder struct{}

func (b *fakeBatchGetBuilder) Keys(_ []any) dynamormCore.BatchGetBuilder                   { return b }
func (b *fakeBatchGetBuilder) ChunkSize(_ int) dynamormCore.BatchGetBuilder                { return b }
func (b *fakeBatchGetBuilder) ConsistentRead() dynamormCore.BatchGetBuilder                { return b }
func (b *fakeBatchGetBuilder) Parallel(_ int) dynamormCore.BatchGetBuilder                 { return b }
func (b *fakeBatchGetBuilder) WithRetry(_ *dynamormCore.RetryPolicy) dynamormCore.BatchGetBuilder {
	return b
}
func (b *fakeBatchGetBuilder) Select(_ ...string) dynamormCore.BatchGetBuilder                         { return b }
func (b *fakeBatchGetBuilder) OnProgress(_ dynamormCore.BatchProgressCallback) dynamormCore.BatchGetBuilder {
	return b
}
func (b *fakeBatchGetBuilder) OnError(_ dynamormCore.BatchChunkErrorHandler) dynamormCore.BatchGetBuilder {
	return b
}
func (b *fakeBatchGetBuilder) Execute(_ any) error { return nil }

type s3Object struct {
	body []byte
}

type s3Memory struct {
	mu      sync.Mutex
	objects map[string]s3Object
}

func newS3Memory() *s3Memory {
	return &s3Memory{objects: make(map[string]s3Object)}
}

func (m *s3Memory) put(key string, body []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = s3Object{body: body}
}

func (m *s3Memory) get(key string) (s3Object, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, ok := m.objects[key]
	return obj, ok
}

func (m *s3Memory) keysWithPrefix(prefix string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func newTestS3Server(t testing.TB, bucket string, mem *s3Memory) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path-style: /{bucket}/{key...}
		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "missing bucket", http.StatusBadRequest)
			return
		}

		if parts[0] != bucket {
			writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket not found")
			return
		}

		key := ""
		if len(parts) == 2 {
			key = parts[1]
		}

		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			mem.put(key, body)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `<PutObjectResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></PutObjectResult>`)
		case http.MethodHead:
			obj, ok := mem.get(key)
			if !ok {
				writeS3Error(w, http.StatusNotFound, "NotFound", "not found")
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(obj.body)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			// ListObjectsV2 uses GET with list-type=2
			if r.URL.Query().Get("list-type") == "2" {
				prefix := r.URL.Query().Get("prefix")
				keys := mem.keysWithPrefix(prefix)
				writeListObjectsV2Response(w, bucket, prefix, keys, mem)
				return
			}

			obj, ok := mem.get(key)
			if !ok {
				writeS3Error(w, http.StatusNotFound, "NoSuchKey", "key not found")
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(obj.body)
		default:
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		}
	}))
}

func writeS3Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	escapedMessage := xmlEscape(message)
	_, _ = io.WriteString(w, fmt.Sprintf(
		`<Error><Code>%s</Code><Message>%s</Message></Error>`,
		code, escapedMessage))
}

func xmlEscape(s string) string {
	buf := new(bytes.Buffer)
	_ = xml.EscapeText(buf, []byte(s))
	return buf.String()
}

func writeListObjectsV2Response(w http.ResponseWriter, bucket, prefix string, keys []string, mem *s3Memory) {
	type objectEntry struct {
		Key  string `xml:"Key"`
		Size int    `xml:"Size"`
	}

	type listBucketResult struct {
		XMLName     xml.Name `xml:"ListBucketResult"`
		Xmlns       string   `xml:"xmlns,attr"`
		Name        string   `xml:"Name"`
		Prefix      string   `xml:"Prefix"`
		KeyCount    int      `xml:"KeyCount"`
		MaxKeys     int      `xml:"MaxKeys"`
		IsTruncated bool     `xml:"IsTruncated"`
		Contents    []objectEntry `xml:"Contents"`
	}

	result := listBucketResult{
		Xmlns:       "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:        bucket,
		Prefix:      prefix,
		KeyCount:    len(keys),
		MaxKeys:     1000,
		IsTruncated: false,
		Contents:    make([]objectEntry, 0, len(keys)),
	}
	for _, k := range keys {
		obj, _ := mem.get(k)
		result.Contents = append(result.Contents, objectEntry{Key: k, Size: len(obj.body)})
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	out, _ := xml.Marshal(result)
	_, _ = w.Write(out)
}

func newTestS3Client(serverURL string) *s3.Client {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.EndpointResolver = s3.EndpointResolverFromURL(serverURL)
	})
}

type cloudWatchRecorder struct {
	mu sync.Mutex

	putCalls int
	getCalls int

	putShouldError bool
}

func newTestCloudWatchServer(t testing.TB, rec *cloudWatchRecorder) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CloudWatch uses AWS Query protocol, and the AWS SDK may gzip the body.
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
			reader, err := gzip.NewReader(bytes.NewReader(body))
			if err == nil {
				decoded, decodeErr := io.ReadAll(reader)
				_ = reader.Close()
				if decodeErr == nil {
					body = decoded
				}
			}
		}

		values, _ := url.ParseQuery(string(body))
		action := values.Get("Action")
		switch action {
		case "PutMetricData":
			rec.mu.Lock()
			rec.putCalls++
			shouldError := rec.putShouldError
			rec.mu.Unlock()

			if shouldError {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `<ErrorResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><Error><Type>Sender</Type><Code>InternalError</Code><Message>boom</Message></Error></ErrorResponse>`)
				return
			}

			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `<PutMetricDataResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><ResponseMetadata><RequestId>1</RequestId></ResponseMetadata></PutMetricDataResponse>`)
		case "GetMetricStatistics":
			rec.mu.Lock()
			rec.getCalls++
			rec.mu.Unlock()

			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2026-01-02T00:00:00Z</Timestamp><Sum>60000</Sum></member></Datapoints><Label>BytesTransferred</Label></GetMetricStatisticsResult><ResponseMetadata><RequestId>1</RequestId></ResponseMetadata></GetMetricStatisticsResponse>`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "unknown action")
		}
	}))
}

func newTestCloudWatchClient(serverURL string) *cloudwatch.Client {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	}

	return cloudwatch.NewFromConfig(cfg, func(o *cloudwatch.Options) {
		o.EndpointResolver = cloudwatch.EndpointResolverFromURL(serverURL)
	})
}

func urlParseQueryCompat(body string) (urlValues, error) {
	// The AWS Query protocol uses application/x-www-form-urlencoded.
	values, err := parseQuery(body)
	if err != nil {
		return urlValues{}, err
	}
	return urlValues{values: values}, nil
}

type urlValues struct {
	values map[string][]string
}

func (v urlValues) Get(key string) string {
	if v.values == nil {
		return ""
	}
	if vals := v.values[key]; len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func parseQuery(query string) (map[string][]string, error) {
	result := make(map[string][]string)
	if query == "" {
		return result, nil
	}

	pairs := strings.Split(query, "&")
	for _, pair := range pairs {
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 0 {
			continue
		}
		key := parts[0]
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}
		key, err := url.QueryUnescape(key)
		if err != nil {
			return nil, err
		}
		value, err = url.QueryUnescape(value)
		if err != nil {
			return nil, err
		}
		result[key] = append(result[key], value)
	}
	return result, nil
}
