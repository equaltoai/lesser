// Package security provides security-related utilities and auditing functions
package security

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ValidationFinding represents a security finding from the audit
type ValidationFinding struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	File        string    `json:"file"`
	Line        int       `json:"line"`
	Column      int       `json:"column"`
	Function    string    `json:"function"`
	Issue       string    `json:"issue"`
	Description string    `json:"description"`
	Suggestion  string    `json:"suggestion"`
	CWE         string    `json:"cwe,omitempty"`
	OWASP       string    `json:"owasp,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// AuditReport represents the complete validation audit report
type AuditReport struct {
	Summary   AuditSummary        `json:"summary"`
	Findings  []ValidationFinding `json:"findings"`
	Timestamp time.Time           `json:"timestamp"`
	Duration  time.Duration       `json:"duration"`
}

// AuditSummary provides a summary of audit findings
type AuditSummary struct {
	TotalFiles       int            `json:"total_files"`
	FilesScanned     int            `json:"files_scanned"`
	TotalFindings    int            `json:"total_findings"`
	BySeverity       map[string]int `json:"by_severity"`
	ByType           map[string]int `json:"by_type"`
	CriticalIssues   int            `json:"critical_issues"`
	HighPriorityIssues int          `json:"high_priority_issues"`
}

// InputValidationAuditor performs comprehensive input validation audits
type InputValidationAuditor struct {
	logger       *zap.Logger
	rootPath     string
	fileSet      *token.FileSet
	findings     []ValidationFinding
	summary      AuditSummary
	excludePaths []string
}

// NewInputValidationAuditor creates a new input validation auditor
func NewInputValidationAuditor(logger *zap.Logger, rootPath string) *InputValidationAuditor {
	return &InputValidationAuditor{
		logger:   logger,
		rootPath: rootPath,
		fileSet:  token.NewFileSet(),
		findings: make([]ValidationFinding, 0),
		summary: AuditSummary{
			BySeverity: make(map[string]int),
			ByType:     make(map[string]int),
		},
		excludePaths: []string{
			"vendor/",
			".git/",
			"node_modules/",
			"tests/",
			"_test.go",
			"testdata/",
		},
	}
}

// AuditDirectory performs a comprehensive validation audit on a directory
func (a *InputValidationAuditor) AuditDirectory(ctx context.Context) (*AuditReport, error) {
	start := time.Now()
	a.logger.Info("Starting input validation audit", zap.String("root_path", a.rootPath))

	// Walk the directory tree
	err := filepath.Walk(a.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip excluded paths
		if a.shouldExclude(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process Go files
		if !strings.HasSuffix(path, ".go") || info.IsDir() {
			return nil
		}

		a.summary.TotalFiles++

		// Audit the file
		if auditErr := a.auditFile(ctx, path); auditErr != nil {
			a.logger.Warn("failed to audit file", zap.String("file", path), zap.Error(auditErr))
		} else {
			a.summary.FilesScanned++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	// Calculate summary statistics
	a.calculateSummary()

	duration := time.Since(start)
	
	report := &AuditReport{
		Summary:   a.summary,
		Findings:  a.findings,
		Timestamp: time.Now(),
		Duration:  duration,
	}

	a.logger.Info("Input validation audit completed",
		zap.Int("total_files", a.summary.TotalFiles),
		zap.Int("files_scanned", a.summary.FilesScanned),
		zap.Int("total_findings", a.summary.TotalFindings),
		zap.Duration("duration", duration))

	return report, nil
}

// auditFile audits a single Go file for input validation issues
func (a *InputValidationAuditor) auditFile(ctx context.Context, filePath string) error {
	// Parse the Go file
	src, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	file, err := parser.ParseFile(a.fileSet, filePath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse Go file: %w", err)
	}

	// Create a visitor to analyze the AST
	visitor := &validationVisitor{
		auditor:  a,
		file:     file,
		filePath: filePath,
		src:      src,
		fset:     a.fileSet,
	}

	// Walk the AST
	ast.Walk(visitor, file)

	return nil
}

// validationVisitor implements ast.Visitor to analyze Go code
type validationVisitor struct {
	auditor     *InputValidationAuditor
	file        *ast.File
	filePath    string
	src         []byte
	fset        *token.FileSet
	currentFunc string
}

// Visit implements ast.Visitor interface
func (v *validationVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *ast.FuncDecl:
		v.currentFunc = n.Name.Name
		v.checkFunctionSignature(n)

	case *ast.CallExpr:
		v.checkFunctionCall(n)

	case *ast.AssignStmt:
		v.checkAssignment(n)

	case *ast.IfStmt:
		v.checkConditional(n)

	case *ast.BasicLit:
		v.checkLiteral(n)

	case *ast.BinaryExpr:
		v.checkBinaryExpression(n)
	}

	return v
}

// checkFunctionSignature analyzes function signatures for validation patterns
func (v *validationVisitor) checkFunctionSignature(fn *ast.FuncDecl) {
	if fn.Type.Params == nil {
		return
	}

	// Check for HTTP handlers without proper validation
	if v.isHTTPHandler(fn) {
		v.checkHTTPHandlerValidation(fn)
	}

	// Check for functions accepting string parameters without validation
	for _, param := range fn.Type.Params.List {
		v.checkParameterValidation(fn, param)
	}
}

// checkFunctionCall analyzes function calls for validation issues
func (v *validationVisitor) checkFunctionCall(call *ast.CallExpr) {
	// Get function name
	funcName := v.getFunctionName(call)

	// Check for dangerous function calls
	v.checkDangerousFunctions(call, funcName)

	// Check SQL-related calls
	v.checkSQLInjection(call, funcName)

	// Check HTTP request processing
	v.checkHTTPRequestProcessing(call, funcName)

	// Check string conversion functions
	v.checkStringConversions(call, funcName)

	// Check file operations
	v.checkFileOperations(call, funcName)
}

// checkAssignment analyzes assignments for validation patterns
func (v *validationVisitor) checkAssignment(assign *ast.AssignStmt) {
	// Check for direct user input assignments without validation
	for i, rhs := range assign.Rhs {
		if i < len(assign.Lhs) {
			v.checkInputAssignment(assign.Lhs[i], rhs)
		}
	}
}

// checkConditional analyzes conditionals for validation patterns
func (v *validationVisitor) checkConditional(ifStmt *ast.IfStmt) {
	// Check for validation patterns in conditionals
	v.checkValidationPatterns(ifStmt.Cond)
}

// checkLiteral analyzes literals for security issues
func (v *validationVisitor) checkLiteral(lit *ast.BasicLit) {
	switch lit.Kind {
	case token.STRING:
		v.checkStringLiteral(lit)
	case token.INT:
		v.checkIntegerLiteral(lit)
	}
}

// checkBinaryExpression analyzes binary expressions for validation patterns
func (v *validationVisitor) checkBinaryExpression(expr *ast.BinaryExpr) {
	// Check for length/boundary checks
	v.checkBoundaryValidation(expr)
}

// Specific check methods

// isHTTPHandler checks if a function is an HTTP handler
func (v *validationVisitor) isHTTPHandler(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) < 1 {
		return false
	}

	// Check for common HTTP handler patterns
	patterns := []string{
		"http.ResponseWriter",
		"*http.Request",
		"*gin.Context",
		"*lift.Context",
		"*fiber.Ctx",
	}

	for _, param := range fn.Type.Params.List {
		paramType := v.getTypeString(param.Type)
		for _, pattern := range patterns {
			if strings.Contains(paramType, pattern) {
				return true
			}
		}
	}

	return false
}

// checkHTTPHandlerValidation checks HTTP handlers for proper input validation
func (v *validationVisitor) checkHTTPHandlerValidation(fn *ast.FuncDecl) {
	// This is a simplified check - in practice, you'd need more sophisticated analysis
	bodyHasValidation := v.functionHasValidationCalls(fn)
	
	if !bodyHasValidation {
		v.addFinding(ValidationFinding{
			Type:        "missing_validation",
			Severity:    "HIGH",
			Function:    fn.Name.Name,
			Issue:       "HTTP handler lacks input validation",
			Description: "HTTP handler does not appear to validate user input",
			Suggestion:  "Add input validation for all user-provided parameters",
			CWE:         "CWE-20",
			OWASP:       "A03:2021 – Injection",
		}, fn.Pos())
	}
}

// checkParameterValidation checks function parameters for validation
func (v *validationVisitor) checkParameterValidation(fn *ast.FuncDecl, param *ast.Field) {
	// Check if string parameters have validation
	if v.isStringType(param.Type) {
		// Look for validation in function body
		if !v.parameterHasValidation(fn, param) {
			for _, name := range param.Names {
				v.addFinding(ValidationFinding{
					Type:        "unvalidated_parameter",
					Severity:    "MEDIUM",
					Function:    fn.Name.Name,
					Issue:       fmt.Sprintf("String parameter '%s' lacks validation", name.Name),
					Description: "Function parameter of string type is not validated",
					Suggestion:  "Add validation for string parameters to prevent injection attacks",
					CWE:         "CWE-20",
				}, param.Pos())
			}
		}
	}
}

// checkDangerousFunctions checks for calls to dangerous functions
func (v *validationVisitor) checkDangerousFunctions(call *ast.CallExpr, funcName string) {
	dangerousFunctions := map[string]string{
		"exec.Command":      "Command injection risk",
		"os.Exec":          "Command injection risk", 
		"syscall.Exec":     "Command injection risk",
		"eval":             "Code injection risk",
		"template.HTML":    "XSS risk with unescaped HTML",
		"template.JS":      "XSS risk with unescaped JavaScript",
		"template.CSS":     "XSS risk with unescaped CSS",
		"sql.Query":        "SQL injection risk",
		"sql.Exec":         "SQL injection risk",
		"fmt.Sprintf":      "Potential format string vulnerability",
	}

	if risk, exists := dangerousFunctions[funcName]; exists {
		v.addFinding(ValidationFinding{
			Type:        "dangerous_function",
			Severity:    v.getDangerousFunctionSeverity(funcName),
			Function:    v.currentFunc,
			Issue:       fmt.Sprintf("Use of dangerous function: %s", funcName),
			Description: risk,
			Suggestion:  v.getDangerousFunctionSuggestion(funcName),
			CWE:         v.getDangerousFunctionCWE(funcName),
			OWASP:       "A03:2021 – Injection",
		}, call.Pos())
	}
}

// checkSQLInjection checks for SQL injection vulnerabilities
func (v *validationVisitor) checkSQLInjection(call *ast.CallExpr, funcName string) {
	sqlFunctions := []string{
		"db.Query", "db.QueryRow", "db.Exec", 
		"tx.Query", "tx.QueryRow", "tx.Exec",
		"sql.Query", "sql.Exec",
		"DB.Query", "DB.Exec",
	}

	for _, sqlFunc := range sqlFunctions {
		if strings.Contains(funcName, sqlFunc) {
			// Check if query uses string concatenation or formatting
			if v.hasStringConcatenationInArgs(call) {
				v.addFinding(ValidationFinding{
					Type:        "sql_injection",
					Severity:    "CRITICAL",
					Function:    v.currentFunc,
					Issue:       "Potential SQL injection vulnerability",
					Description: "SQL query appears to use string concatenation or formatting",
					Suggestion:  "Use parameterized queries with placeholders instead of string concatenation",
					CWE:         "CWE-89",
					OWASP:       "A03:2021 – Injection",
				}, call.Pos())
			}
		}
	}
}

// checkHTTPRequestProcessing checks HTTP request processing for validation
func (v *validationVisitor) checkHTTPRequestProcessing(call *ast.CallExpr, funcName string) {
	httpInputFunctions := map[string]string{
		"FormValue":       "Form parameter should be validated",
		"Query":           "Query parameter should be validated", 
		"PostFormValue":   "POST form value should be validated",
		"Header":          "HTTP header should be validated",
		"Cookie":          "Cookie value should be validated",
		"PathParam":       "Path parameter should be validated",
		"Param":           "URL parameter should be validated",
	}

	for httpFunc, suggestion := range httpInputFunctions {
		if strings.Contains(funcName, httpFunc) {
			v.addFinding(ValidationFinding{
				Type:        "unvalidated_input",
				Severity:    "MEDIUM",
				Function:    v.currentFunc,
				Issue:       fmt.Sprintf("Unvalidated HTTP input from %s", httpFunc),
				Description: "HTTP input is extracted but not validated",
				Suggestion:  suggestion,
				CWE:         "CWE-20",
				OWASP:       "A03:2021 – Injection",
			}, call.Pos())
		}
	}
}

// checkStringConversions checks string conversion functions
func (v *validationVisitor) checkStringConversions(call *ast.CallExpr, funcName string) {
	conversionFunctions := []string{
		"strconv.Atoi", "strconv.ParseInt", "strconv.ParseFloat",
		"strconv.ParseBool", "strconv.ParseUint",
	}

	for _, convFunc := range conversionFunctions {
		if strings.Contains(funcName, convFunc) {
			// Check if error is handled
			if !v.hasErrorHandling(call) {
				v.addFinding(ValidationFinding{
					Type:        "unhandled_conversion_error",
					Severity:    "MEDIUM",
					Function:    v.currentFunc,
					Issue:       "String conversion without error handling",
					Description: "String to number conversion does not handle conversion errors",
					Suggestion:  "Always handle errors from string conversion functions",
					CWE:         "CWE-754",
				}, call.Pos())
			}
		}
	}
}

// checkFileOperations checks file operation functions
func (v *validationVisitor) checkFileOperations(call *ast.CallExpr, funcName string) {
	fileOperations := map[string]string{
		"os.Open":       "File path should be validated to prevent directory traversal",
		"os.OpenFile":   "File path should be validated to prevent directory traversal",
		"ioutil.ReadFile": "File path should be validated to prevent directory traversal",
		"os.ReadFile":   "File path should be validated to prevent directory traversal",
		"filepath.Join": "Path components should be validated",
	}

	for fileFunc, suggestion := range fileOperations {
		if strings.Contains(funcName, fileFunc) {
			v.addFinding(ValidationFinding{
				Type:        "path_traversal_risk",
				Severity:    "HIGH",
				Function:    v.currentFunc,
				Issue:       "Potential path traversal vulnerability",
				Description: fmt.Sprintf("File operation %s may be vulnerable to path traversal", fileFunc),
				Suggestion:  suggestion,
				CWE:         "CWE-22",
				OWASP:       "A01:2021 – Broken Access Control",
			}, call.Pos())
		}
	}
}

// checkInputAssignment checks assignments from user input
func (v *validationVisitor) checkInputAssignment(lhs, rhs ast.Expr) {
	// This is a simplified implementation
	// In practice, you'd need more sophisticated taint analysis
	
	if call, ok := rhs.(*ast.CallExpr); ok {
		funcName := v.getFunctionName(call)
		if v.isUserInputFunction(funcName) {
			v.addFinding(ValidationFinding{
				Type:        "unvalidated_assignment",
				Severity:    "MEDIUM",
				Function:    v.currentFunc,
				Issue:       "Direct assignment from user input without validation",
				Description: "User input is assigned directly without validation",
				Suggestion:  "Validate user input before assignment",
				CWE:         "CWE-20",
			}, call.Pos())
		}
	}
}

// checkValidationPatterns checks for validation patterns in conditions
func (v *validationVisitor) checkValidationPatterns(cond ast.Expr) {
	// This would check for common validation patterns like length checks,
	// null checks, type checks, etc.
}

// checkStringLiteral checks string literals for security issues
func (v *validationVisitor) checkStringLiteral(lit *ast.BasicLit) {
	value := strings.Trim(lit.Value, `"`)

	// Check for hardcoded credentials
	if v.isCredentialPattern(value) {
		v.addFinding(ValidationFinding{
			Type:        "hardcoded_credential",
			Severity:    "CRITICAL",
			Function:    v.currentFunc,
			Issue:       "Hardcoded credential detected",
			Description: "String literal appears to contain hardcoded credentials",
			Suggestion:  "Use environment variables or secure configuration for credentials",
			CWE:         "CWE-798",
			OWASP:       "A07:2021 – Identification and Authentication Failures",
		}, lit.Pos())
	}

	// Check for SQL keywords (potential SQL injection)
	if v.hasSQLKeywords(value) {
		v.addFinding(ValidationFinding{
			Type:        "potential_sql_injection",
			Severity:    "HIGH",
			Function:    v.currentFunc,
			Issue:       "String contains SQL keywords",
			Description: "String literal contains SQL keywords and may be vulnerable to injection",
			Suggestion:  "Use parameterized queries instead of string concatenation",
			CWE:         "CWE-89",
			OWASP:       "A03:2021 – Injection",
		}, lit.Pos())
	}
}

