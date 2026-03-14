#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

pattern='agents\.(ResolveSkills|ApplySkills|EnumerateSkillCapabilities|ResolveSkillPaths|ValidateSkillPaths|SkillRoot|SkillManifestPath|ResolveAgentSpec|ApplyManifestDefaultsForAgent|ApplyManifestDefaults|GlobalAgentDefaults|DefaultConfigPath|DefaultAgentPaths|LoadGlobalConfig|SaveGlobalConfig)'
matches="$(rg -n "$pattern" app testsuite --glob '*.go' || true)"

if [[ -n "${matches}" ]]; then
  echo "deprecated agents wrapper usage detected in app/test code"
  echo
  echo "${matches}"
  exit 1
fi

echo "deprecated agents wrapper check passed"
