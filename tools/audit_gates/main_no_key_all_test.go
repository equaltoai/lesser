package main

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// countUnkeyedAllInSrc parses src as a Go file and returns how many key-less
// All(...) chains the detector counts in it.
func countUnkeyedAllInSrc(t *testing.T, src string) int {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	n, err := countGoUnkeyedAllCallsInFile(path)
	if err != nil {
		t.Fatalf("countGoUnkeyedAllCallsInFile: %v", err)
	}
	return n
}

func TestCountGoUnkeyedAllCallsKeyConditionSemantics(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "fresh chain with non-key Where filter is flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.WithContext(ctx).Model(&Item{}).Where("Status", "=", "active").All(&out)
}
`,
			want: 1,
		},
		{
			name: "fresh chain with sort-key range only is flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("SK", ">", cursor).All(&out)
}
`,
			want: 1,
		},
		{
			name: "fresh chain with partition-key equality is not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.WithContext(ctx).Model(&Item{}).Where("PK", "=", "USER#x").All(&out)
}
`,
			want: 0,
		},
		{
			name: "fresh chain with GSI partition-key equality is not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("gsi1PK", "=", "TOKEN#x").All(&out)
}
`,
			want: 0,
		},
		{
			name: "key equality among other filters is not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.WithContext(ctx).Model(&Item{}).Where("Status", "=", "active").Where("gsi8PK", "=", "RELAYS").Where("SK", "<", cursor).All(&out)
}
`,
			want: 0,
		},
		{
			name: "range operator on partition key is flagged (demoted to filter)",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("PK", ">", "USER#x").All(&out)
}
`,
			want: 1,
		},
		{
			name: "begins_with on partition key is flagged (demoted to filter)",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("PK", "begins_with", "USER#").All(&out)
}
`,
			want: 1,
		},
		{
			name: "fresh chain with no Where remains flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.WithContext(ctx).Model(&Item{}).All(&out)
}
`,
			want: 1,
		},
		{
			name: "fresh chain with Filter only remains flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Filter("UpdatedAt", "<", before).All(&out)
}
`,
			want: 1,
		},
		{
			name: "pre-built query variable All is not flagged",
			src: `package fixture

func f(db DB) {
	query := db.WithContext(ctx).Model(&Item{}).Where("Status", "=", "active")
	var out []Item
	_ = query.All(&out)
}
`,
			want: 0,
		},
		{
			name: "chain without Model/WithContext construct is not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Where("Status", "=", "active").All(&out)
}
`,
			want: 0,
		},
		{
			name: "non-literal Where field is indeterminate and not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where(prefix+"PK", "=", "USER#x").All(&out)
}
`,
			want: 0,
		},
		{
			name: "non-literal Where operator is indeterminate and not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("PK", op, "USER#x").All(&out)
}
`,
			want: 0,
		},
		{
			name: "non-literal Where among literal filters is indeterminate",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("Status", "=", "active").Where(attr, ">", cursor).All(&out)
}
`,
			want: 0,
		},
		{
			name: "First on a fresh chain is not counted",
			src: `package fixture

func f(db DB) {
	var out Item
	_ = db.Model(&Item{}).Where("Status", "=", "active").First(&out)
}
`,
			want: 0,
		},
		{
			// Raw-condition Where form, literal bound values. TableTheory v3.0.6
			// Query.Where(field, op, value) is strictly 3-arg — the "field"
			// string "PK = ? AND SK = ?" is taken literally and matches no model
			// attribute, so the condition is a filter, never a key condition, and
			// the chain compiles to a Scan. The detector parses this
			// determinately (both args literal, field not a partition key) and
			// flags it — correct.
			name: "raw-condition Where form with literal bound values is flagged (non-key filter)",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("PK = ? AND SK = ?", "USER#x", "POST#1").All(&out)
}
`,
			want: 1,
		},
		{
			// Raw-condition Where form, variable bound values: args[1] is the
			// bound value, not an operator literal, so keyConditionOfWhere
			// reports indeterminate and the chain is deliberately NOT flagged
			// (conservative direction, same rule as any non-literal operator).
			name: "raw-condition Where form with variable bound values is indeterminate and not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("PK = ? AND SK = ?", pk, sk).All(&out)
}
`,
			want: 0,
		},
		{
			// Whitespace-padded operators are normalized identically to
			// TableTheory's partitionConditionsForKeys (strings.ToUpper +
			// strings.TrimSpace), so " = " still binds the partition key.
			name: "whitespace-padded equality operator on partition key is not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("PK", " = ", "USER#x").All(&out)
}
`,
			want: 0,
		},
		{
			name: "oauthClientsPK equality on the oauth-clients-index is not flagged",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.WithContext(ctx).Model(&Item{}).Index("oauth-clients-index").Where("oauthClientsPK", "=", "OAUTH_CLIENTS").All(&out)
}
`,
			want: 0,
		},
		{
			name: "non-equality operator on oauthClientsPK is flagged (demoted to filter)",
			src: `package fixture

