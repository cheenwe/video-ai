<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch, nextTick } from "vue";
import Hls from "hls.js";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { Camera } from "@/api/videoAi";
import { liveURL } from "@/api/nvr";
import { useToast } from "@/composables/useToast";

const toast = useToast();
const cameras = ref<Camera[]>([]);
const selected = ref<number[]>([]);
const slots = computed(() => {
  const ids = selected.value.slice(0, 4);
  while (ids.length < 4) ids.push(0);
  return ids;
});

const players = ref<(HTMLVideoElement | null)[]>([]);
const canvases = ref<(HTMLCanvasElement | null)[]>([]);
const hlsMap = new Map<number, Hls>();

// Live analysis state pushed over WebSocket, keyed by camera.
const overlays = ref<Record<number, any>>({});
const results = ref<Record<number, string>>({});
const liveEvents = ref<any[]>([]);
let ws: WebSocket | null = null;
let reconnectTimer: number | undefined;

function camById(id: number) {
  return cameras.value.find((c) => c.id === id);
}

function isNvrCam(c?: Camera) {
  return !!c?.nvr_agent_id && !!c?.agent_camera_id;
}

async function load() {
  try {
    const res = await api.listCameras({ page: 1, page_size: 200 });
    // Every camera is selectable: NVR-bound ones use HLS, the rest use the
    // server-side puller (MJPEG) instead of being filtered out.
    cameras.value = (res.items || []).filter((c) => !c.archived);
    if (!selected.value.length) {
      selected.value = cameras.value.slice(0, 4).map((c) => c.id);
    }
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}

function setSlot(idx: number, cameraId: number) {
  const next = [...selected.value];
  next[idx] = cameraId;
  selected.value = next;
}

function attachVideo(idx: number, el: HTMLVideoElement | null) {
  players.value[idx] = el;
  playSlot(idx);
}

function attachCanvas(idx: number, el: HTMLCanvasElement | null) {
  canvases.value[idx] = el;
}

function playSlot(idx: number) {
  const el = players.value[idx];
  const cam = camById(slots.value[idx]);
  const old = hlsMap.get(idx);
  if (old) { old.destroy(); hlsMap.delete(idx); }
  if (!el) return;
  if (!isNvrCam(cam)) {
    el.removeAttribute("src");
    return;
  }
  const url = liveURL(cam!.nvr_agent_id!, cam!.agent_camera_id!);
  if (Hls.isSupported()) {
    const hls = new Hls({ enableWorker: false });
    hls.loadSource(url);
    hls.attachMedia(el);
    hlsMap.set(idx, hls);
  } else if (el.canPlayType("application/vnd.apple.mpegurl")) {
    el.src = url;
  }
}

// ------------------------------------------------------------ overlay ----

const resultColor: Record<string, string> = {
  normal: "#2fb344", near: "#f59f00", cross: "#d63939",
  unknown: "#626976", offline: "#868e96",
};

function drawOverlay(idx: number) {
  const cv = canvases.value[idx];
  if (!cv) return;
  const box = cv.parentElement;
  if (!box) return;
  const w = box.clientWidth;
  const h = box.clientHeight;
  const dpr = window.devicePixelRatio || 1;
  if (cv.width !== Math.round(w * dpr) || cv.height !== Math.round(h * dpr)) {
    cv.width = Math.round(w * dpr);
    cv.height = Math.round(h * dpr);
  }
  const ctx = cv.getContext("2d");
  if (!ctx) return;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);

  const camId = slots.value[idx];
  const ov = overlays.value[camId];
  const result = results.value[camId];
  if (!ov) return;

  const rw = Number(ov.ref_width) || 1280;
  const rh = Number(ov.ref_height) || 720;
  const sx = w / rw;
  const sy = h / rh;

  // Pit-edge boundary.
  const pts: { x: number; y: number }[] = ov.boundary || [];
  if (pts.length > 1) {
    ctx.strokeStyle = "#f59f00";
    ctx.lineWidth = 3;
    ctx.beginPath();
    pts.forEach((p, i) => {
      const x = p.x * sx;
      const y = p.y * sy;
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    });
    ctx.stroke();
  }

  // Detected grab bucket contour.
  const contour: { x: number; y: number }[] = ov.contour || [];
  if (contour.length > 2) {
    ctx.strokeStyle = resultColor[result] || "#206bc4";
    ctx.lineWidth = 2;
    ctx.beginPath();
    contour.forEach((p, i) => {
      const x = p.x * sx;
      const y = p.y * sy;
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    });
    ctx.closePath();
    ctx.stroke();
  }

  // Centroid marker.
  const c = ov.centroid;
  if (c) {
    ctx.fillStyle = resultColor[result] || "#206bc4";
    ctx.beginPath();
    ctx.arc(c.x * sx, c.y * sy, 5, 0, Math.PI * 2);
    ctx.fill();
  }
}