// checkIntegerLiteral checks integer literals for security issues
func (v *validationVisitor) checkIntegerLiteral(lit *ast.BasicLit) {
	// Check for suspicious large numbers that might cause overflows
	if value, err := strconv.ParseInt(lit.Value, 10, 64); err == nil {
		if value > 2147483647 || value < -2147483648 {
			v.addFinding(ValidationFinding{
				Type:        "potential_overflow",
				Severity:    "LOW",
				Function:    v.currentFunc,
				Issue:       "Large integer literal",
				Description: "Integer literal is very large and may cause overflow issues",
				Suggestion:  "Verify that large integers are handled correctly",
				CWE:         "CWE-190",
			}, lit.Pos())
		}
	}
}

// checkBoundaryValidation checks for boundary validation patterns
func (v *validationVisitor) checkBoundaryValidation(expr *ast.BinaryExpr) {
	// Check for length/size validations
	if expr.Op == token.LSS || expr.Op == token.GTR || expr.Op == token.LEQ || expr.Op == token.GEQ {
		// This is a boundary check - good practice
		// We could flag missing boundary checks elsewhere
	}
}

// Helper methods for analysis

func (v *validationVisitor) getFunctionName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			return ident.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	}
	return ""
}

func (v *validationVisitor) getTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + v.getTypeString(t.X)
	case *ast.SelectorExpr:
		return v.getTypeString(t.X) + "." + t.Sel.Name
	}
	return ""
}

