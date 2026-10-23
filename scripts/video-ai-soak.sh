#!/usr/bin/env bash
# M5 four-way soak test.
#
# There is no mock adapter: deployments only analyse real frames from a real
# model. The script therefore runs in one of two modes and says which one it
# ran — it never claims a pass it did not observe.
#
#   real mode      SOAK_RTSP_URL + SOAK_MODEL(+working onnx worker) provided:
#                  every lane must heartbeat AND analyse real frames.
#   fail-closed    defaults (no real stream/model): lanes must heartbeat,
#                  report offline, and produce ZERO events (no fabrication).
#
# Usage:
#   BASE_URL=http://127.0.0.1:8181 ADMIN_PASS='ChangeMe123' ./scripts/video-ai-soak.sh
#   BASE_URL=... TOKEN='Bearer x' ./scripts/video-ai-soak.sh
#   SOAK_RTSP_URL=rtsp://host/stream SOAK_MODEL=/path/model.onnx ./scripts/video-ai-soak.sh
#
# Options (env):
#   DURATION=90      seconds to observe steady-state heartbeats
#   USERNAME=admin   login name when using password auth
#   SOAK_RTSP_URL    reachable RTSP URL for all four lanes (real mode)
#   SOAK_MODEL       path to a real .onnx model (real mode)
#
# Exit codes: 0 = PASS, 1 = FAIL (assertion), 3 = BLOCKED (real mode
# requested but the onnx worker / stream is unavailable).
#
# -u is off deliberately: empty arrays are used before they are filled, which
# older bash (3.2 on macOS) rejects under nounset.
set -eo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
USERNAME="${USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASS:-}"
DURATION="${DURATION:-90}"
LANES=4
SOAK_RTSP_URL="${SOAK_RTSP_URL:-}"
SOAK_MODEL="${SOAK_MODEL:-}"

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

# ------------------------------------------------------------------ mode ----

REAL_MODE=0
if [[ -n "$SOAK_RTSP_URL" || -n "$SOAK_MODEL" ]]; then
  [[ -n "$SOAK_RTSP_URL" && -n "$SOAK_MODEL" ]] || fail "真实模式需要同时提供 SOAK_RTSP_URL 与 SOAK_MODEL"
  [[ -f "$SOAK_MODEL" ]] || fail "SOAK_MODEL 文件不存在: $SOAK_MODEL"
  REAL_MODE=1
fi

if [[ "$REAL_MODE" == "1" ]]; then
  # Real inference needs a working onnx worker; verify before promising a pass.
  api GET "/api/video/self-check" >"$TMP/selfcheck.json"
  WORKER_OK=$(python3 - "$TMP/selfcheck.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
checks=d.get("data",{}).get("checks",[])
print(1 if any(c.get("name")=="onnx_worker" and c.get("ok") for c in checks) else 0)
PY
)
  if [[ "$WORKER_OK" != "1" ]]; then
    echo "BLOCKED: 请求了真实模式，但 onnx 推理 worker 不可用（见 /api/video/self-check 的 onnx_worker 项）。" >&2
    echo "        安装 worker 依赖（backend/workers/onnx）后重试；本次不计入 soak 通过。" >&2
    exit 3
  fi
  info "模式：真实帧 + 真实 ONNX 模型"
else
  info "模式：fail-closed（无真实码流/模型；断言心跳 + offline + 零事件，禁止伪造）"
fi

# ------------------------------------------------------- model version ----

# Deployments only start when the template's model version is published.
# Real mode registers the operator's model; fail-closed mode publishes a
# placeholder file so the start gate passes — inference then fails honestly
# at the frame stage (no stream), never at the model stage.
info "查询内置业务模板"
api GET "/api/algorithms/templates" >"$TMP/templates.json"
TPL_ID=$(jget "$TMP/templates.json" "d['data'][0]['id']")
[[ "$TPL_ID" =~ ^[0-9]+$ ]] || fail "未找到业务模板: $TPL_ID"

