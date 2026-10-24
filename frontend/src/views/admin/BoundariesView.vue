<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { BoundaryVersion, Camera } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";

const toast = useToast();
const cameras = ref<Camera[]>([]);
const cameraId = ref(0);
const items = ref<BoundaryVersion[]>([]);
const pointsText = ref("100,400\n640,420\n1180,400");
// Reference frame the polyline is expressed in. An AI candidate overwrites
// these with the real frame size it inferred on, so the geometry is not
// silently rescaled against a guessed 1280x720.
const refW = ref(1280);
const refH = ref(720);
const pitSide = ref<"above" | "below">("below");

async function loadCams() {
  const res = await api.listCameras({ page: 1, page_size: 100 });
  cameras.value = res.items || [];
  if (!cameraId.value && cameras.value.length) cameraId.value = cameras.value[0].id;
}

async function load() {
  if (!cameraId.value) return;
  try {
    items.value = await api.listBoundaries(cameraId.value);
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}

const candidateInfo = ref("");

async function candidate() {
  // The endpoint infers on a fresh live frame with a published model and
  // returns a suggested polyline. When there is no frame, no published model,
  // or the model cannot see the pit edge, it fails with the reason — that
  // reason is surfaced verbatim instead of being swallowed.
  try {
    const res = await api.boundaryCandidate(cameraId.value);
    const pts = res.geometry?.points || [];
    if (pts.length < 2) {
      toast.show("候选未返回有效折线");
      return;
    }
    refW.value = res.ref_width || refW.value;
    refH.value = res.ref_height || refH.value;
    if (res.geometry.pit_side) pitSide.value = res.geometry.pit_side;
    pointsText.value = pts.map((p) => `${Math.round(p.x)},${Math.round(p.y)}`).join("\n");
    candidateInfo.value = `候选来自模型 #${res.model_version_id}，坑边目标 ${res.edge_detections} 个，平均置信度 ${(res.confidence * 100).toFixed(0)}%；请核对后保存草稿`;
    toast.show("已获取候选折线，请核对后保存草稿");
  } catch (e) {
    candidateInfo.value = "";
    toast.show(e instanceof ApiError ? e.message : "获取候选失败，请手动绘制");
  }
}

async function createDraft() {
  const points = pointsText.value.trim().split(/\n+/).map((line) => {
    const [x, y] = line.split(",").map(Number);
    return { x, y };
  });
  try {
    await api.createBoundary({
      camera_id: cameraId.value,
      name: "draft",
      ref_width: refW.value,
      ref_height: refH.value,
      geometry: { kind: "polyline", pit_side: pitSide.value, buffer_norm: 0.02, points },
    });
    toast.show("已创建草稿");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  }
}

async function publish(id: number) {
  try {
    await api.publishBoundary(id);
    toast.show("已发布");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "发布失败");
  }
}

async function rollback(id: number) {
  try {
    await api.rollbackBoundary(id);
    toast.show("已生成回滚草稿");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "回滚失败");
  }
}

onMounted(async () => { await loadCams(); await load(); });
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">坑边管理</h2>
    <div class="text-secondary">边界版本：草稿 / 已发布 / 失效 / 归档。发布前校验点数与自交。</div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row g-3">
      <div class="col-lg-5">
        <div class="card">
          <div class="card-body">
            <label class="form-label">摄像头</label>
            <select v-model.number="cameraId" class="form-select mb-2" @change="load">
              <option v-for="c in cameras" :key="c.id" :value="c.id">{{ c.name }}</option>
            </select>
            <label class="form-label">折线点（x,y 每行一个，参考 {{ refW }}×{{ refH }}）</label>
            <textarea v-model="pointsText" class="form-control mb-2" rows="6" />
            <div class="row g-2 mb-2">
              <div class="col-4">
                <label class="form-label">参考宽</label>
                <input v-model.number="refW" type="number" class="form-control" />
              </div>
              <div class="col-4">
                <label class="form-label">参考高</label>
                <input v-model.number="refH" type="number" class="form-control" />
              </div>
              <div class="col-4">
                <label class="form-label">坑侧</label>
                <select v-model="pitSide" class="form-select">
                  <option value="below">下方</option>
                  <option value="above">上方</option>
                </select>
              </div>
            </div>
            <div class="btn-list">
              <button class="btn" v-permission="'boundary:edit'" @click="candidate"
                title="用已发布模型在当前实时帧上推理，返回候选折线；无帧/无模型/模型无坑边类时会明确报错">AI 候选</button>
              <button class="btn btn-primary" v-permission="'boundary:edit'" @click="createDraft">保存草稿</button>
            </div>
            <div v-if="candidateInfo" class="text-secondary small mt-2">{{ candidateInfo }}</div>
          </div>
        </div>
      </div>
      <div class="col-lg-7">
        <div class="card">
          <div class="table-responsive">
            <table class="table table-vcenter card-table">
              <thead><tr><th>版本</th><th>状态</th><th>来源</th><th></th></tr></thead>
              <tbody>
                <tr v-for="b in items" :key="b.id">
                  <td>v{{ b.version }} {{ b.name }}</td>
                  <td><span class="badge">{{ b.status }}</span></td>
                  <td>{{ b.source }}</td>
                  <td class="text-end">
                    <button class="btn btn-sm" v-permission="'boundary:publish'" :disabled="b.status==='published'" @click="publish(b.id)">发布</button>
                    <button class="btn btn-sm ms-1" v-permission="'boundary:edit'" @click="rollback(b.id)">回滚</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div></div>
</template>
