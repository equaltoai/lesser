#!/usr/bin/env bash
set -euo pipefail

input_domain="${1:-}"
if [[ -z "$input_domain" ]]; then
  echo "usage: $0 <domain-or-root-domain>" >&2
  exit 2
fi

domain="$(echo "$input_domain" | tr '[:upper:]' '[:lower:]' | xargs)"
domain="${domain%.}"

case "$domain" in
  dev.*) root_domain="${domain#dev.}" ;;
  staging.*) root_domain="${domain#staging.}" ;;
  live.*) root_domain="${domain#live.}" ;;
  *) root_domain="$domain" ;;
esac

if [[ -z "$root_domain" ]]; then
  echo "failed to resolve root domain from input: $input_domain" >&2
  exit 2
fi

zone_id="$(
  aws route53 list-hosted-zones-by-name \
    --dns-name "$root_domain" \
    --query "HostedZones[?Name=='${root_domain}.']|[0].Id" \
    --output text
)"

if [[ -z "$zone_id" || "$zone_id" == "None" ]]; then
  echo "no hosted zone found for: $root_domain (create it in Route53, or pass ROOT_DOMAIN that matches an existing zone)" >&2
  exit 1
fi

zone_id="${zone_id#/hostedzone/}"

cat <<EOF
export HOSTED_ZONE_NAME='${root_domain}'
export HOSTED_ZONE_ID='${zone_id}'
EOF

