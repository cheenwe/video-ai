#!/usr/bin/env bash
# M5 four-way soak test: create 4 deployments, run them, verify every lane
# keeps heartbeating, then simulate one stream failing and confirm the other
# three keep analysing. Exits non-zero on any failed assertion.
#
# Usage:
#   BASE_URL=http://127.0.0.1:8080 ADMIN_PASS='ChangeMe123' ./scripts/video-ai-soak.sh
#   BASE_URL=... TOKEN='Bearer x' ./scripts/video-ai-soak.sh
#
# Options (env):
#   DURATION=90      seconds to observe steady-state heartbeats
#   USERNAME=admin   login name when using password auth
# -u is off deliberately: empty arrays are used before they are filled, which
# older bash (3.2 on macOS) rejects under nounset.
set -eo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
USERNAME="${USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASS:-}"
DURATION="${DURATION:-90}"
LANES=4

fail() { echo "FAIL: $*" >&2; exit 1; }
info() { echo "==> $*"; }

command -v python3 >/dev/null 2>&1 || fail "需要 python3 解析接口返回"
command -v curl >/dev/null 2>&1 || fail "需要 curl"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
COOKIE_JAR="$TMP/cookies.txt"
AUTH_HEADER=()

login() { # login PASSWORD
  curl -fsS --noproxy '*' -c "$COOKIE_JAR" -H 'Content-Type: application/json' \
    -H "Origin: ${BASE_URL%/}" \
    -d "$(printf '{"username":"%s","password":"%s"}' "$USERNAME" "$1")" \
    "${BASE_URL%/}/api/auth/login" >"$TMP/login.json" || fail "登录失败"
}

if [[ -n "${TOKEN:-}" ]]; then
  AUTH_HEADER=(-H "Authorization: Bearer ${TOKEN#Bearer }")
else
  [[ -n "$ADMIN_PASS" ]] || fail "请设置 ADMIN_PASS 或 TOKEN"
  info "登录 $BASE_URL"
  login "$ADMIN_PASS"

  # A freshly seeded admin must rotate its password before any API works.
  if python3 -c "import json,sys; sys.exit(0 if json.load(open('$TMP/login.json'))['data']['user'].get('must_change_password') else 1)"; then
    NEW_PASS="${NEW_PASS:-${ADMIN_PASS}Aa1}"
    info "初始密码需轮换，改为 NEW_PASS"
    curl -fsS --noproxy '*' -b "$COOKIE_JAR" -H 'Content-Type: application/json' -H "Origin: ${BASE_URL%/}" \
      -d "$(printf '{"old_password":"%s","new_password":"%s"}' "$ADMIN_PASS" "$NEW_PASS")" \
      "${BASE_URL%/}/api/auth/change-password" >/dev/null || fail "改密失败"
    # The session is revoked by the rotation, so log in again.
    login "$NEW_PASS"
    ADMIN_PASS="$NEW_PASS"
  fi
fi

api() { # api METHOD PATH [BODY]
  local method="$1" path="$2" body="${3-}"
  # --noproxy keeps localhost traffic off any corporate proxy.
  local args=(-sS --noproxy '*' -b "$COOKIE_JAR" -H 'Content-Type: application/json' -H "Origin: ${BASE_URL%/}")
  # Guard against set -u with an empty array (password auth path).
  args+=(${AUTH_HEADER[@]+"${AUTH_HEADER[@]}"})
  [[ -n "$body" ]] && args+=(-d "$body")
  curl -X "$method" "${args[@]}" "${BASE_URL%/}$path"
}

# jget FILE PYEXPR — evaluate a python expression over the JSON body.
# Fails loudly (stderr) when the API itself reported an error, so callers can
# attach the raw response to their failure message.
jget() {
  python3 - "$1" "$2" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))
if data.get("success") != 1:
    sys.stderr.write("接口返回失败: %s\n" % data.get("msg"))
    sys.exit(1)
try:
    print(eval(sys.argv[2], {"d": data, "json": json}))
except Exception as e:
    sys.stderr.write("解析失败: %s\n" % e)
    sys.exit(1)
PY
}

info "查询内置业务模板"
api GET "/api/algorithms/templates" >"$TMP/templates.json"
TPL_ID=$(jget "$TMP/templates.json" "d['data'][0]['id']")
[[ "$TPL_ID" =~ ^[0-9]+$ ]] || fail "未找到业务模板（$TPL_ID）"

GEOM='{"kind":"polyline","pit_side":"below","buffer_norm":0.02,"points":[{"x":100,"y":400},{"x":640,"y":420},{"x":1180,"y":400}]}'

