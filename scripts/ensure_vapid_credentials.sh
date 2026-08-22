#!/bin/bash

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <environment> [domain]" >&2
  echo "Example: $0 development dev.lesser.host" >&2
  exit 1
fi

ENVIRONMENT="$1"
DOMAIN_OVERRIDE="${2:-}"

resolve_stage() {
  case "$1" in
    development|dev) echo "dev" ;;
    staging|test) echo "staging" ;;
    production|prod|live) echo "live" ;;
    *) echo "$1" ;;
  esac
}

STAGE="$(resolve_stage "$ENVIRONMENT")"
SECRET_NAME="lesser/vapid-key-${STAGE}"

DEFAULT_DOMAIN() {
  case "$1" in
    live) echo "lesser.host" ;;
    *) echo "$1.lesser.host" ;;
  esac
}

if [ -n "$DOMAIN_OVERRIDE" ]; then
  BASE_DOMAIN="$DOMAIN_OVERRIDE"
else
  BASE_DOMAIN="$(DEFAULT_DOMAIN "$STAGE")"
fi

SUBJECT_DEFAULT="mailto:push@${BASE_DOMAIN}"
SUBJECT="${VAPID_SUBJECT_OVERRIDE:-$SUBJECT_DEFAULT}"

A_REGION="${AWS_REGION:-us-east-1}"
AWS_ARGS=("--region" "$A_REGION")
if [ -n "${AWS_PROFILE:-}" ]; then
  AWS_ARGS+=(--profile "$AWS_PROFILE")
fi

aws_cli() {
  aws "${AWS_ARGS[@]}" "$@"
}

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

PRIVATE_KEY_PATH="$TMP_DIR/private.pem"
PUBLIC_DER_PATH="$TMP_DIR/public.der"

generate_vapid_pair() {
  openssl ecparam -name prime256v1 -genkey -noout -out "$PRIVATE_KEY_PATH" >/dev/null 2>&1
  openssl ec -in "$PRIVATE_KEY_PATH" -pubout -outform DER -out "$PUBLIC_DER_PATH" >/dev/null 2>&1
python3 - <<'PY' "$PUBLIC_DER_PATH"
import base64
import sys

der = open(sys.argv[1], "rb").read()
if len(der) < 65:
    raise SystemExit("DER public key too short")
pub = der[-65:]
if pub[0] != 4:
    raise SystemExit("unexpected public key prefix (expected uncompressed SEC1)")
print(base64.urlsafe_b64encode(pub).rstrip(b"=").decode())
PY
}

ensure_secret() {
  local secret_arn existing_secret public_key created_at updated_at private_key subject now resolved_subject
  now="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  resolved_subject="$SUBJECT"

  if secret_arn=$(aws_cli secretsmanager describe-secret --secret-id "$SECRET_NAME" --query 'ARN' --output text 2>/dev/null); then
    existing_secret=$(aws_cli secretsmanager get-secret-value --secret-id "$SECRET_NAME" --query 'SecretString' --output text 2>/dev/null || echo '')
    if [ -n "$existing_secret" ] && [ "$existing_secret" != "None" ]; then
      public_key=$(printf '%s' "$existing_secret" | jq -r '.public_key // empty')
      private_key=$(printf '%s' "$existing_secret" | jq -r '.private_key // empty')
      subject=$(printf '%s' "$existing_secret" | jq -r '.subject // empty')
      created_at=$(printf '%s' "$existing_secret" | jq -r '.created_at // empty')
      updated_at=$(printf '%s' "$existing_secret" | jq -r '.updated_at // empty')
      if [ -n "$subject" ] && [ "$subject" != "null" ]; then
        resolved_subject="$subject"
      fi
    fi

    if [ -z "${public_key:-}" ] || [ -z "${private_key:-}" ]; then
      public_key="$(generate_vapid_pair)"
      jq -n \
        --rawfile priv "$PRIVATE_KEY_PATH" \
        --arg pub "$public_key" \
        --arg sub "$resolved_subject" \
        --arg created "$now" \
        --arg updated "$now" \
        '{public_key:$pub, private_key:$priv, subject:$sub, created_at:$created, updated_at:$updated}' \
        >"$TMP_DIR/secret.json"
      aws_cli secretsmanager put-secret-value \
        --secret-id "$SECRET_NAME" \
        --secret-string "file://$TMP_DIR/secret.json" >/dev/null
    else
      if [ -z "$subject" ] || [ "$subject" = "null" ]; then
        resolved_subject="$SUBJECT"
      fi
      jq -n \
        --arg pub "$public_key" \
        --arg sub "$resolved_subject" \
        --arg created "${created_at:-$now}" \
        --arg updated "$now" \
        --argjson priv "$(printf '%s' "$existing_secret" | jq '.private_key')" \
        '{
          public_key: $pub,
          private_key: ($priv | tostring),
          subject: $sub,
          created_at: $created,
          updated_at: $updated
        }' >"$TMP_DIR/secret.json"
      aws_cli secretsmanager put-secret-value \
        --secret-id "$SECRET_NAME" \
        --secret-string "file://$TMP_DIR/secret.json" >/dev/null
      public_key=$(printf '%s' "$existing_secret" | jq -r '.public_key')
    fi
  else
    public_key="$(generate_vapid_pair)"
    jq -n \
      --rawfile priv "$PRIVATE_KEY_PATH" \
        --arg pub "$public_key" \
      --arg sub "$resolved_subject" \
      --arg created "$now" \
      --arg updated "$now" \
      '{public_key:$pub, private_key:$priv, subject:$sub, created_at:$created, updated_at:$updated}' \
      >"$TMP_DIR/secret.json"
    aws_cli secretsmanager create-secret \
      --name "$SECRET_NAME" \
      --description "VAPID key pair for ${STAGE} (managed by ensure_vapid_credentials.sh)" \
      --secret-string "file://$TMP_DIR/secret.json" >/dev/null
    secret_arn=$(aws_cli secretsmanager describe-secret --secret-id "$SECRET_NAME" --query 'ARN' --output text)
  fi

  if [ -z "${public_key:-}" ]; then
    public_key=$(aws_cli secretsmanager get-secret-value --secret-id "$SECRET_NAME" --query 'SecretString' --output text | jq -r '.public_key')
  fi

  printf '%s\n%s\n%s\n' "$secret_arn" "$public_key" "$resolved_subject"
}

read -r SECRET_ARN PUBLIC_KEY SUBJECT_RESOLVED < <(ensure_secret)

if [ -z "$SECRET_ARN" ] || [ "$SECRET_ARN" = "None" ]; then
  echo "Failed to resolve VAPID secret ARN" >&2
  exit 1
fi

printf 'export VAPID_SECRET_ARN=%q\n' "$SECRET_ARN"
printf 'export VAPID_PUBLIC_KEY=%q\n' "$PUBLIC_KEY"
printf 'export VAPID_SUBJECT=%q\n' "$SUBJECT_RESOLVED"