func (v *validationVisitor) isStringType(expr ast.Expr) bool {
	return v.getTypeString(expr) == "string"
}

func (v *validationVisitor) functionHasValidationCalls(fn *ast.FuncDecl) bool {
	validationKeywords := []string{
		"Validate", "Check", "Verify", "Sanitize", "Clean",
		"IsValid", "HasValidFormat", "ValidateInput",
	}

	// This is a simplified check - look for validation-related function calls
	if fn.Body == nil {
		return false
	}

	for _, stmt := range fn.Body.List {
		if v.statementContainsValidation(stmt, validationKeywords) {
			return true
		}
	}

	return false
}

func (v *validationVisitor) parameterHasValidation(fn *ast.FuncDecl, param *ast.Field) bool {
	// Simplified check for parameter validation
	return v.functionHasValidationCalls(fn)
}

func (v *validationVisitor) hasStringConcatenationInArgs(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if v.hasConcatenation(arg) {
			return true
		}
	}
	return false
}

func (v *validationVisitor) hasConcatenation(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		return e.Op == token.ADD || v.hasConcatenation(e.X) || v.hasConcatenation(e.Y)
	case *ast.CallExpr:
		funcName := v.getFunctionName(e)
		return strings.Contains(funcName, "Sprintf") || strings.Contains(funcName, "Sprint")
	}
	return false
}

