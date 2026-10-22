#!/usr/bin/env bash
# M5 smoke checklist helper: calls platform video self-check API.
# Usage: BASE_URL=http://127.0.0.1:8080 COOKIE='baseplatform_session=...' ./scripts/video-ai-selfcheck.sh
set -euo pipefail
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
if [[ -z "${COOKIE:-}" && -z "${TOKEN:-}" ]]; then
  echo "Set COOKIE=baseplatform_session=... or TOKEN=Bearer jwt" >&2
  exit 2
fi
AUTH=()
if [[ -n "${TOKEN:-}" ]]; then
  AUTH=(-H "Authorization: Bearer ${TOKEN#Bearer }")
else
  AUTH=(-H "Cookie: ${COOKIE}")
fi
curl -fsS "${AUTH[@]}" "${BASE_URL%/}/api/video/self-check" | python3 -m json.tool
echo
echo "Manual long-run checks:"
echo " 1) Start 4 deployments; confirm runtime heartbeats on /admin/reports"
echo " 2) Disconnect one stream; other three keep analyzing"
echo " 3) Change main stream URL; published boundary becomes invalid"
echo " 4) Stop/start platform; running deployments restore via videoai.Manager"