MODEL_FILE="$SOAK_MODEL"
if [[ "$REAL_MODE" != "1" ]]; then
  MODEL_FILE="$TMP/soak-placeholder.onnx"
  printf 'soak placeholder; real soak needs SOAK_MODEL' >"$MODEL_FILE"
fi

api GET "/api/algorithms/models" >"$TMP/models.json"
MODEL_ID=$(jget "$TMP/models.json" "next((m['id'] for m in d['data'] if m.get('code')=='grab_bucket_seg'), 0)")
[[ "$MODEL_ID" =~ ^[0-9]+$ ]] || fail "未找到抓斗分割模型: $MODEL_ID"

api GET "/api/algorithms/models" >"$TMP/models2.json"
SOAK_VID=$(jget "$TMP/models2.json" "next((v['id'] for m in d['data'] if m.get('code')=='grab_bucket_seg' for v in (m.get('versions') or []) if v.get('version')=='soak-1'), 0)" 2>/dev/null || echo 0)
if [[ ! "$SOAK_VID" =~ ^[1-9][0-9]*$ ]]; then
  api POST "/api/algorithms/model-versions" "{\"model_id\":$MODEL_ID,\"version\":\"soak-1\",\"adapter\":\"onnx\",\"file_path\":\"$MODEL_FILE\"}" >"$TMP/mv.json"
  SOAK_VID=$(jget "$TMP/mv.json" "d['data']['id']") || fail "创建模型版本失败：$(cat "$TMP/mv.json")"
fi
api POST "/api/algorithms/model-versions/$SOAK_VID/publish" "{}" >"$TMP/pub.json" || fail "发布模型版本失败：$(cat "$TMP/pub.json")"
# Point the template at the soak version (idempotent).
api PUT "/api/algorithms/templates/$TPL_ID" "{\"code\":\"grab_edge_guard\",\"name\":\"抓斗越界监测\",\"model_version_id\":$SOAK_VID}" >/dev/null 2>&1 || true

GEOM='{"kind":"polyline","pit_side":"below","buffer_norm":0.02,"points":[{"x":100,"y":400},{"x":640,"y":420},{"x":1180,"y":400}]}'

DEP_IDS=()
CAM_IDS=()
for i in $(seq 1 "$LANES"); do
  CODE="soak-cam-$i"
  if [[ "$REAL_MODE" == "1" ]]; then
    STREAM_URL="$SOAK_RTSP_URL"
  else
    STREAM_URL="rtsp://127.0.0.1:554/$i"
  fi
  # Reuse a camera left over from an earlier run so the script is repeatable.
  api GET "/api/cameras?keyword=$CODE" >"$TMP/find$i.json"
  CAM_ID=$(jget "$TMP/find$i.json" "d['data']['items'][0]['id'] if d['data'].get('items') else 0" 2>/dev/null || echo 0)
  if [[ ! "$CAM_ID" =~ ^[1-9][0-9]*$ ]]; then
    info "创建摄像头 $CODE"
    api POST "/api/cameras" "{\"code\":\"$CODE\",\"name\":\"压测 $i 路\",\"access_type\":\"rtsp\",\"main_url\":\"$STREAM_URL\"}" >"$TMP/cam$i.json"
    CAM_ID=$(jget "$TMP/cam$i.json" "d['data']['id']") || fail "创建摄像头失败：$(cat "$TMP/cam$i.json")"
    [[ "$CAM_ID" =~ ^[0-9]+$ ]] || fail "创建摄像头失败：$(cat "$TMP/cam$i.json")"
  else
    info "复用摄像头 $CODE (#$CAM_ID)"
    cp "$TMP/find$i.json" "$TMP/cam$i.json"
    # Re-enable in case a previous run disabled it to simulate a dropout,
    # and re-point the stream in case the mode changed between runs.
    api PUT "/api/cameras/$CAM_ID" "{\"code\":\"$CODE\",\"name\":\"压测 $i 路\",\"access_type\":\"rtsp\",\"enabled\":1,\"main_url\":\"$STREAM_URL\"}" >/dev/null
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
  # Start is refused unless self-check passes; treat that as a hard failure.
  api POST "/api/deployments/$DID/start" "{}" >"$TMP/start$i.json" || fail "布控启动被拒绝：$(cat "$TMP/start$i.json")"
  jget "$TMP/start$i.json" "1" >/dev/null || fail "布控启动被拒绝：$(cat "$TMP/start$i.json")"
  DEP_IDS+=("$DID")
  CAM_IDS+=("$CAM_ID")