func (v *validationVisitor) hasErrorHandling(call *ast.CallExpr) bool {
	// This is a simplified check - in practice, you'd need more sophisticated analysis
	// to track if the error return value is actually handled
	return true // Assume error handling for now
}

func (v *validationVisitor) isUserInputFunction(funcName string) bool {
	userInputFunctions := []string{
		"FormValue", "Query", "PostFormValue", "Header", 
		"Cookie", "PathParam", "Param", "Body", "GetHeader",
	}

	for _, inputFunc := range userInputFunctions {
		if strings.Contains(funcName, inputFunc) {
			return true
		}
	}
	return false
}

func (v *validationVisitor) isCredentialPattern(value string) bool {
	credentialPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)password\s*=\s*['"]?[a-zA-Z0-9!@#$%^&*()_+=-]{6,}['"]?`),
		regexp.MustCompile(`(?i)secret\s*=\s*['"]?[a-zA-Z0-9+/]{10,}['"]?`),
		regexp.MustCompile(`(?i)token\s*=\s*['"]?[a-zA-Z0-9._-]{20,}['"]?`),
		regexp.MustCompile(`(?i)api[_-]?key\s*=\s*['"]?[a-zA-Z0-9]{16,}['"]?`),
	}

	for _, pattern := range credentialPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func (v *validationVisitor) hasSQLKeywords(value string) bool {
	sqlKeywords := []string{
		"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER",
		"UNION", "WHERE", "FROM", "JOIN", "ORDER BY", "GROUP BY",
	}

	upperValue := strings.ToUpper(value)
	for _, keyword := range sqlKeywords {
		if strings.Contains(upperValue, keyword) {
			return true
		}
	}
	return false
}

