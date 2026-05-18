#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/processor_storm_recovery_snapshot.sh --app <app> --stage <stage> --profile <aws-profile> [options]

Read-only evidence collector for processor-storm recovery preparation. The script writes local JSON/Markdown files
and only calls AWS read APIs. It does not deploy, mutate event-source mappings, mutate EventBridge rules, purge or
redrive queues, run backfills, or read secret values.

Options:
  --app <app>           App slug, e.g. simulacrum or theory. Required.
  --stage <stage>       Stage, e.g. dev, staging, live. Required.
  --profile <profile>   AWS profile. Required.
  --region <region>     AWS region. Default: us-east-1.
  --domain <domain>     Stage domain to record in summary. Optional.
  --hours <n>           CloudWatch lookback hours. Default: 1.
  --out <dir>           Output directory. Default: tmp/processor-storm-recovery/<app>-<stage>-<timestamp>.
  -h, --help            Show this help.
USAGE
}

app=""
stage=""
profile=""
region="us-east-1"
domain=""
hours="1"
out=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --app) app=${2:-}; shift 2 ;;
    --stage) stage=${2:-}; shift 2 ;;
    --profile|--aws-profile) profile=${2:-}; shift 2 ;;
    --region) region=${2:-}; shift 2 ;;
    --domain|--stage-domain) domain=${2:-}; shift 2 ;;
    --hours) hours=${2:-}; shift 2 ;;
    --out) out=${2:-}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$app" || -z "$stage" || -z "$profile" ]]; then
  echo "--app, --stage, and --profile are required" >&2
  usage >&2
  exit 2
fi

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI is required" >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 2
fi

prefix="${app}-${stage}"
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
if [[ -z "$out" ]]; then
  out="tmp/processor-storm-recovery/${prefix}-${timestamp}"
fi
mkdir -p "$out/tables"

start=$(date -u -d "${hours} hour ago" +%Y-%m-%dT%H:%M:%SZ)
end=$(date -u +%Y-%m-%dT%H:%M:%SZ)
period=300

aws_ro() {
  AWS_PROFILE="$profile" AWS_REGION="$region" aws "$@"
}

metric_sum() {
  local metric=$1 dims=$2
  aws_ro cloudwatch get-metric-statistics \
    --namespace AWS/Lambda --metric-name "$metric" --dimensions "$dims" \
    --start-time "$start" --end-time "$end" --period "$period" --statistics Sum \
    --query 'sum(Datapoints[].Sum)' --output text 2>/dev/null || true
}

metric_max() {
  local metric=$1 dims=$2
  aws_ro cloudwatch get-metric-statistics \
    --namespace AWS/Lambda --metric-name "$metric" --dimensions "$dims" \
    --start-time "$start" --end-time "$end" --period "$period" --statistics Maximum \
    --query 'max(Datapoints[].Maximum)' --output text 2>/dev/null || true
}

aws_ro sts get-caller-identity --output json > "$out/caller.json"
aws_ro cloudformation describe-stacks --stack-name "$prefix" --output json > "$out/stack.json" 2>/dev/null || true
aws_ro lambda list-functions --query "Functions[?starts_with(FunctionName, \`${prefix}-\`)]" --output json > "$out/functions.json"

jq -r '.[] | select(.FunctionName|test("(processor|aggregator|indexer|tracker|scheduler|delivery|router)$")) | .FunctionName' \
  "$out/functions.json" | sort > "$out/processor-functions.txt"

