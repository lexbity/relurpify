#!/usr/bin/env bash
# Enforce invariants V2 and V3 from the relurplint engineering spec.
set -euo pipefail

fail=0

# V2: scripts/boundaryaudit must not be imported by any runtime or app package.
importers=$(grep -rn '"codeburg.org/lexbit/relurpify/scripts/boundaryaudit"' --include='*.go' . \
  | grep -v 'scripts/boundaryaudit/' \
  || true)
if [ -n "$importers" ]; then
  echo "FAIL V2: scripts/boundaryaudit imported by non-audit package:"
  echo "$importers"
  fail=1
else
  echo "PASS V2: scripts/boundaryaudit is import-isolated"
fi

# V3: relurplint must not define its own validation logic (only map/render).
# Check that each non-test check_*.go file imports at least one validator.
for f in app/relurplint/check_*.go; do
  [ -f "$f" ] || continue
  case "$f" in
    *_test.go) continue ;;
  esac
  if ! grep -q 'codeburg.org/lexbit/relurpify/framework/cfgload\|codeburg.org/lexbit/relurpify/framework/prompt\|codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipe\|codeburg.org/lexbit/relurpify/testsuite/configcheck' "$f" 2>/dev/null; then
    echo "FAIL V3: $f does not import a validator package"
    fail=1
  fi
done
if [ "$fail" -eq 0 ]; then
  echo "PASS V3: relurplint checks delegate to validators"
fi

exit $fail