function redrawAll() {
  for (let i = 0; i < 4; i++) drawOverlay(i);
}

// --------------------------------------------------------- websocket ----

function connectWS() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
  try {
    ws = new WebSocket(api.videoWSURL());
  } catch {
    return;
  }
  ws.onmessage = (ev) => {
    try {
      const u = JSON.parse(ev.data);
      if (!u || !u.camera_id) return;
      if (u.overlay) overlays.value[u.camera_id] = u.overlay;
      if (u.result) results.value[u.camera_id] = u.result;
      if (u.type === "event") {
        liveEvents.value.unshift({ ...u, at: u.ts || new Date().toISOString() });
        if (liveEvents.value.length > 30) liveEvents.value.pop();
      }
      nextTick(redrawAll);
    } catch {
      /* ignore malformed frame */
    }
  };
  ws.onclose = () => {
    ws = null;
    window.clearTimeout(reconnectTimer);
    reconnectTimer = window.setTimeout(connectWS, 3000);
  };
  ws.onerror = () => { try { ws?.close(); } catch { /* noop */ } };
}

function fmtTime(iso?: string) {
  if (!iso) return "-";
  const d = new Date(iso);
  return d.toLocaleTimeString();
}

const resultLabel: Record<string, string> = {
  normal: "正常", near: "临边", cross: "越界", unknown: "不可判定", offline: "离线",
};

watch(slots, () => { for (let i = 0; i < 4; i++) playSlot(i); }, { deep: true });
watch([overlays, results], () => nextTick(redrawAll), { deep: true });

let resizeObserver: ResizeObserver | null = null;

onMounted(async () => {
  await load();
  connectWS();
  resizeObserver = new ResizeObserver(() => redrawAll());
  document.querySelectorAll(".live-stage").forEach((el) => resizeObserver!.observe(el));
  window.addEventListener("resize", redrawAll);
});

onUnmounted(() => {
  hlsMap.forEach((h) => h.destroy());
  hlsMap.clear();
  window.clearTimeout(reconnectTimer);
  window.removeEventListener("resize", redrawAll);
  resizeObserver?.disconnect();
  try { ws?.close(); } catch { /* noop */ }
});
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">实时监控</h2>
    <div class="text-secondary">
      四宫格预览：已绑定 NVR 通道走 HLS，直连 RTSP 走服务端取流（MJPEG）。分析轮廓与事件经 WebSocket 实时叠加。
    </div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row g-3">
      <div class="col-lg-9">
        <div class="row g-2">
          <div v-for="(id, idx) in slots" :key="idx" class="col-md-6">
            <div class="card">
              <div class="card-header py-2">
                <select class="form-select form-select-sm" :value="id"
                  @change="setSlot(idx, Number(($event.target as HTMLSelectElement).value))">
                  <option :value="0">未选择</option>
                  <option v-for="c in cameras" :key="c.id" :value="c.id">{{ c.name }} ({{ c.code }})</option>
                </select>
              </div>
              <div class="card-body p-0 bg-dark position-relative live-stage" style="aspect-ratio:16/9">
                <video v-if="isNvrCam(camById(id))"
                  :ref="(el) => attachVideo(idx, el as HTMLVideoElement)"
                  class="w-100 h-100" style="object-fit:contain" muted autoplay playsinline controls />
                <img v-else-if="id"
                  :src="api.cameraPreviewURL(id)" class="w-100 h-100" style="object-fit:contain" alt="实时画面" />
                <div v-else class="d-flex align-items-center justify-content-center h-100 text-secondary">未选择摄像头</div>
                <canvas :ref="(el) => attachCanvas(idx, el as HTMLCanvasElement)"
                  class="position-absolute top-0 start-0 w-100 h-100" style="pointer-events:none" />
                <span v-if="results[id]" class="badge position-absolute top-0 end-0 m-2"
                  :style="{ background: resultColor[results[id]] || '#626976' }">
                  {{ resultLabel[results[id]] || results[id] }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="col-lg-3">
        <div class="card">
          <div class="card-header"><h3 class="card-title">实时事件</h3></div>
          <div class="list-group list-group-flush" style="max-height: 60vh; overflow:auto">
            <div v-if="!liveEvents.length" class="list-group-item text-secondary small">
              暂无实时事件。启动布控后，越界/临边事件会在此即时出现。
            </div>
            <div v-for="(e, i) in liveEvents" :key="i" class="list-group-item">
              <div class="d-flex justify-content-between">
                <span class="fw-bold small">{{ resultLabel[e.result] || e.result }}</span>
                <span class="text-secondary small">{{ fmtTime(e.at) }}</span>
              </div>
              <div class="text-secondary small">
                摄像头 #{{ e.camera_id }}<span v-if="e.camera_name"> · {{ e.camera_name }}</span>
              </div>
              <div v-if="e.reason" class="small text-truncate" :title="e.reason">{{ e.reason }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div></div>
</template>