: > "$out/event-source-mappings.jsonl"
: > "$out/function-metrics.tsv"
while IFS= read -r fn; do
  [[ -n "$fn" ]] || continue
  aws_ro lambda list-event-source-mappings --function-name "$fn" --output json \
    | jq -c --arg fn "$fn" '.EventSourceMappings[]? | {function:$fn,uuid:.UUID,state:.State,stateTransitionReason:.StateTransitionReason,lastModified:.LastModified,eventSourceArn:.EventSourceArn,batchSize:.BatchSize,maximumBatchingWindowInSeconds:.MaximumBatchingWindowInSeconds,parallelizationFactor:.ParallelizationFactor,startingPosition:.StartingPosition,maximumRetryAttempts:.MaximumRetryAttempts,maximumRecordAgeInSeconds:.MaximumRecordAgeInSeconds,bisectBatchOnFunctionError:.BisectBatchOnFunctionError,functionResponseTypes:.FunctionResponseTypes,destinationConfig:.DestinationConfig}' \
    >> "$out/event-source-mappings.jsonl"

  inv=$(metric_sum Invocations "Name=FunctionName,Value=$fn"); inv=${inv:-None}
  err=$(metric_sum Errors "Name=FunctionName,Value=$fn"); err=${err:-None}
  throttles=$(metric_sum Throttles "Name=FunctionName,Value=$fn"); throttles=${throttles:-None}
  duration=$(metric_max Duration "Name=FunctionName,Value=$fn"); duration=${duration:-None}
  iterator_age=$(metric_max IteratorAge "Name=FunctionName,Value=$fn"); iterator_age=${iterator_age:-None}
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$fn" "$inv" "$err" "$throttles" "$duration" "$iterator_age" \
    >> "$out/function-metrics.tsv"
done < "$out/processor-functions.txt"

aws_ro events list-rules --name-prefix "$prefix" --output json > "$out/eventbridge-rules.json" 2>/dev/null || true
jq -r '.Rules[]?.Name' "$out/eventbridge-rules.json" | sort > "$out/eventbridge-rule-names.txt"
: > "$out/eventbridge-targets.jsonl"
while IFS= read -r rule; do
  [[ -n "$rule" ]] || continue
  aws_ro events list-targets-by-rule --rule "$rule" --output json \
    | jq -c --arg rule "$rule" '.Targets[]? | {rule:$rule,id:.Id,arn:.Arn,input:.Input}' \
    >> "$out/eventbridge-targets.jsonl" || true
done < "$out/eventbridge-rule-names.txt"

aws_ro sqs list-queues --queue-name-prefix "$prefix" --output json > "$out/sqs-queues.json" 2>/dev/null || true
jq -r '.QueueUrls[]?' "$out/sqs-queues.json" | sort > "$out/sqs-queue-urls.txt"
: > "$out/sqs-attributes.jsonl"
while IFS= read -r qurl; do
  [[ -n "$qurl" ]] || continue
  aws_ro sqs get-queue-attributes --queue-url "$qurl" --attribute-names All --output json \
    | jq -c --arg url "$qurl" '{url:$url,attributes:.Attributes}' \
    >> "$out/sqs-attributes.jsonl" || true
done < "$out/sqs-queue-urls.txt"

for table in "${prefix}-main-table" "${prefix}-stream-events-table" "${prefix}-rate-limits-table"; do
  aws_ro dynamodb describe-table --table-name "$table" --output json > "$out/tables/${table}.json" 2>/dev/null || true
done

aws_ro secretsmanager list-secrets --filters Key=name,Values="$app" --output json > "$out/secrets.json" 2>/dev/null || true
aws_ro kms list-aliases --output json > "$out/kms-aliases.json" 2>/dev/null || true
alias_name="alias/${app}-shared-encryption"
key_id=$(jq -r --arg alias "$alias_name" '.Aliases[]? | select(.AliasName==$alias) | .TargetKeyId // empty' "$out/kms-aliases.json" | head -1)
if [[ -n "$key_id" ]]; then
  aws_ro kms describe-key --key-id "$key_id" --output json > "$out/kms-shared-encryption.json" 2>/dev/null || true
fi

