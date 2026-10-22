<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ApiError } from "@/api/client";
import * as api from "@/api/videoAi";
import type { AlgoModel, BusinessTemplate, RuleVersion } from "@/api/videoAi";
import { useToast } from "@/composables/useToast";

const toast = useToast();
const models = ref<AlgoModel[]>([]);
const rules = ref<RuleVersion[]>([]);
const templates = ref<BusinessTemplate[]>([]);

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
</script>

<template>
  <div class="page-header d-print-none"><div class="container-xl">
    <h2 class="page-title">算法管理</h2>
    <div class="text-secondary">基础模型 / 行为规则 / 业务模板。演示适配器为 mock，ONNX 可后续替换。</div>
  </div></div>
  <div class="page-body"><div class="container-xl">
    <div class="row g-3">
      <div class="col-lg-4">
        <div class="card"><div class="card-header"><h3 class="card-title">基础模型</h3></div>
          <div class="list-group list-group-flush">
            <div v-for="m in models" :key="m.id" class="list-group-item">
              <div class="fw-bold">{{ m.name }}</div>
              <div class="text-secondary">{{ m.code }} · {{ m.framework }}</div>
              <div v-for="v in m.versions || []" :key="v.id" class="small mt-1">{{ v.version }} · {{ v.status }} · {{ v.adapter }}</div>
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
  </div></div>
</template>
