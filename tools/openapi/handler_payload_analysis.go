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

const packagePathLift = "github.com/equaltoai/lesser/cmd/api/handlers"
const packagePathAppTheory = "github.com/theory-cloud/apptheory/runtime"

type handlerPayloadInfo struct {
	Request      types.Type
	Response     types.Type
	SuccessCodes []int
	PrimaryCode  int
	QueryParams  []string
	Scopes       []string
}

type payloadAnalysis struct {
	Base    handlerPayloadInfo
	Callees map[string]struct{}
}

func inferLiftHandlerPayloads(pkg *packages.Package) (map[string]handlerPayloadInfo, error) {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil, nil
	}

	analyses := collectHandlerPayloadAnalyses(pkg)
	payloads := initHandlerPayloads(analyses)
	propagateHandlerPayloads(analyses, payloads)
	return payloads, nil
}

func collectHandlerPayloadAnalyses(pkg *packages.Package) map[string]payloadAnalysis {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil
	}

	analyses := map[string]payloadAnalysis{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn == nil || fn.Name == nil || fn.Body == nil || fn.Recv == nil {
				continue
			}
			if !isHandlerReceiver(fn.Recv) {
				continue
			}

			name := strings.TrimSpace(fn.Name.Name)
			if name == "" {
				continue
			}
			analyses[name] = payloadAnalysis{
				Base:    analyzeHandlerPayloads(fn, pkg.TypesInfo),
				Callees: handlerReceiverCallees(fn),
			}
		}
	}

	return analyses
}

func initHandlerPayloads(analyses map[string]payloadAnalysis) map[string]handlerPayloadInfo {
	if len(analyses) == 0 {
		return nil
	}

	payloads := make(map[string]handlerPayloadInfo, len(analyses))
	for name, analysis := range analyses {
		payloads[name] = analysis.Base
	}
	return payloads
}