DEP_IDS=()
CAM_IDS=()
for i in $(seq 1 "$LANES"); do
  CODE="soak-cam-$i"
  # Reuse a camera left over from an earlier run so the script is repeatable.
  api GET "/api/cameras?keyword=$CODE" >"$TMP/find$i.json"
  CAM_ID=$(jget "$TMP/find$i.json" "d['data']['items'][0]['id'] if d['data'].get('items') else 0" 2>/dev/null || echo 0)
  if [[ ! "$CAM_ID" =~ ^[1-9][0-9]*$ ]]; then
    info "创建摄像头 $CODE"
    api POST "/api/cameras" "{\"code\":\"$CODE\",\"name\":\"压测 $i 路\",\"access_type\":\"rtsp\",\"main_url\":\"rtsp://127.0.0.1:554/$i\"}" >"$TMP/cam$i.json"
    CAM_ID=$(jget "$TMP/cam$i.json" "d['data']['id']") || fail "创建摄像头失败：$(cat "$TMP/cam$i.json")"
    [[ "$CAM_ID" =~ ^[0-9]+$ ]] || fail "创建摄像头失败：$(cat "$TMP/cam$i.json")"
  else
    info "复用摄像头 $CODE (#$CAM_ID)"
    cp "$TMP/find$i.json" "$TMP/cam$i.json"
    # Re-enable in case a previous run disabled it to simulate a dropout.
    api PUT "/api/cameras/$CAM_ID" "{\"code\":\"$CODE\",\"name\":\"压测 $i 路\",\"access_type\":\"rtsp\",\"enabled\":1}" >/dev/null
  fi

  # Reuse an already published boundary, otherwise create and publish one.
  api GET "/api/boundaries?camera_id=$CAM_ID" >"$TMP/blist$i.json"
  BID=$(jget "$TMP/blist$i.json" "next((b['id'] for b in d['data'] if b.get('status')=='published'), 0)" 2>/dev/null || echo 0)
  if [[ ! "$BID" =~ ^[1-9][0-9]*$ ]]; then
    api POST "/api/boundaries" "{\"camera_id\":$CAM_ID,\"name\":\"soak edge\",\"ref_width\":1280,\"ref_height\":720,\"geometry\":$GEOM}" >"$TMP/bound$i.json"
    BID=$(jget "$TMP/bound$i.json" "d['data']['id']") || fail "创建边界失败：$(cat "$TMP/bound$i.json")"
    api POST "/api/boundaries/$BID/publish" "{}" >/dev/null
  fi

  # Reuse an existing deployment for this camera, else create one.
  api GET "/api/deployments" >"$TMP/dlist$i.json"
  DID=$(jget "$TMP/dlist$i.json" "next((x['id'] for x in d['data'] if x.get('camera_id')==$CAM_ID), 0)" 2>/dev/null || echo 0)
  if [[ ! "$DID" =~ ^[1-9][0-9]*$ ]]; then
    api POST "/api/deployments" "{\"name\":\"soak-$i\",\"camera_id\":$CAM_ID,\"template_id\":$TPL_ID,\"boundary_version_id\":$BID,\"analysis_fps\":2}" >"$TMP/dep$i.json"
    DID=$(jget "$TMP/dep$i.json" "d['data']['id']") || fail "创建布控失败：$(cat "$TMP/dep$i.json")"
    [[ "$DID" =~ ^[0-9]+$ ]] || fail "创建布控失败：$(cat "$TMP/dep$i.json")"
  fi

  info "启动布控 #$DID : 自检 -> 启动"
  api POST "/api/deployments/$DID/self-check" "{}" >"$TMP/check$i.json"
  api POST "/api/deployments/$DID/start" "{}" >/dev/null
  DEP_IDS+=("$DID")
  CAM_IDS+=("$CAM_ID")
done

# Observe heartbeats on every lane.
info "观察 ${DURATION}s：每一路都应持续心跳"
DEADLINE=$(( $(date +%s) + DURATION ))
LATEST_HEARTBEAT=$(date +%s)
while [[ $(date +%s) -lt $DEADLINE ]]; do
  sleep 10
  api GET "/api/reports/runtime" >"$TMP/runtime.json"
  # Count lanes whose state is running.
  RUNNING=$(python3 - "$TMP/runtime.json" "${DEP_IDS[*]}" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))["data"]
want={int(x) for x in sys.argv[2].split()}
rows=[r for r in data if r.get("deployment_id") in want]
print(sum(1 for r in rows if r.get("state") in ("running","out_of_schedule")))
PY
)
  echo "    运行中路数: $RUNNING / $LANES"
  if [[ "$RUNNING" -lt "$LANES" ]]; then
    fail "出现掉线：期望 $LANES 路运行，实际 $RUNNING 路"
  fi
done

# Simulate one lane losing its stream by disabling its camera. The remaining
# lanes must keep analysing.
info "模拟第 1 路断流（停用摄像头）"
FIRST_CAM="${CAM_IDS[0]}"
api PUT "/api/cameras/$FIRST_CAM" "{\"code\":\"soak-cam-1\",\"name\":\"压测 1 路\",\"access_type\":\"rtsp\",\"enabled\":0}" >/dev/null
sleep 20

api GET "/api/reports/runtime" >"$TMP/runtime2.json"
RUNNING=$(python3 - "$TMP/runtime2.json" "${DEP_IDS[*]:1}" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))["data"]
want={int(x) for x in sys.argv[2].split()}
rows=[r for r in data if r.get("deployment_id") in want]
print(sum(1 for r in rows if r.get("state") in ("running","out_of_schedule")))
PY
)
echo "    其余路数运行中: $RUNNING / $((LANES - 1))"
[[ "$RUNNING" -ge $((LANES - 1)) ]] || fail "断流后其余路未继续分析（$RUNNING）"

info "停止并清理布控"
for DID in "${DEP_IDS[@]}"; do
  api POST "/api/deployments/$DID/stop" "{}" >/dev/null
done

info "四路压测通过：全部路数持续心跳，单路断流不影响其余路"