{
  echo "# Processor storm recovery snapshot: ${prefix}"
  echo
  echo "- app: ${app}"
  echo "- stage: ${stage}"
  echo "- profile: ${profile}"
  echo "- account: $(jq -r '.Account // "unknown"' "$out/caller.json")"
  echo "- region: ${region}"
  echo "- domain: ${domain:-unknown}"
  echo "- collection window: ${start} to ${end}"
  echo "- output: ${out}"
  echo
  echo "## Stack"
  jq -r '.Stacks[0] // {} | "- stack: \(.StackName // "unknown")\n- status: \(.StackStatus // "unknown")\n- last updated: \(.LastUpdatedTime // "unknown")"' "$out/stack.json" 2>/dev/null || echo "- stack metadata unavailable"
  echo
  echo "## Processor-like functions"
  jq -r '.[] | select(.FunctionName|test("(processor|aggregator|indexer|tracker|scheduler|delivery|router)$")) | "- \(.FunctionName): lastModified=\(.LastModified), codeSha256=\(.CodeSha256)"' "$out/functions.json"
  echo
  echo "## Event source mappings"
  if [[ -s "$out/event-source-mappings.jsonl" ]]; then
    jq -r '. | "- \(.function): state=\(.state), source=\(.eventSourceArn), retry=\(.maximumRetryAttempts // "null"), maxAge=\(.maximumRecordAgeInSeconds // "null"), bisect=\(.bisectBatchOnFunctionError // "null"), partial=\((.functionResponseTypes // [])|join(",")), onFailure=\(.destinationConfig.OnFailure.Destination // "none"), batch=\(.batchSize), window=\(.maximumBatchingWindowInSeconds // 0), parallel=\(.parallelizationFactor // "null"), reason=\(.stateTransitionReason // "")"' "$out/event-source-mappings.jsonl"
  else
    echo "- none found"
  fi
  echo
  echo "## EventBridge rules"
  jq -r '.Rules[]? | "- \(.Name): state=\(.State), schedule=\(.ScheduleExpression // "event-pattern")"' "$out/eventbridge-rules.json"
  echo
  echo "## SQS depths"
  if [[ -s "$out/sqs-attributes.jsonl" ]]; then
    jq -r '. | .attributes as $a | "- \(.url|split("/")[-1]): visible=\($a.ApproximateNumberOfMessages // "0"), notVisible=\($a.ApproximateNumberOfMessagesNotVisible // "0"), delayed=\($a.ApproximateNumberOfMessagesDelayed // "0"), oldestAgeSeconds=\($a.ApproximateAgeOfOldestMessage // "0"), kms=\($a.KmsMasterKeyId // "none"), redrive=\($a.RedrivePolicy // "none")"' "$out/sqs-attributes.jsonl"
  else
    echo "- none found"
  fi
  echo
  echo "## Lambda metrics (${hours}h window)"
  awk -F '\t' '{printf "- %s: invocations=%s errors=%s throttles=%s maxDurationMs=%s maxIteratorAgeMs=%s\n", $1,$2,$3,$4,$5,$6}' "$out/function-metrics.tsv"
  echo
  echo "## Secrets/KMS metadata only"
  jq -r '.SecretList[]? | "- secret: \(.Name), arn=\(.ARN), kms=\(.KmsKeyId // "default"), lastChanged=\(.LastChangedDate // "unknown")"' "$out/secrets.json"
  if [[ -f "$out/kms-shared-encryption.json" ]]; then
    jq -r '.KeyMetadata | "- kms shared encryption: keyId=\(.KeyId), arn=\(.Arn), state=\(.KeyState), enabled=\(.Enabled), origin=\(.Origin)"' "$out/kms-shared-encryption.json"
  fi
  echo
  echo "## Tables"
  for table_file in "$out"/tables/*.json; do
    [[ -f "$table_file" ]] || continue
    jq -r '.Table | "- \(.TableName): status=\(.TableStatus), streamEnabled=\(.StreamSpecification.StreamEnabled // false), streamView=\(.StreamSpecification.StreamViewType // "none"), latestStreamArn=\(.LatestStreamArn // "none")"' "$table_file"
  done
} > "$out/summary.md"

echo "wrote ${out}/summary.md"
