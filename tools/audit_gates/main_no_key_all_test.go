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
		{"PK begins_with", call("PK", "begins_with"), false, true},
		{"PK range", call("PK", ">"), false, true},
		{"SK equality", call("SK", "="), false, true},
		{"gsi1SK equality", call("gsi1SK", "="), false, true},
		{"non-key attribute", call("Status", "="), false, true},
		{"ID equality", call("ID", "="), false, true},
		{"PKX equality", call("PKX", "="), false, true},
		{"gsiPK equality", call("gsiPK", "="), false, true},
		// Case-sensitivity matches the shared isPartitionKeyField helper (the
		// sibling BadPKWhere gate); canonical spelling is lowercase "gsiNPK".
		{"mixed-case GSI PK", call("Gsi1PK", "="), false, true},
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
	valid := []string{"PK", "gsi1PK", "gsi2PK", "gsi8PK", "gsi9PK", "gsi0PK"}
	for _, f := range valid {
		if !isPartitionKeyField(f) {
			t.Errorf("isPartitionKeyField(%q) = false, want true", f)
		}
	}
	invalid := []string{"", "SK", "gsi1SK", "gsiPK", "gsixPK", "PKX", "pK"}
	for _, f := range invalid {
		if isPartitionKeyField(f) {
			t.Errorf("isPartitionKeyField(%q) = true, want false", f)
		}
	}
}
