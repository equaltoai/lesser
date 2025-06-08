package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"sort"
	"strings"
)

func main() {
	// Parse the interface file
	interfaceMethods, err := parseInterface("pkg/storage/interface.go")
	if err != nil {
		log.Fatalf("Failed to parse interface: %v", err)
	}

	// Parse the mock file
	mockMethods, err := parseMockMethods("internal/testutil/mocks/storage.go")
	if err != nil {
		log.Fatalf("Failed to parse mock file: %v", err)
	}

	// Find missing methods
	missing := findMissingMethods(interfaceMethods, mockMethods)

	fmt.Printf("Found %d methods in interface\n", len(interfaceMethods))
	fmt.Printf("Found %d methods already implemented\n", len(mockMethods))
	fmt.Printf("Need to generate %d methods\n", len(missing))

	// Generate mock implementations
	generated := generateMockMethods(missing)

	// Write to file
	err = os.WriteFile("generated_mocks.go", []byte(generated), 0644)
	if err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}

	fmt.Println("Generated mock methods written to generated_mocks.go")
}

type Method struct {
	Name     string
	Params   []Param
	Results  []Result
	Receiver string
}

type Param struct {
	Name string
	Type string
}

type Result struct {
	Type string
}

func parseInterface(filename string) ([]Method, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var methods []Method

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for the Storage interface
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		if typeSpec.Name.Name != "Storage" {
			return true
		}

		iface, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}

		// Extract methods from interface
		for _, method := range iface.Methods.List {
			if len(method.Names) == 0 {
				continue // embedded interface
			}

			funcType, ok := method.Type.(*ast.FuncType)
			if !ok {
				continue
			}

			m := Method{
				Name: method.Names[0].Name,
			}

			// Parse parameters
			if funcType.Params != nil {
				for _, param := range funcType.Params.List {
					paramType := exprToString(param.Type)
					if len(param.Names) == 0 {
						// Unnamed parameter
						m.Params = append(m.Params, Param{Type: paramType})
					} else {
						for _, name := range param.Names {
							m.Params = append(m.Params, Param{
								Name: name.Name,
								Type: paramType,
							})
						}
					}
				}
			}

			// Parse results
			if funcType.Results != nil {
				for _, result := range funcType.Results.List {
					resultType := exprToString(result.Type)
					m.Results = append(m.Results, Result{Type: resultType})
				}
			}

			methods = append(methods, m)
		}

		return false
	})

	return methods, nil
}

func parseMockMethods(filename string) (map[string]bool, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	methods := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Check if it's a method on MockStorage
		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			return true
		}

		recvType := exprToString(funcDecl.Recv.List[0].Type)
		if strings.Contains(recvType, "MockStorage") {
			methods[funcDecl.Name.Name] = true
		}

		return true
	})

	return methods, nil
}

func findMissingMethods(all []Method, implemented map[string]bool) []Method {
	var missing []Method
	for _, method := range all {
		if !implemented[method.Name] {
			missing = append(missing, method)
		}
	}

	// Sort for consistent output
	sort.Slice(missing, func(i, j int) bool {
		return missing[i].Name < missing[j].Name
	})

	return missing
}

func generateMockMethods(methods []Method) string {
	var buf bytes.Buffer

	buf.WriteString("// Generated mock methods for storage.Storage interface\n\n")

	for _, method := range methods {
		buf.WriteString(generateMockMethod(method))
		buf.WriteString("\n\n")
	}

	return buf.String()
}

func generateMockMethod(m Method) string {
	var buf bytes.Buffer

	// Write comment
	buf.WriteString(fmt.Sprintf("// %s mocks the %s method\n", m.Name, m.Name))

	// Write method signature
	buf.WriteString(fmt.Sprintf("func (m *MockStorage) %s(", m.Name))

	// Write parameters
	for i, param := range m.Params {
		if i > 0 {
			buf.WriteString(", ")
		}
		if param.Name != "" {
			buf.WriteString(fmt.Sprintf("%s %s", param.Name, param.Type))
		} else {
			buf.WriteString(param.Type)
		}
	}
	buf.WriteString(") ")

	// Write return types
	if len(m.Results) > 0 {
		if len(m.Results) == 1 {
			buf.WriteString(m.Results[0].Type)
		} else {
			buf.WriteString("(")
			for i, result := range m.Results {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(result.Type)
			}
			buf.WriteString(")")
		}
	}

	buf.WriteString(" {\n")

	// Write method body
	buf.WriteString("\targs := m.Called(")
	for i, param := range m.Params {
		if i > 0 {
			buf.WriteString(", ")
		}
		if param.Name != "" {
			buf.WriteString(param.Name)
		} else {
			buf.WriteString(fmt.Sprintf("arg%d", i))
		}
	}
	buf.WriteString(")\n")

	// Generate return statement
	if len(m.Results) > 0 {
		buf.WriteString(generateReturnStatement(m.Results))
	}

	buf.WriteString("}")

	return buf.String()
}