func propagateHandlerPayloads(analyses map[string]payloadAnalysis, payloads map[string]handlerPayloadInfo) {
	if len(analyses) == 0 || len(payloads) == 0 {
		return
	}

	for {
		changed := false
		for name, analysis := range analyses {
			current := payloads[name]
			next := mergePayloadInfo(current, analysis.Callees, payloads)
			if !payloadInfosEqual(current, next) {
				payloads[name] = next
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func mergePayloadInfo(current handlerPayloadInfo, callees map[string]struct{}, payloads map[string]handlerPayloadInfo) handlerPayloadInfo {
	next := current

	next.Request = bestPayloadType(current.Request, callees, payloads, func(info handlerPayloadInfo) types.Type { return info.Request })
	next.Response = bestPayloadType(current.Response, callees, payloads, func(info handlerPayloadInfo) types.Type { return info.Response })

	next.QueryParams = mergeQueryParams(current.QueryParams, callees, payloads)
	next.SuccessCodes = mergeSuccessCodes(current.SuccessCodes, callees, payloads)
	next.PrimaryCode = choosePrimarySuccessCode(next.SuccessCodes)
	next.Scopes = mergeScopes(current.Scopes, callees, payloads)

	return next
}

func bestPayloadType(
	current types.Type,
	callees map[string]struct{},
	payloads map[string]handlerPayloadInfo,
	get func(handlerPayloadInfo) types.Type,
) types.Type {
	best := current
	bestScore := scorePayloadType(best)

	for callee := range callees {
		info, ok := payloads[callee]
		if !ok {
			continue
		}
		t := get(info)
		if score := scorePayloadType(t); score > bestScore {
			best = t
			bestScore = score
		}
	}

	return best
}

func mergeQueryParams(current []string, callees map[string]struct{}, payloads map[string]handlerPayloadInfo) []string {
	params := map[string]struct{}{}
	for _, q := range current {
		params[q] = struct{}{}
	}
	for callee := range callees {
		info, ok := payloads[callee]
		if !ok {
			continue
		}
		for _, q := range info.QueryParams {
			params[q] = struct{}{}
		}
	}
	return sortedQueryParams(params)
}

func mergeSuccessCodes(current []int, callees map[string]struct{}, payloads map[string]handlerPayloadInfo) []int {
	codes := map[int]struct{}{}
	for _, code := range current {
		codes[code] = struct{}{}
	}
	for callee := range callees {
		info, ok := payloads[callee]
		if !ok {
			continue
		}
		for _, code := range info.SuccessCodes {
			codes[code] = struct{}{}
		}
	}

	out := make([]int, 0, len(codes))
	for code := range codes {
		out = append(out, code)
	}
	sort.Ints(out)
	return out
}

func mergeScopes(current []string, callees map[string]struct{}, payloads map[string]handlerPayloadInfo) []string {
	scopes := map[string]struct{}{}
	for _, scope := range current {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		scopes[scope] = struct{}{}
	}
	for callee := range callees {
		info, ok := payloads[callee]
		if !ok {
			continue
		}
		for _, scope := range info.Scopes {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			scopes[scope] = struct{}{}
		}
	}

	return sortedScopes(scopes)
}

func payloadInfosEqual(a, b handlerPayloadInfo) bool {
	if a.Request == nil || b.Request == nil {
		if a.Request != b.Request {
			return false
		}
	} else if !types.Identical(a.Request, b.Request) {
		return false
	}

	if a.Response == nil || b.Response == nil {
		if a.Response != b.Response {
			return false
		}
	} else if !types.Identical(a.Response, b.Response) {
		return false
	}
	if len(a.SuccessCodes) != len(b.SuccessCodes) {
		return false
	}
	for i := range a.SuccessCodes {
		if a.SuccessCodes[i] != b.SuccessCodes[i] {
			return false
		}
	}
	if a.PrimaryCode != b.PrimaryCode {
		return false
	}
	if len(a.QueryParams) != len(b.QueryParams) {
		return false
	}
	for i := range a.QueryParams {
		if a.QueryParams[i] != b.QueryParams[i] {
			return false
		}
	}
	if len(a.Scopes) != len(b.Scopes) {
		return false
	}
	for i := range a.Scopes {
		if a.Scopes[i] != b.Scopes[i] {
			return false
		}
	}
	return true
}

func handlerReceiverCallees(fn *ast.FuncDecl) map[string]struct{} {
	if fn == nil || fn.Body == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return nil
	}

	recvName := ""
	if len(fn.Recv.List[0].Names) > 0 {
		recvName = strings.TrimSpace(fn.Recv.List[0].Names[0].Name)
	}
	if recvName == "" {
		recvName = "h"
	}

	callees := map[string]struct{}{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call == nil {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel == nil || sel.Sel == nil {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok || strings.TrimSpace(recv.Name) != recvName {
			return true
		}
		callee := strings.TrimSpace(sel.Sel.Name)
		if callee != "" {
			callees[callee] = struct{}{}
		}
		return true
	})

	return callees
}

func analyzeHandlerPayloads(fn *ast.FuncDecl, info *types.Info) handlerPayloadInfo {
	var request types.Type
	var response types.Type
	requestScore := 0
	responseScore := 0
	successCodes := map[int]int{}
	queryParams := map[string]struct{}{}
	scopes := map[string]struct{}{}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call == nil {
			return true
		}

		for _, scope := range scopeNamesFromCall(call, info) {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			scopes[scope] = struct{}{}
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
		Scopes:       sortedScopes(scopes),
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

func scopeNamesFromCall(call *ast.CallExpr, info *types.Info) []string {
	if call == nil || info == nil {
		return nil
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return nil
	}

	switch sel.Sel.Name {
	case "authenticateWithScope":
		if len(call.Args) < 2 {
			return nil
		}
		scope, ok := stringValue(call.Args[1], info)
		if !ok {
			return nil
		}
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil
		}
		return []string{scope}
	case "HasScope":
		if len(call.Args) < 1 {
			return nil
		}
		scope, ok := stringValue(call.Args[0], info)
		if !ok {
			return nil
		}
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil
		}
		return []string{scope}
	default:
		return nil
	}
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

func sortedScopes(scopes map[string]struct{}) []string {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]string, 0, len(scopes))
	for scope := range scopes {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func successStatusFromCall(call *ast.CallExpr, info *types.Info) (int, bool) {
	if call == nil {
		return 0, false
	}

	if isLiftHelperCall(call, info, "noContent") {
		return 204, true
	}
	if isLiftHelperCall(call, info, "okJSON") {
		return 200, true
	}
	if isLiftHelperCall(call, info, "createdJSON") {
		return 201, true
	}

	if isHandlerRespondOKCall(call) {
		return 200, true
	}
	if isHandlerRespondCreatedCall(call) {
		return 201, true
	}
	if isAppTheoryJSONCall(call, info) && len(call.Args) >= 1 {
		code, ok := intValue(call.Args[0], info)
		if !ok {
			return 0, false
		}
		if code < 200 || code >= 400 {
			return 0, false
		}
		return code, true
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

func stringValue(expr ast.Expr, info *types.Info) (string, bool) {
	if expr == nil {
		return "", false
	}

	if v, ok := evalStringLiteral(expr); ok {
		return v, true
	}

	if info != nil {
		if tv, ok := info.Types[expr]; ok && tv.Value != nil {
			if tv.Value.Kind() == constant.String {
				return constant.StringVal(tv.Value), true
			}
		}
	}

	return "", false
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

	if isHandlerParseEmojiRequestCall(call) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
		return t, scorePayloadType(t)
	}

	if isHandlerParseScheduledStatusRequestCall(call) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
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

	if isLiftHelperCall(call, info, "okJSON") && len(call.Args) >= 1 {
		t := payloadTypeFromExpr(call.Args[0], info)
		return t, scorePayloadType(t)
	}
	if isLiftHelperCall(call, info, "createdJSON") && len(call.Args) >= 1 {
		t := payloadTypeFromExpr(call.Args[0], info)
		return t, scorePayloadType(t)
	}

	if isHandlerRespondOKCall(call) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
		return t, scorePayloadType(t)
	}
	if isHandlerRespondCreatedCall(call) && len(call.Args) >= 2 {
		t := payloadTypeFromExpr(call.Args[1], info)
		return t, scorePayloadType(t)
	}

	if isAppTheoryJSONCall(call, info) && len(call.Args) >= 2 {
		code, ok := intValue(call.Args[0], info)
		if !ok || code < 200 || code >= 400 {
			return nil, 0
		}
		t := payloadTypeFromExpr(call.Args[1], info)
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
	return ok && sel.Sel.Name == "parseRequestBody" && recv.Name != ""
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

func isHandlerParseEmojiRequestCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "h" && sel.Sel.Name == "parseEmojiRequest"
}

func isHandlerParseScheduledStatusRequestCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "h" && sel.Sel.Name == "parseScheduledStatusRequest"
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
	// Deprecated: legacy matcher. Prefer isAppTheoryJSONCall.
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	if sel.Sel.Name != "JSON" {
		return false
	}

	return isAppTheoryJSONCall(call, info)
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

func isLiftHelperCall(call *ast.CallExpr, info *types.Info, name string) bool {
	if call == nil {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident == nil {
		return false
	}
	if strings.TrimSpace(ident.Name) != name {
		return false
	}
	if info == nil {
		return true
	}
	fn, ok := info.Uses[ident].(*types.Func)
	if !ok || fn == nil || fn.Pkg() == nil {
		return false
	}
	return fn.Pkg().Path() == packagePathLift && strings.TrimSpace(fn.Name()) == name
}

func isAppTheoryJSONCall(call *ast.CallExpr, info *types.Info) bool {
	if call == nil || info == nil {
		return false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return false
	}
	if strings.TrimSpace(sel.Sel.Name) != "JSON" {
		return false
	}

	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn == nil || fn.Pkg() == nil {
		return false
	}
	return fn.Pkg().Path() == packagePathAppTheory && strings.TrimSpace(fn.Name()) == "JSON"
}
