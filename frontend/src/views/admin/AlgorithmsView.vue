<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { AlgoModel, BusinessTemplate, RuleVersion } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";

const toast = useToast();
const models = ref<AlgoModel[]>([]);
const rules = ref<RuleVersion[]>([]);
const templates = ref<BusinessTemplate[]>([]);

const show = ref(false);
const saving = ref(false);
const form = ref({ model_id: 0, version: "", adapter: "mock", backend: "cpu", file_path: "", checksum: "" });
const checking = ref<Record<number, boolean>>({});
const publishing = ref<Record<number, boolean>>({});

type SelfCheckReport = { ok: boolean; adapter: string; demo: boolean; checks: { name: string; ok: boolean; detail: string }[] };
const report = ref<SelfCheckReport | null>(null);

async function load() {
  try {
    const [m, r, t] = await Promise.all([api.listModels(), api.listRules(), api.listTemplates()]);
    models.value = m || [];
    rules.value = r || [];
    templates.value = t || [];
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  }
}
onMounted(load);

function openCreateVersion(m: AlgoModel) {
  form.value = { model_id: m.id, version: "", adapter: "mock", backend: "cpu", file_path: "", checksum: "" };
  show.value = true;
}

function adapterChanged() {
  if (form.value.adapter === "mock") form.value.file_path = "";
}

async function saveVersion() {
  if (!form.value.version.trim()) {
    toast.show("请填写版本号");
    return;
  }
  if (form.value.adapter === "onnx" && !form.value.file_path.trim()) {
    toast.show("ONNX 版本需要填写模型文件路径");
    return;
  }
  saving.value = true;
  try {
    await api.createModelVersion({ ...form.value, version: form.value.version.trim() });
    show.value = false;
    toast.show("版本已创建（草稿），发布后布控才会加载");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "创建失败");
  } finally {
    saving.value = false;
  }
}

async function publish(v: { id: number; version: string }) {
  publishing.value[v.id] = true;
  try {
    await api.publishModelVersion(v.id);
    toast.show("已发布");
    await load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "发布失败");
  } finally {
    publishing.value[v.id] = false;
  }
}

async function selfCheck(v: { id: number; version: string }) {
  checking.value[v.id] = true;
  report.value = null;
  try {
    const res = await api.selfCheckModelVersion(v.id);
    report.value = res;
    if (!res.ok) toast.show("自检未通过，见报告详情");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "自检失败");
  } finally {
    checking.value[v.id] = false;
  }
}
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">算法管理</h2>
    <div class="text-secondary">基础模型 / 行为规则 / 业务模板。mock 为演示适配器；ONNX 版本需真实推理运行时接入后才会通过自检。</div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row g-3">
      <div class="col-lg-4">
        <div class="card"><div class="card-header"><h3 class="card-title">基础模型</h3></div>
          <div class="list-group list-group-flush">
            <div v-for="m in models" :key="m.id" class="list-group-item">
              <div class="d-flex justify-content-between align-items-start">
                <div>
                  <div class="fw-bold">{{ m.name }}</div>
                  <div class="text-secondary">{{ m.code }} · {{ m.framework }}</div>
                </div>
                <button class="btn btn-sm btn-outline-primary" v-permission="'algorithm:publish'" @click="openCreateVersion(m)">新建版本</button>
              </div>
              <div v-for="v in m.versions || []" :key="v.id" class="small mt-2 border-top pt-1">
                <div class="d-flex justify-content-between align-items-center">
                  <span>{{ v.version }} · <span class="badge" :class="v.status === 'published' ? 'bg-success' : 'bg-secondary'">{{ v.status }}</span> · {{ v.adapter }}</span>
                  <span>
                    <button class="btn btn-sm btn-link p-0 me-2" :disabled="checking[v.id]" @click="selfCheck(v)">自检</button>
                    <button v-if="v.status !== 'published'" class="btn btn-sm btn-link p-0" :disabled="publishing[v.id]" v-permission="'algorithm:publish'" @click="publish(v)">发布</button>
                  </span>
                </div>
                <div v-if="v.file_path" class="text-secondary text-truncate" :title="v.file_path">{{ v.file_path }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="col-lg-4">
        <div class="card"><div class="card-header"><h3 class="card-title">行为规则</h3></div>
          <div class="list-group list-group-flush">
            <div v-for="r in rules" :key="r.id" class="list-group-item">
              <div class="fw-bold">{{ r.name }}</div>
              <div class="text-secondary">{{ r.code }} @ {{ r.version }}</div>
              <div class="small">{{ r.description }}</div>
            </div>
          </div>
        </div>
      </div>
      <div class="col-lg-4">
        <div class="card"><div class="card-header"><h3 class="card-title">业务模板</h3></div>
          <div class="list-group list-group-flush">
            <div v-for="t in templates" :key="t.id" class="list-group-item">
              <div class="fw-bold">{{ t.name }}</div>
              <div class="text-secondary">{{ t.code }} @ {{ t.version }} · {{ t.status }}</div>
              <div class="small">{{ t.description }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="report" class="card mt-3"><div class="card-header d-flex justify-content-between">
      <h3 class="card-title">版本自检报告（{{ report.adapter }}）</h3>
      <button class="btn btn-sm btn-link" @click="report = null">关闭</button>
    </div>
      <div class="card-body">
        <table class="table table-sm">
          <thead><tr><th>检查项</th><th>结果</th><th>详情</th></tr></thead>
          <tbody>
            <tr v-for="ck in report.checks" :key="ck.name">
              <td>{{ ck.name }}</td>
              <td><span class="badge" :class="ck.ok ? 'bg-success' : 'bg-danger'">{{ ck.ok ? "通过" : "失败" }}</span></td>
              <td class="text-secondary">{{ ck.detail }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div></div>

  <CdModal :show="show" title="新建模型版本" @close="show = false">
    <div class="modal-body">
      <div class="mb-2"><label class="form-label">版本号</label><input v-model="form.version" class="form-control" placeholder="如 1.0.0" /></div>
      <div class="mb-2"><label class="form-label">适配器</label>
        <select v-model="form.adapter" class="form-select" @change="adapterChanged">
          <option value="mock">mock（演示）</option>
          <option value="onnx">onnx（需推理运行时）</option>
        </select>
      </div>
      <template v-if="form.adapter === 'onnx'">
        <div class="mb-2"><label class="form-label">模型文件路径</label><input v-model="form.file_path" class="form-control" placeholder="服务端可访问的 .onnx 路径" /></div>
        <div class="mb-2"><label class="form-label">校验和（可选，SHA-256）</label><input v-model="form.checksum" class="form-control" /></div>
      </template>
      <div class="alert alert-warning small mb-0" v-if="form.adapter === 'onnx'">
        当前未接入 ONNX Runtime / GPU Worker，onnx 版本可以登记与发布，但布控运行会报「推理适配器未接入」，自检将失败。
      </div>
    </div>
    <template #footer>
      <button class="btn" @click="show = false">取消</button>
      <button class="btn btn-primary" :disabled="saving" @click="saveVersion">保存</button>
    </template>
  </CdModal>
</template>
