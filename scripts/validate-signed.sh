#!/usr/bin/env bash
# Validate signed PDF test outputs (ISO 32000 structure, byte-range integrity,
# Go library verification, optional EU DSS / ETSI PAdES checks).
#
# Usage:
#   ./scripts/validate-signed.sh
#   ./scripts/validate-signed.sh testfiles/success/foo.pdf
#   ./scripts/validate-signed.sh --dss --with-dss-docker
#   DSS_URL=http://localhost:8080 ./scripts/validate-signed.sh --dss
#
# Prerequisites:
#   pdfcpu  — go install github.com/pdfcpu/pdfcpu/cmd/pdfcpu@latest
#   jq      — for pdfsign verify JSON checks
#   python3 — for DSS validation (--dss)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DIR="$ROOT/testfiles/success"
RUN_PDFCPU=1
RUN_VERIFY=1
RUN_DSS=0
WITH_DSS_DOCKER=0
DSS_URL="${DSS_URL:-http://localhost:8080}"
DSS_CONTAINER="pdfsign-dss-validate"

usage() {
  sed -n '2,12p' "$0" | sed 's/^# \?//'
  echo ""
  echo "Options:"
  echo "  --dir PATH           Directory of signed PDFs (default: testfiles/success)"
  echo "  --skip-pdfcpu        Skip pdfcpu structure and signature integrity"
  echo "  --skip-verify        Skip pdfsign verify (RFC 5652/CMS + byte range via Go)"
  echo "  --dss                Run EU DSS PAdES validation (ETSI rules via DSS)"
  echo "  --with-dss-docker    Start ghcr.io/vysmaty/dockerized-dss before --dss"
  echo "  --dss-url URL        DSS base URL (default: \$DSS_URL or http://localhost:8080)"
  echo "  -h, --help           Show this help"
}

cleanup_dss() {
  if docker ps -q -f "name=^${DSS_CONTAINER}$" 2>/dev/null | grep -q .; then
    docker rm -f "$DSS_CONTAINER" >/dev/null 2>&1 || true
  fi
}

wait_for_dss() {
  local url="$1"
  local i
  echo "Waiting for DSS at $url ..."
  for i in $(seq 1 90); do
    if curl -sf "${url%/}/" >/dev/null 2>&1 || \
       curl -sf "${url%/}/services/rest/validation" >/dev/null 2>&1; then
      echo "DSS is ready."
      return 0
    fi
    sleep 2
  done
  echo "DSS did not become ready in time." >&2
  return 1
}

start_dss_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required for --with-dss-docker" >&2
    exit 1
  fi
  cleanup_dss
  echo "Starting EU DSS container ..."
  docker run -d --name "$DSS_CONTAINER" -p 8080:8080 ghcr.io/vysmaty/dockerized-dss:latest >/dev/null
  trap cleanup_dss EXIT
  wait_for_dss "$DSS_URL"
}

ensure_pdfcpu() {
  if command -v pdfcpu >/dev/null 2>&1; then
    return 0
  fi
  if command -v go >/dev/null 2>&1; then
    echo "Installing pdfcpu ..."
    go install github.com/pdfcpu/pdfcpu/cmd/pdfcpu@latest
    export PATH="$PATH:$(go env GOPATH)/bin"
  fi
  command -v pdfcpu >/dev/null 2>&1 || {
    echo "pdfcpu not found; install with: go install github.com/pdfcpu/pdfcpu/cmd/pdfcpu@latest" >&2
    exit 1
  }
}

ensure_pdfsign() {
  if [[ -x "$ROOT/pdfsign" ]]; then
    PDFSIGN="$ROOT/pdfsign"
    return 0
  fi
  if command -v go >/dev/null 2>&1; then
    echo "Building pdfsign ..."
    go build -o "$ROOT/pdfsign" .
    PDFSIGN="$ROOT/pdfsign"
    return 0
  fi
  echo "go is required to build pdfsign" >&2
  exit 1
}

ensure_jq() {
  command -v jq >/dev/null 2>&1 || {
    echo "jq is required for pdfsign verify checks" >&2
    exit 1
  }
}

