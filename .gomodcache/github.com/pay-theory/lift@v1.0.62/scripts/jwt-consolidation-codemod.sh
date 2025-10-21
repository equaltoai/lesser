#!/usr/bin/env bash
set -euo pipefail

# Codemod to migrate from lift.WithJWTAuth to middleware.JWTAuth
# Usage: scripts/jwt-consolidation-codemod.sh [path]

ROOT="${1:-.}"

echo "[codemod] Scanning Go files under $ROOT"

# Replace app construction patterns with app.Use(middleware.JWTAuth(...)) guidance.
# This codemod performs conservative textual changes and prints hints for manual review.

rg -n "WithJWTAuth\(|WithSimpleJWTAuth\(" "$ROOT" || true

echo "[codemod] Suggested replacements (manual):"
echo "- Replace: lift.WithJWTAuth(lift.JWTAuthConfig{...})"
echo "  With:    app.Use(middleware.JWTAuth(middleware.JWTConfig{...}))"
echo "- Replace: lift.WithSimpleJWTAuth(secret)"
echo "  With:    app.Use(middleware.JWTAuth(middleware.JWTConfig{Secret: secret}))"

echo "[codemod] Adding import if needed: github.com/pay-theory/lift/pkg/middleware"
echo "[codemod] Done. Please review diffs and run 'go test ./...'"

