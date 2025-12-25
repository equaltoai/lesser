package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const packagePathLift = "github.com/equaltoai/lesser/cmd/api/lift"

type handlerPayloadInfo struct {
	Request      types.Type
	Response     types.Type
	SuccessCodes []int
	PrimaryCode  int
	QueryParams  []string
}

func inferLiftHandlerPayloads(pkg *packages.Package) (map[string]handlerPayloadInfo, error) {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil, nil
	}

	payloads := map[string]handlerPayloadInfo{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn == nil || fn.Name == nil || fn.Body == nil || fn.Recv == nil {
				continue
			}
			if !isHandlerReceiver(fn.Recv) {
				continue
			}

			payloads[fn.Name.Name] = analyzeHandlerPayloads(fn, pkg.TypesInfo)
		}
	}

	return payloads, nil
}

func analyzeHandlerPayloads(fn *ast.FuncDecl, info *types.Info) handlerPayloadInfo {
	var request types.Type
	var response types.Type
	requestScore := 0
	responseScore := 0
	successCodes := map[int]int{}
	queryParams := map[string]struct{}{}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call == nil {
			return true
		}

		if t, score := requestPayloadFromCall(call, info); score > requestScore {
			request = t
			requestScore = score
		}
		if t, score := responsePayloadFromCall(call, info); score > responseScore {
			response = t
			responseScore = score
		}

		if code, ok := successStatusFromCall(call, info); ok {
			successCodes[code]++
		}

		if name, ok := queryParamNameFromCall(call); ok {
			queryParams[name] = struct{}{}
		}

		return true
	})

	codes := sortedSuccessCodes(successCodes)

	return handlerPayloadInfo{
		Request:      request,
		Response:     response,
		SuccessCodes: codes,
		PrimaryCode:  choosePrimarySuccessCode(codes),
		QueryParams:  sortedQueryParams(queryParams),
	}
}

func queryParamNameFromCall(call *ast.CallExpr) (string, bool) {
	if call == nil {
		return "", false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return "", false
	}
	switch sel.Sel.Name {
	case "Query", "QueryParam":
	default:
		return "", false
	}
	if len(call.Args) < 1 {
		return "", false
	}
	name, ok := evalStringLiteral(call.Args[0])
	if !ok {
		return "", false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	return name, true
}

func sortedQueryParams(params map[string]struct{}) []string {
	if len(params) == 0 {
		return nil
	}
	out := make([]string, 0, len(params))
	for name := range params {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func successStatusFromCall(call *ast.CallExpr, info *types.Info) (int, bool) {
	if call == nil {
		return 0, false
	}

	if isHandlerRespondOKCall(call) {
		return 200, true
	}
	if isHandlerRespondCreatedCall(call) {
		return 201, true
	}
	if isLiftJSONCall(call, info) {
		return 200, true
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return 0, false
	}
	if sel.Sel.Name != "Status" {
		return 0, false
	}
	if len(call.Args) < 1 {
		return 0, false
	}

	code, ok := intValue(call.Args[0], info)
	if !ok {
		return 0, false
	}
	if code < 200 || code >= 400 {
		return 0, false
	}
	return code, true
}

func intValue(expr ast.Expr, info *types.Info) (int, bool) {
	if expr == nil {
		return 0, false
	}
	if info != nil {
		if tv, ok := info.Types[expr]; ok && tv.Value != nil {
			if v, ok := constant.Int64Val(tv.Value); ok {
				return int(v), true
			}
		}
	}

	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	value, err := strconv.Atoi(lit.Value)
	if err != nil {
		return 0, false
	}
	return value, true
}

func sortedSuccessCodes(counts map[int]int) []int {
	if len(counts) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	var codes []int
	for code := range counts {
		if code < 200 || code >= 400 {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}

func choosePrimarySuccessCode(codes []int) int {
	if len(codes) == 0 {
		return 0
	}

	for _, preferred := range []int{201, 202, 204, 200, 302} {
		for _, code := range codes {
			if code == preferred {
				return code
			}
		}
	}
	return codes[0]
}

func requestPayloadFromCall(call *ast.CallExpr, info *types.Info) (types.Type, int) {
	if call == nil || info == nil {
		return nil, 0
	}

	if isJSONUnmarshalCall(call, info) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
		return t, scorePayloadType(t)
	}

	if isHandlerParseRequestBodyCall(call) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
		return t, scorePayloadType(t)
	}

	if isLiftParseRequestCall(call, info) && len(call.Args) >= 1 {
		t := payloadTypeFromExpr(call.Args[0], info)
		return t, scorePayloadType(t)
	}

	if idx, ok := commonParseTargetArgIndex(call, info); ok && len(call.Args) > idx {
		t := payloadTypeFromExpr(call.Args[idx], info)
		return t, scorePayloadType(t)
	}

	return nil, 0
}

func responsePayloadFromCall(call *ast.CallExpr, info *types.Info) (types.Type, int) {
	if call == nil || info == nil {
		return nil, 0
	}

	if isHandlerRespondOKCall(call) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
		return t, scorePayloadType(t)
	}
	if isHandlerRespondCreatedCall(call) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
		return t, scorePayloadType(t)
	}

	if isLiftJSONCall(call, info) && len(call.Args) >= 1 {
		t := payloadTypeFromExpr(call.Args[0], info)
		return t, scorePayloadType(t)
	}

	return nil, 0
}

func payloadTypeFromExpr(expr ast.Expr, info *types.Info) types.Type {
	if expr == nil || info == nil {
		return nil
	}
	tv, ok := info.Types[expr]
	if !ok {
		return nil
	}

	t := types.Unalias(tv.Type)
	for {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = types.Unalias(ptr.Elem())
	}
	return t
}

func isJSONUnmarshalCall(call *ast.CallExpr, info *types.Info) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn == nil || fn.Pkg() == nil {
		return false
	}
	return fn.Pkg().Path() == "encoding/json" && fn.Name() == "Unmarshal"
}

func isHandlerParseRequestBodyCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "h" && sel.Sel.Name == "parseRequestBody"
}

func isHandlerRespondOKCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "h" && sel.Sel.Name == "respondOK"
}

func isHandlerRespondCreatedCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "h" && sel.Sel.Name == "respondCreated"
}

func isLiftParseRequestCall(call *ast.CallExpr, info *types.Info) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	if sel.Sel.Name != "ParseRequest" {
		return false
	}
	if info == nil {
		return true
	}
	if selObj, ok := info.Selections[sel]; ok && selObj != nil && selObj.Obj() != nil {
		if fn, ok := selObj.Obj().(*types.Func); ok && fn.Name() == "ParseRequest" {
			return true
		}
	}
	return true
}

func commonParseTargetArgIndex(call *ast.CallExpr, info *types.Info) (int, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return 0, false
	}

	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn == nil || fn.Pkg() == nil {
		return 0, false
	}
	if fn.Pkg().Path() != "github.com/equaltoai/lesser/pkg/common" {
		return 0, false
	}

	switch fn.Name() {
	case "ParseRequestWithFallback",
		"ParseRequestWithValidation",
		"ParseRequestWithCustomError",
		"ParseRequestBodyWithValidation",
		"ParseRequestBody":
		return 1, true
	default:
		return 0, false
	}
}

func isLiftJSONCall(call *ast.CallExpr, info *types.Info) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	if sel.Sel.Name != "JSON" {
		return false
	}

	obj, ok := info.Selections[sel]
	if ok && obj != nil && obj.Obj() != nil {
		if fn, ok := obj.Obj().(*types.Func); ok && fn.Pkg() != nil && fn.Pkg().Path() == packagePathLift {
			return true
		}
	}

	return true
}

func scorePayloadType(t types.Type) int {
	if t == nil {
		return 0
	}

	t = types.Unalias(t)
	switch tt := t.(type) {
	case *types.Named:
		if tt.Obj() != nil && tt.Obj().Pkg() != nil {
			switch tt.Obj().Pkg().Path() {
			case packagePathModels:
				return 100
			case packagePathAuth:
				return 80
			}
		}
		return 50
	case *types.Slice, *types.Array:
		return 60
	case *types.Map:
		return 20
	case *types.Struct:
		return 20
	default:
		return 10
	}
}
