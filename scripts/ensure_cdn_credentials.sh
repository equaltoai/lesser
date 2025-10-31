#!/bin/bash

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <environment>" >&2
  echo "Example: $0 development" >&2
  exit 1
fi

ENVIRONMENT="$1"

resolve_stage() {
  case "$1" in
    development|dev) echo "dev" ;;
    staging|test) echo "staging" ;;
    production|live) echo "live" ;;
    *) echo "$1" ;;
  esac
}

STAGE="$(resolve_stage "$ENVIRONMENT")"
SECRET_NAME="lesser/cdn-private-key-${STAGE}"
KEY_NAME="lesser-${STAGE}-key"
KEYGROUP_NAME="lesser-${STAGE}-keygroup"
CALLER_REFERENCE="lesser-${STAGE}-$(date +%s)"

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
PUBLIC_KEY_PATH="$TMP_DIR/public.pem"

ensure_secret() {
  local arn
  if arn=$(aws_cli secretsmanager describe-secret --secret-id "$SECRET_NAME" --query 'ARN' --output text 2>/dev/null); then
    # Secret exists; extract private key for later use
    aws_cli secretsmanager get-secret-value --secret-id "$SECRET_NAME" --query 'SecretString' --output text >"$PRIVATE_KEY_PATH"
  else
    # Generate new RSA key pair
    openssl genrsa -out "$PRIVATE_KEY_PATH" 2048 >/dev/null 2>&1
    aws_cli secretsmanager create-secret \
      --name "$SECRET_NAME" \
      --description "CloudFront private key for ${STAGE} (managed by ensure_cdn_credentials.sh)" \
      --secret-string "file://$PRIVATE_KEY_PATH" >/dev/null
    arn=$(aws_cli secretsmanager describe-secret --secret-id "$SECRET_NAME" --query 'ARN' --output text)
  fi
  echo "$arn"
}

ensure_public_key() {
  local key_id
  key_id=$(aws_cli cloudfront list-public-keys \
    --query "PublicKeyList.Items[?Name=='${KEY_NAME}'].Id" \
    --output text 2>/dev/null | tr -d '\r')

  if [ -z "$key_id" ] || [ "$key_id" = "None" ]; then
    openssl rsa -in "$PRIVATE_KEY_PATH" -pubout -out "$PUBLIC_KEY_PATH" >/dev/null 2>&1
    local encoded_key
    encoded_key=$(python3 - <<'PY' "$PUBLIC_KEY_PATH"
import json, sys
with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = f.read()
print(json.dumps(data))
PY
)
    local payload
    payload=$(cat <<JSON
{
  "CallerReference": "${CALLER_REFERENCE}",
  "Name": "${KEY_NAME}",
  "EncodedKey": ${encoded_key},
  "Comment": "Managed by ensure_cdn_credentials.sh"
}
JSON
)
    key_id=$(aws_cli cloudfront create-public-key \
      --public-key-config "$payload" \
      --query 'PublicKey.Id' \
      --output text)
  fi

  echo "$key_id"
}

ensure_key_group() {
  local key_id="$1"
  local group_id
  group_id=$(aws_cli cloudfront list-key-groups \
    --query "KeyGroupList.Items[?KeyGroup.KeyGroupConfig.Name=='${KEYGROUP_NAME}'].KeyGroup.Id" \
    --output text 2>/dev/null | tr -d '\r')

  if [ -z "$group_id" ] || [ "$group_id" = "None" ]; then
    group_id=$(aws_cli cloudfront create-key-group \
      --key-group-config "{\"Name\":\"${KEYGROUP_NAME}\",\"Items\":[\"${key_id}\"],\"Comment\":\"Managed by ensure_cdn_credentials.sh\"}" \
      --query 'KeyGroup.Id' \
      --output text)
  else
    local current
    current=$(aws_cli cloudfront get-key-group-config --id "$group_id")
    local etag
    etag=$(echo "$current" | jq -r '.ETag')
    local needs_update
    needs_update=$(echo "$current" | jq --arg key "$key_id" '(.KeyGroupConfig.Items | index($key)) == null')
    if [ "$needs_update" = "true" ]; then
      local update_payload
      update_payload=$(echo "$current" | jq --arg key "$key_id" '
        .KeyGroupConfig.Items |= (if index($key) == null then . + [$key] else . end)
      ' | jq '.KeyGroupConfig')
      aws_cli cloudfront update-key-group \
        --id "$group_id" \
        --if-match "$etag" \
        --key-group-config "$update_payload" >/dev/null
    fi
  fi

  echo "$group_id"
}

SECRET_ARN="$(ensure_secret)"
openssl rsa -in "$PRIVATE_KEY_PATH" -pubout -out "$PUBLIC_KEY_PATH" >/dev/null 2>&1
KEY_ID="$(ensure_public_key)"
KEY_GROUP_ID="$(ensure_key_group "$KEY_ID")"

cat <<EOF
CLOUDFRONT_PRIVATE_KEY_PATH=${SECRET_NAME}
CLOUDFRONT_KEY_PAIR_ID=${KEY_ID}
CLOUDFRONT_KEY_GROUP_ID=${KEY_GROUP_ID}
CLOUDFRONT_SECRET_ARN=${SECRET_ARN}
EOF