collect_pdfs() {
  PDFS=()
  if [[ $# -gt 0 ]]; then
    PDFS=("$@")
    return 0
  fi
  if [[ ! -d "$DIR" ]]; then
    echo "No signed PDF directory: $DIR" >&2
    exit 1
  fi
  while IFS= read -r f; do
    PDFS+=("$f")
  done < <(find "$DIR" -name '*.pdf' -size +0c | sort)
  if [[ ${#PDFS[@]} -eq 0 ]]; then
    echo "No signed PDFs in $DIR (run: go test -v ./sign/...)" >&2
    exit 1
  fi
}

source_baseline_skip() {
  BASELINE_ERRORS="$ROOT/testfiles/.validate-baseline-errors.txt"
  : > "$BASELINE_ERRORS"
  for source_file in "$ROOT"/testfiles/*.pdf; do
    [[ -e "$source_file" ]] || continue
    filename=$(basename "$source_file")
    if ! pdfcpu validate --mode strict "$source_file" >/dev/null 2>&1; then
      echo "$filename" >> "$BASELINE_ERRORS"
    fi
  done
}

should_skip_pdf() {
  local signed_file="$1"
  local filename source_filename
  filename=$(basename "$signed_file")
  source_filename=$(echo "$filename" | sed 's/_TestSignPDF.pdf/.pdf/' | sed 's/_TestSignPDFVisibleAll.pdf/.pdf/')
  if [[ -f "$BASELINE_ERRORS" ]] && grep -qx "$source_filename" "$BASELINE_ERRORS"; then
    echo "⏭️  $filename (skipped — source $source_filename has pre-existing pdfcpu errors)"
    return 0
  fi
  return 1
}

validate_pdfcpu() {
  local signed_file="$1"
  local filename
  filename=$(basename "$signed_file")
  should_skip_pdf "$signed_file" && return 0

  echo "📄 $filename"
  if pdfcpu validate --mode strict "$signed_file" >/dev/null 2>&1; then
    echo "  ✅ pdfcpu structure (ISO 32000 strict)"
  else
    echo "  ❌ pdfcpu structure FAILED" >&2
    pdfcpu validate --mode strict "$signed_file" 2>&1 || true
    return 1
  fi

  local sig_out
  sig_out=$(pdfcpu signatures validate -f "$signed_file" 2>&1) || true
  if echo "$sig_out" | grep -q "DocModified: false"; then
    echo "  ✅ pdfcpu signature integrity (byte range / DocModified: false)"
  else
    echo "  ❌ pdfcpu signature integrity FAILED" >&2
    echo "$sig_out" >&2
    return 1
  fi
}

validate_pdfsign() {
  local signed_file="$1"
  local out valid
  out=$("$PDFSIGN" verify "$signed_file" 2>/dev/null || true)
  valid=$(echo "$out" | jq -r '.Signatures[0].validation.valid_signature // empty' 2>/dev/null || true)
  if [[ "$valid" == "true" ]]; then
    echo "  ✅ pdfsign verify (CMS + byte range, RFC 5652/9336)"
    return 0
  fi
  echo "  ❌ pdfsign verify FAILED (valid_signature=$valid)" >&2
  echo "$out" | jq '.Signatures[0].validation // .Signatures[0]' >&2 || echo "$out" >&2
  return 1
}

validate_dss() {
  local signed_file="$1"
  local profile="auto"
  local base
  base=$(basename "$signed_file")
  if [[ "$base" == *"_TestSignLTA"* ]]; then
    profile="lta"
  elif [[ "$base" == *"_TestSignLTV"* ]]; then
    profile="ltv"
  fi
  python3 "$ROOT/scripts/dss_validate.py" --url "$DSS_URL" --profile "$profile" "$signed_file"
}

# --- parse args ---
FILES=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      DIR="$2"
      shift 2
      ;;
    --skip-pdfcpu)
      RUN_PDFCPU=0
      shift
      ;;
    --skip-verify)
      RUN_VERIFY=0
      shift
      ;;
    --dss)
      RUN_DSS=1
      shift
      ;;
    --with-dss-docker)
      WITH_DSS_DOCKER=1
      RUN_DSS=1
      shift
      ;;
    --dss-url)
      DSS_URL="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      FILES+=("$@")
      break
      ;;
    -*)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      FILES+=("$1")
      shift
      ;;
  esac
done

if [[ $WITH_DSS_DOCKER -eq 1 ]]; then
  start_dss_docker
fi

collect_pdfs ${FILES[@]+"${FILES[@]}"}

[[ $RUN_PDFCPU -eq 1 ]] && ensure_pdfcpu && source_baseline_skip
[[ $RUN_VERIFY -eq 1 ]] && ensure_pdfsign && ensure_jq
if [[ $RUN_DSS -eq 1 ]]; then
  command -v python3 >/dev/null 2>&1 || {
    echo "python3 is required for --dss" >&2
    exit 1
  }
fi

FAILED=0
echo "### Signed PDF validation (${#PDFS[@]} file(s))"

for signed_file in "${PDFS[@]}"; do
  [[ -s "$signed_file" ]] || {
    echo "⏭️  $(basename "$signed_file") (empty — skipped)"
    continue
  }
  echo "--------------------------------------------------"
  if [[ $RUN_PDFCPU -eq 1 ]]; then
    validate_pdfcpu "$signed_file" || FAILED=1
  fi
  if [[ $RUN_VERIFY -eq 1 ]]; then
    validate_pdfsign "$signed_file" || FAILED=1
  fi
  if [[ $RUN_DSS -eq 1 ]]; then
    validate_dss "$signed_file" || FAILED=1
  fi
done

echo "--------------------------------------------------"
if [[ $FAILED -ne 0 ]]; then
  echo "❌ Validation failed."
  exit 1
fi
echo "✅ All validations passed."