func (v *validationVisitor) statementContainsValidation(stmt ast.Stmt, keywords []string) bool {
	// This is a simplified implementation
	// You would need a more sophisticated AST walker here
	return false
}

func (v *validationVisitor) getDangerousFunctionSeverity(funcName string) string {
	criticalFunctions := []string{"exec.Command", "os.Exec", "syscall.Exec", "eval"}
	for _, critical := range criticalFunctions {
		if strings.Contains(funcName, critical) {
			return "CRITICAL"
		}
	}
	return "HIGH"
}

func (v *validationVisitor) getDangerousFunctionSuggestion(funcName string) string {
	suggestions := map[string]string{
		"exec.Command":   "Validate and sanitize all command arguments, use allowlists for commands",
		"template.HTML":  "Use proper template escaping or html/template package",
		"sql.Query":      "Use parameterized queries with database/sql package",
		"fmt.Sprintf":    "Validate format strings and arguments to prevent format string attacks",
	}

	for pattern, suggestion := range suggestions {
		if strings.Contains(funcName, pattern) {
			return suggestion
		}
	}
	return "Review usage and implement proper input validation"
}

func (v *validationVisitor) getDangerousFunctionCWE(funcName string) string {
	cweMap := map[string]string{
		"exec.Command": "CWE-78",
		"sql.Query":    "CWE-89", 
		"template.HTML": "CWE-79",
		"fmt.Sprintf":  "CWE-134",
	}

	for pattern, cwe := range cweMap {
		if strings.Contains(funcName, pattern) {
			return cwe
		}
	}
	return "CWE-20"
}

