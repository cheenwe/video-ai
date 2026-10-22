<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
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
const hlsMap = new Map<number, Hls>();

async function load() {
  try {
    const res = await api.listCameras({ page: 1, page_size: 100 });
    cameras.value = (res.items || []).filter((c) => c.nvr_agent_id && c.agent_camera_id);
    if (!selected.value.length) {
      selected.value = cameras.value.slice(0, 4).map((c) => c.id);
    }
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}

function camById(id: number) {
  return cameras.value.find((c) => c.id === id);
}

function setSlot(idx: number, cameraId: number) {
  const next = [...selected.value];
  next[idx] = cameraId;
  selected.value = next;
}

function attach(idx: number, el: HTMLVideoElement | null) {
  players.value[idx] = el;
  playSlot(idx);
}

function playSlot(idx: number) {
  const el = players.value[idx];
  const cam = camById(slots.value[idx]);
  const old = hlsMap.get(idx);
  if (old) { old.destroy(); hlsMap.delete(idx); }
  if (!el || !cam?.nvr_agent_id || !cam.agent_camera_id) {
    if (el) el.removeAttribute("src");
    return;
  }
  const url = liveURL(cam.nvr_agent_id, cam.agent_camera_id);
  if (Hls.isSupported()) {
    const hls = new Hls({ enableWorker: false });
    hls.loadSource(url);
    hls.attachMedia(el);
    hlsMap.set(idx, hls);
  } else if (el.canPlayType("application/vnd.apple.mpegurl")) {
    el.src = url;
  }
}

watch(slots, () => { for (let i = 0; i < 4; i++) playSlot(i); }, { deep: true });
onMounted(load);
onUnmounted(() => { hlsMap.forEach((h) => h.destroy()); hlsMap.clear(); });
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">实时监控</h2>
    <div class="text-secondary">四宫格预览（经平台代理 HLS）。分析叠加见布控运行状态与告警中心。</div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row g-2">
      <div v-for="(id, idx) in slots" :key="idx" class="col-md-6">
        <div class="card">
          <div class="card-header py-2">
            <select class="form-select form-select-sm" :value="id" @change="setSlot(idx, Number(($event.target as HTMLSelectElement).value))">
              <option :value="0">未选择</option>
              <option v-for="c in cameras" :key="c.id" :value="c.id">{{ c.name }} ({{ c.code }})</option>
            </select>
          </div>
          <div class="card-body p-0 bg-dark" style="aspect-ratio:16/9">
            <video :ref="(el) => attach(idx, el as HTMLVideoElement)" class="w-100 h-100" style="object-fit:contain" muted autoplay playsinline controls />
          </div>
        </div>
      </div>
    </div>
  </div></div>
</template>