done

# Observe heartbeats on every lane, plus the mode-specific analysis signal.
if [[ "$REAL_MODE" == "1" ]]; then MODE_LABEL="真实分析"; else MODE_LABEL="offline（fail-closed）"; fi
info "观察 ${DURATION}s：每一路都应持续心跳"
DEADLINE=$(( $(date +%s) + DURATION ))
while [[ $(date +%s) -lt $DEADLINE ]]; do
  sleep 10
  api GET "/api/reports/runtime" >"$TMP/runtime.json"
  SUMMARY=$(python3 - "$TMP/runtime.json" "${DEP_IDS[*]}" "$REAL_MODE" <<'PY'
import json,sys
data=json.load(open(sys.argv[1]))["data"]
want={int(x) for x in sys.argv[2].split()}
real=sys.argv[3]=="1"
rows=[r for r in data if r.get("deployment_id") in want]
hb=sum(1 for r in rows if r.get("heartbeat_at"))
analysing=0
for r in rows:
    age=r.get("frame_age_ms")
    if real:
        if isinstance(age,(int,float)) and age>=0 and r.get("last_result") in ("normal","near","cross"):
            analysing+=1
    else:
        if r.get("last_result")=="offline":
            analysing+=1
print(hb, analysing, len(rows))
PY
)
  read -r HB ANALYSING SEEN <<<"$SUMMARY"
  # Note: keep ASCII separators next to $vars — this sandbox's bash drops the
  # first byte of fullwidth punctuation that directly follows an expansion.
  echo "    心跳路数: $HB / $LANES | ${MODE_LABEL} 路数: $ANALYSING / $LANES"
  [[ "$HB" -ge "$LANES" ]] || fail "出现掉线：期望 $LANES 路心跳，实际 $HB 路"
  [[ "$ANALYSING" -ge "$LANES" ]] || fail "分析信号不符模式预期: $ANALYSING / $LANES"
done

# Simulate one lane losing its stream by disabling its camera. The remaining
# lanes must keep heartbeating.
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
[[ "$RUNNING" -ge $((LANES - 1)) ]] || fail "断流后其余路未继续运行: $RUNNING"

if [[ "$REAL_MODE" != "1" ]]; then
  # Fail-closed mode must not fabricate any events across all lanes.
  api GET "/api/events?page=1&page_size=100" >"$TMP/events.json"
  EVENTS=$(python3 - "$TMP/events.json" "${CAM_IDS[*]}" <<'PY'
import json,sys
items=json.load(open(sys.argv[1])).get("data",{}).get("items") or []
want={int(x) for x in sys.argv[2].split()}
print(sum(1 for e in items if e.get("camera_id") in want))
PY
)
  [[ "$EVENTS" == "0" ]] || fail "无帧模式下产生了 $EVENTS 个事件（禁止伪造）"
  info "零事件确认：无帧模式未产生任何告警"
fi

info "停止并清理布控"
for DID in "${DEP_IDS[@]}"; do
  api POST "/api/deployments/$DID/stop" "{}" >/dev/null
done

if [[ "$REAL_MODE" == "1" ]]; then
  info "四路压测通过（真实模式）：全部路数持续心跳并分析真实帧，单路断流不影响其余路"
else
  info "四路压测通过（fail-closed 模式）：全部路数持续心跳、状态 offline、零事件；真实帧长跑需 SOAK_RTSP_URL + SOAK_MODEL + onnx worker"
fi