func f(db DB) {
	var out []Item
	_ = db.Model(&Item{}).Where("oauthClientsPK", "begins_with", "OAUTH").All(&out)
}
`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countUnkeyedAllInSrc(t, tt.src)
			if got != tt.want {
				t.Errorf("countGoUnkeyedAllCallsInFile = %d, want %d\nsrc:\n%s", got, tt.want, tt.src)
			}
		})
	}
}

// TestCountGoUnkeyedAllCallsBaselineFilesSanity guards the detection-honesty
// invariant: the number of sites the gate reports for the tracked files must
// never drop below the current baseline after a detector change.
func TestCountGoUnkeyedAllCallsBaselineFilesSanity(t *testing.T) {
	t.Chdir("../..")

	b, err := loadBaseline("tools/audit_gates/baseline.yml")
	if err != nil {
		t.Fatalf("loadBaseline: %v", err)
	}
	for path, want := range b.GoDynamoDBAllNoKey {
		n, err := countGoUnkeyedAllCallsInFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("countGoUnkeyedAllCallsInFile(%q): %v", path, err)
		}
		if n < want {
			t.Errorf("detection regression for %s: detector reports %d, baseline expects at least %d", path, n, want)
		}
	}
}

// countUnkeyedCountInSrc parses src as a Go file and returns how many key-less
// Count(...) chains the detector counts in it.
func countUnkeyedCountInSrc(t *testing.T, src string) int {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	n, err := countGoUnkeyedCountCallsInFile(path)
	if err != nil {
		t.Fatalf("countGoUnkeyedCountCallsInFile: %v", err)
	}
	return n
}

// TestCountGoUnkeyedCountCallsKeyConditionSemantics mirrors the All() scenarios:
// Count() shares All()'s compile path (tabletheory v3.0.6
// pkg/query/query_execution.go:80-111 — Compile() decides Query vs Scan, then
// ExecuteScan runs Select=COUNT), so a key-less fresh-chain Count is a counted
// full-table scan and must be flagged under the same rules.
func TestCountGoUnkeyedCountCallsKeyConditionSemantics(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "fresh chain Count with no Where is flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.WithContext(ctx).Model(&Item{}).Count()
}
`,
			want: 1,
		},
		{
			name: "fresh chain Count with non-key Where filter is flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.WithContext(ctx).Model(&Item{}).Where("Status", "=", "active").Count()
}
`,
			want: 1,
		},
		{
			name: "fresh chain Count with sort-key range only is flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.Model(&Item{}).Where("SK", ">", cursor).Count()
}
`,
			want: 1,
		},
		{
			name: "fresh chain Count with partition-key equality is not flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.WithContext(ctx).Model(&Item{}).Where("PK", "=", "USER#x").Count()
}
`,
			want: 0,
		},
		{
			name: "fresh chain Count with GSI partition-key equality is not flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.Model(&Item{}).Where("gsi1PK", "=", "TOKEN#x").Count()
}
`,
			want: 0,
		},
		{
			name: "range operator on partition key Count is flagged (demoted to filter)",
			src: `package fixture

func f(db DB) {
	_, _ = db.Model(&Item{}).Where("PK", ">", "USER#x").Count()
}
`,
			want: 1,
		},
		{
			name: "fresh chain Count with Filter only is flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.Model(&Item{}).Filter("UpdatedAt", "<", before).Count()
}
`,
			want: 1,
		},
		{
			name: "pre-built query variable Count is not flagged",
			src: `package fixture

func f(db DB) {
	query := db.WithContext(ctx).Model(&Item{}).Where("Status", "=", "active")
	_, _ = query.Count()
}
`,
			want: 0,
		},
		{
			name: "chain without Model/WithContext construct Count is not flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.Where("Status", "=", "active").Count()
}
`,
			want: 0,
		},
		{
			name: "non-literal Where field Count is indeterminate and not flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.Model(&Item{}).Where(prefix+"PK", "=", "USER#x").Count()
}
`,
			want: 0,
		},
		{
			// Raw-condition Where form, literal bound values: the detector sees a
			// determinate non-key condition (field "PK = ? AND SK = ?" is literal
			// and matches no model attribute) and flags the counted scan —
			// correct for tabletheory v3.0.6, whose Where(field, op, value) takes
			// the field string literally.
			name: "raw-condition Where form with literal bound values Count is flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.Model(&Item{}).Where("PK = ? AND SK = ?", "USER#x", "POST#1").Count()
}
`,
			want: 1,
		},
		{
			// Raw-condition Where form, variable bound values: indeterminate,
			// conservative not-flagged (same rule as any non-literal operator).
			name: "raw-condition Where form with variable bound values Count is indeterminate and not flagged",
			src: `package fixture

func f(db DB) {
	_, _ = db.Model(&Item{}).Where("PK = ? AND SK = ?", pk, sk).Count()
}
`,
			want: 0,
		},
		{
			name: "non-query Count method on a window type is not flagged",
			src: `package fixture

type window struct{}

func (w window) Count() int { return 0 }

func f(w window) {
	_ = w.Count()
}
`,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countUnkeyedCountInSrc(t, tt.src)
			if got != tt.want {
				t.Errorf("countGoUnkeyedCountCallsInFile = %d, want %d\nsrc:\n%s", got, tt.want, tt.src)
			}
		})
	}
}