func generateReturnStatement(results []Result) string {
	var parts []string

	for i, result := range results {
		switch {
		case strings.Contains(result.Type, "error"):
			parts = append(parts, fmt.Sprintf("args.Error(%d)", i))
		case strings.Contains(result.Type, "string"):
			parts = append(parts, fmt.Sprintf("args.String(%d)", i))
		case strings.Contains(result.Type, "int"):
			parts = append(parts, fmt.Sprintf("args.Int(%d)", i))
		case strings.Contains(result.Type, "int64"):
			parts = append(parts, fmt.Sprintf("args.Get(%d).(int64)", i))
		case strings.Contains(result.Type, "float"):
			parts = append(parts, fmt.Sprintf("args.Get(%d).(float64)", i))
		case strings.Contains(result.Type, "bool"):
			parts = append(parts, fmt.Sprintf("args.Bool(%d)", i))
		case strings.Contains(result.Type, "time.Time"):
			parts = append(parts, fmt.Sprintf("args.Get(%d).(time.Time)", i))
		case strings.Contains(result.Type, "time.Duration"):
			parts = append(parts, fmt.Sprintf("args.Get(%d).(time.Duration)", i))
		case strings.HasPrefix(result.Type, "*"):
			// Pointer type - need nil check
			return generatePointerReturn(results, i)
		case strings.HasPrefix(result.Type, "[]"):
			// Slice type - need nil check
			return generateSliceReturn(results, i)
		case strings.HasPrefix(result.Type, "map"):
			// Map type - need nil check
			return generateMapReturn(results, i)
		default:
			// Generic interface or struct
			parts = append(parts, fmt.Sprintf("args.Get(%d).(%s)", i, result.Type))
		}
	}

	return "\treturn " + strings.Join(parts, ", ") + "\n"
}

func generatePointerReturn(results []Result, pointerIndex int) string {
	var buf bytes.Buffer

	resultType := results[pointerIndex].Type

	buf.WriteString(fmt.Sprintf("\tif args.Get(%d) == nil {\n", pointerIndex))

	// Build nil return
	var nilParts []string
	for i, result := range results {
		if i == pointerIndex {
			nilParts = append(nilParts, "nil")
		} else if strings.Contains(result.Type, "error") {
			nilParts = append(nilParts, fmt.Sprintf("args.Error(%d)", i))
		} else {
			nilParts = append(nilParts, "nil")
		}
	}
	buf.WriteString("\t\treturn " + strings.Join(nilParts, ", ") + "\n")
	buf.WriteString("\t}\n")

	// Build normal return
	var parts []string
	for i, result := range results {
		if i == pointerIndex {
			parts = append(parts, fmt.Sprintf("args.Get(%d).(%s)", i, resultType))
		} else if strings.Contains(result.Type, "error") {
			parts = append(parts, fmt.Sprintf("args.Error(%d)", i))
		} else if strings.Contains(result.Type, "string") {
			parts = append(parts, fmt.Sprintf("args.String(%d)", i))
		} else {
			parts = append(parts, fmt.Sprintf("args.Get(%d).(%s)", i, result.Type))
		}
	}
	buf.WriteString("\treturn " + strings.Join(parts, ", ") + "\n")

	return buf.String()
}

func generateSliceReturn(results []Result, sliceIndex int) string {
	var buf bytes.Buffer

	resultType := results[sliceIndex].Type

	buf.WriteString(fmt.Sprintf("\tif args.Get(%d) == nil {\n", sliceIndex))

	// Build nil return
	var nilParts []string
	for i, result := range results {
		if i == sliceIndex {
			nilParts = append(nilParts, "nil")
		} else if strings.Contains(result.Type, "error") {
			nilParts = append(nilParts, fmt.Sprintf("args.Error(%d)", i))
		} else if strings.Contains(result.Type, "string") {
			nilParts = append(nilParts, fmt.Sprintf("args.String(%d)", i))
		} else {
			nilParts = append(nilParts, "nil")
		}
	}
	buf.WriteString("\t\treturn " + strings.Join(nilParts, ", ") + "\n")
	buf.WriteString("\t}\n")

	// Build normal return
	var parts []string
	for i, result := range results {
		if i == sliceIndex {
			parts = append(parts, fmt.Sprintf("args.Get(%d).(%s)", i, resultType))
		} else if strings.Contains(result.Type, "error") {
			parts = append(parts, fmt.Sprintf("args.Error(%d)", i))
		} else if strings.Contains(result.Type, "string") {
			parts = append(parts, fmt.Sprintf("args.String(%d)", i))
		} else {
			parts = append(parts, fmt.Sprintf("args.Get(%d).(%s)", i, result.Type))
		}
	}
	buf.WriteString("\treturn " + strings.Join(parts, ", ") + "\n")

	return buf.String()
}

func generateMapReturn(results []Result, mapIndex int) string {
	// Same as slice return
	return generateSliceReturn(results, mapIndex)
}

func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + exprToString(e.Elt)
		}
		return "[" + exprToString(e.Len) + "]" + exprToString(e.Elt)
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.MapType:
		return "map[" + exprToString(e.Key) + "]" + exprToString(e.Value)
	case *ast.FuncType:
		return "func" // Simplified for now
	case *ast.Ellipsis:
		return "..." + exprToString(e.Elt)
	default:
		return fmt.Sprintf("%T", expr)
	}
}
