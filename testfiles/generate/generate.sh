#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GEN="$(cd "$(dirname "$0")" && pwd)"
VENV="${GEN}/.venv"

cd "$ROOT"

if [[ ! -d "$VENV" ]]; then
  python3 -m venv "$VENV"
fi
# shellcheck source=/dev/null
source "$VENV/bin/activate"
pip install -q -r "$GEN/requirements.txt"
python3 "$GEN/generate.py"