// TestCountGoUnkeyedCountCallsBaselineFilesSanity guards the same
// detection-honesty invariant as the All() sanity test: the count gate must
// never report fewer sites than the baseline for the tracked files.
func TestCountGoUnkeyedCountCallsBaselineFilesSanity(t *testing.T) {
	t.Chdir("../..")

	b, err := loadBaseline("tools/audit_gates/baseline.yml")
	if err != nil {
		t.Fatalf("loadBaseline: %v", err)
	}
	for path, want := range b.GoDynamoDBCountNoKey {
		n, err := countGoUnkeyedCountCallsInFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("countGoUnkeyedCountCallsInFile(%q): %v", path, err)
		}
		if n < want {
			t.Errorf("detection regression for %s: detector reports %d, baseline expects at least %d", path, n, want)
		}
	}
}

func TestKeyConditionOfWhere(t *testing.T) {
	call := func(field, op string) *ast.CallExpr {
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "q"},
				Sel: &ast.Ident{Name: "Where"},
			},
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(field)},
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(op)},
				&ast.Ident{Name: "v"},
			},
		}
	}

	tests := []struct {
		name            string
		call            *ast.CallExpr
		wantBounded     bool
		wantDeterminate bool
	}{
		{"PK equality", call("PK", "="), true, true},
		{"gsi1PK equality", call("gsi1PK", "="), true, true},
		{"gsi8PK equality", call("gsi8PK", "="), true, true},
		{"oauthClientsPK equality", call("oauthClientsPK", "="), true, true},
		{"PK begins_with", call("PK", "begins_with"), false, true},
		{"PK range", call("PK", ">"), false, true},
		{"oauthClientsPK begins_with", call("oauthClientsPK", "begins_with"), false, true},
		{"SK equality", call("SK", "="), false, true},
		{"gsi1SK equality", call("gsi1SK", "="), false, true},
		{"non-key attribute", call("Status", "="), false, true},
		{"ID equality", call("ID", "="), false, true},
		{"PKX equality", call("PKX", "="), false, true},
		{"gsiPK equality", call("gsiPK", "="), false, true},
		// Whitespace-padded operators are normalized (TrimSpace + ToUpper) the
		// same way TableTheory's partitionConditionsForKeys normalizes them.
		{"PK equality padded spaces", call("PK", " = "), true, true},
		{"PK equality padded tabs", call("PK", "\t=\t"), true, true},
		{"PK range padded", call("PK", "  >  "), false, true},
		// Case-sensitivity matches the shared isPartitionKeyField helper (the
		// sibling BadPKWhere gate); canonical spelling is lowercase "gsiNPK".
		{"mixed-case GSI PK", call("Gsi1PK", "="), false, true},
		{"capitalized OAuthClientsPK is not the attr name", call("OAuthClientsPK", "="), false, true},
		// Non-literal field or operator is indeterminate.
		{"non-literal field", &ast.CallExpr{Args: []ast.Expr{&ast.Ident{Name: "f"}, &ast.BasicLit{Kind: token.STRING, Value: `"="`}, &ast.Ident{Name: "v"}}}, false, false},
		{"non-literal operator", &ast.CallExpr{Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"PK"`}, &ast.Ident{Name: "op"}, &ast.Ident{Name: "v"}}}, false, false},
		{"too few args", &ast.CallExpr{Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"PK"`}}}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bounded, determinate := keyConditionOfWhere(tt.call)
			if bounded != tt.wantBounded || determinate != tt.wantDeterminate {
				t.Errorf("keyConditionOfWhere = (%v, %v), want (%v, %v)", bounded, determinate, tt.wantBounded, tt.wantDeterminate)
			}
		})
	}
}

func TestIsPartitionKeyFieldExtended(t *testing.T) {
	valid := []string{"PK", "gsi1PK", "gsi2PK", "gsi8PK", "gsi9PK", "gsi0PK", "oauthClientsPK"}
	for _, f := range valid {
		if !isPartitionKeyField(f) {
			t.Errorf("isPartitionKeyField(%q) = false, want true", f)
		}
	}
	invalid := []string{"", "SK", "gsi1SK", "gsiPK", "gsixPK", "PKX", "pK", "OAuthClientsPK", "oauthClientPK", "oauthClientsSK"}
	for _, f := range invalid {
		if isPartitionKeyField(f) {
			t.Errorf("isPartitionKeyField(%q) = true, want false", f)
		}
	}
}