// addFinding adds a validation finding to the audit results
func (v *validationVisitor) addFinding(finding ValidationFinding, pos token.Pos) {
	position := v.fset.Position(pos)
	finding.File = v.filePath
	finding.Line = position.Line
	finding.Column = position.Column
	finding.Timestamp = time.Now()

	v.auditor.findings = append(v.auditor.findings, finding)
}

// Helper methods for the auditor

// shouldExclude checks if a path should be excluded from the audit
func (a *InputValidationAuditor) shouldExclude(path string) bool {
	for _, excludePath := range a.excludePaths {
		if strings.Contains(path, excludePath) {
			return true
		}
	}
	return false
}

// calculateSummary calculates summary statistics for the audit
func (a *InputValidationAuditor) calculateSummary() {
	a.summary.TotalFindings = len(a.findings)

	for _, finding := range a.findings {
		a.summary.BySeverity[finding.Severity]++
		a.summary.ByType[finding.Type]++

		if finding.Severity == "CRITICAL" {
			a.summary.CriticalIssues++
		} else if finding.Severity == "HIGH" {
			a.summary.HighPriorityIssues++
		}
	}
}

// RunInputValidationAudit is a convenience function to run a complete audit
func RunInputValidationAudit(logger *zap.Logger, rootPath string) (*AuditReport, error) {
	auditor := NewInputValidationAuditor(logger, rootPath)
	return auditor.AuditDirectory(context.Background())
}