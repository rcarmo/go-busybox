#!/bin/bash
set -euo pipefail

ENFORCE=false
THRESHOLD=80
if [[ "${1:-}" == "--enforce" ]]; then
  ENFORCE=true
  THRESHOLD=${2:-80}
fi

if [[ ! -d pkg/applets ]]; then
  echo "0%"
  exit 0
fi

applets=$(find pkg/applets -maxdepth 1 -mindepth 1 -type d -printf "%f\n" | wc -l | tr -d ' ')
fuzz=$(find pkg/applets -maxdepth 2 -name '*_fuzz_test.go' -printf '%h\n' | xargs -r -n1 basename | sort -u | wc -l | tr -d ' ')
if [[ "$applets" -eq 0 ]]; then
  echo "0%"
  exit 0
fi

pct=$(python3 - <<PY
applets=$applets
fuzz=$fuzz
print(f"{(fuzz/applets)*100:.1f}%")
PY
)

echo "$pct"

if $ENFORCE; then
  pct_val=$(echo "$pct" | tr -d '%')
  rounded=$(python3 - <<PY
val=float("$pct_val")
print(int(val + 0.5))
PY
)
  if [[ "$rounded" -lt "$THRESHOLD" ]]; then
    echo "Fuzz target coverage ${pct} is below ${THRESHOLD}%"
    exit 1
  fi
fi
