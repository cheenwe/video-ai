<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { IconPlus, IconPencil, IconTrash, IconRefresh, IconRotate, IconList } from "@tabler/icons-vue";
import { ApiError } from "@/api/client";
import * as webhookApi from "@/api/notification";
import type { WebhookEndpoint, WebhookDelivery } from "@/api/notification";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";

const toast = useToast();

const endpoints = ref<WebhookEndpoint[]>([]);
const loading = ref(false);
const keyword = ref("");

// 前端搜索过滤（名称/URL/事件类型），端点列表未分页
const filteredEndpoints = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return endpoints.value;
  return endpoints.value.filter((ep) =>
    [ep.name, ep.url, ep.event_types.join(" ")].some((v) =>
      String(v || "").toLowerCase().includes(kw),
    ),
  );
});

function search() {
  // 端点列表为前端过滤，无需重新加载
}

const selectedId = ref<number | null>(null);
const deliveries = ref<WebhookDelivery[]>([]);
const deliveryTotal = ref(0);
const deliveryPage = ref(1);
const deliveryLoading = ref(false);

// 表单
const showForm = ref(false);
const editing = ref<WebhookEndpoint | null>(null);
const saving = ref(false);
const form = ref({
  name: "",
  url: "",
  secret: "",
  event_types: [] as string[],
  enabled: 1,
  timeout_sec: 10,
  max_retries: 5,
});

const eventTypeInput = ref("");

const eventTypePresets = [
  "user.create", "user.update", "user.delete",
  "dict.create", "dict.update", "dict.delete",
  "webhook.create", "webhook.update", "webhook.delete",
  "storage.create", "storage.update", "storage.delete",
  "schedule.job.create", "schedule.job.update", "schedule.job.run",
  "backup.create", "backup.restore",
];

async function loadEndpoints() {
  loading.value = true;
  try {
    const res = await webhookApi.listWebhooks();
    endpoints.value = res.items;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  form.value = { name: "", url: "", secret: "", event_types: [], enabled: 1, timeout_sec: 10, max_retries: 5 };
  eventTypeInput.value = "";
  showForm.value = true;
}

function openEdit(ep: WebhookEndpoint) {
  editing.value = ep;
  form.value = {
    name: ep.name,
    url: ep.url,
    secret: "",
    event_types: [...ep.event_types],
    enabled: ep.enabled,
    timeout_sec: ep.timeout_sec,
    max_retries: ep.max_retries,
  };
  eventTypeInput.value = "";
  showForm.value = true;
}

function addEventType() {
  const v = eventTypeInput.value.trim();
  if (v && !form.value.event_types.includes(v)) {
    form.value.event_types.push(v);
  }
  eventTypeInput.value = "";
}

function removeEventType(t: string) {
  form.value.event_types = form.value.event_types.filter((x) => x !== t);
}

async function save() {
  if (!form.value.name.trim() || !form.value.url.trim()) {
    toast.show("名称和 URL 不能为空");
    return;
  }
  saving.value = true;
  try {
    if (editing.value) {
      await webhookApi.updateWebhook(editing.value.id, {
        name: form.value.name,
        url: form.value.url,
        event_types: form.value.event_types,
        enabled: form.value.enabled,
        timeout_sec: form.value.timeout_sec,
        max_retries: form.value.max_retries,
        ...(form.value.secret ? { secret: form.value.secret } : {}),
      });
      toast.show("更新成功");
    } else {
      await webhookApi.createWebhook(form.value);
      toast.show("创建成功");
    }
    showForm.value = false;
    loadEndpoints();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

async function remove(ep: WebhookEndpoint) {
  if (!confirm(`确定删除 Webhook「${ep.name}」吗？`)) return;
  try {
    await webhookApi.deleteWebhook(ep.id);
    toast.show("已删除");
    if (selectedId.value === ep.id) {
      selectedId.value = null;
      deliveries.value = [];
    }
    loadEndpoints();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

async function selectEndpoint(id: number) {
  selectedId.value = id;
  deliveryPage.value = 1;
  loadDeliveries();
}

async function loadDeliveries() {
  if (!selectedId.value) return;
  deliveryLoading.value = true;
  try {
    const res = await webhookApi.listWebhookDeliveries(selectedId.value, {
      page: deliveryPage.value,
      page_size: 10,
    });
    deliveries.value = res.items;
    deliveryTotal.value = res.total;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    deliveryLoading.value = false;
  }
}

async function retry(d: WebhookDelivery) {
  try {
    await webhookApi.retryWebhookDelivery(d.id);
    toast.show("已重新投递");
    loadDeliveries();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "操作失败");
  }
}

function statusLabel(s: string) {
  const m: Record<string, string> = { pending: "待处理", success: "成功", failed: "失败" };
  return m[s] || s;
}

function statusClass(s: string) {
  if (s === "success") return "bg-green-lt";
  if (s === "failed") return "bg-red-lt";
  return "bg-secondary-lt";
}

onMounted(loadEndpoints);
</script>

<template>
  <div>
    <div class="d-flex align-items-center justify-content-between gap-2 mb-4">
      <div>
        <h2>Webhook 管理</h2>
        <p class="text-secondary mb-0">配置事件 Webhook 投递，支持签名校验与失败重试</p>
      </div>
    </div>

    <div class="row g-4">
      <!-- 左：端点列表 -->
      <div class="col-lg-5">
        <div class="card">
          <div class="card-header d-flex align-items-center justify-content-between gap-2">
            <h3 class="card-title mb-0">投递端点</h3>
            <div class="d-flex gap-2">
              <input
                v-model="keyword"
                class="form-control form-control-sm"
                placeholder="搜索..."
                style="max-width: 12rem"
                @keyup.enter="search"
              />
              <button class="btn btn-primary btn-sm" @click="openCreate" v-permission="['webhook:create']">
                <IconPlus :size="16" class="me-1" />新建
              </button>
            </div>
          </div>
          <div class="card-body">
            <div v-if="loading" class="text-secondary text-center py-3">加载中…</div>
            <div v-else-if="!endpoints.length" class="text-secondary text-center py-3">暂无 Webhook</div>
            <div v-else-if="!filteredEndpoints.length" class="text-secondary text-center py-3">未找到匹配的 Webhook</div>
            <div v-else class="list-group list-group-flush">
              <button
                v-for="ep in filteredEndpoints"
                :key="ep.id"
                class="list-group-item list-group-item-action d-flex align-items-center justify-content-between"
                :class="{ active: selectedId === ep.id }"
                @click="selectEndpoint(ep.id)"
              >
                <div class="text-start">
                  <div class="fw-semibold">{{ ep.name }}</div>
                  <div class="small text-secondary text-truncate" style="max-width: 280px">{{ ep.url }}</div>
                  <div class="small text-secondary mt-1">
                    <span class="badge" :class="ep.enabled ? 'bg-green-lt' : 'bg-secondary-lt'">
                      {{ ep.enabled ? '启用' : '停用' }}
                    </span>
                    <span class="ms-2">{{ ep.event_types.length ? ep.event_types.length + ' 个事件' : '全部事件' }}</span>
                  </div>
                </div>
                <div class="d-flex gap-1" @click.stop>
                  <button class="btn btn-sm btn-icon" title="编辑" @click="openEdit(ep)" v-permission="['webhook:update']">
                    <IconPencil :size="15" />
                  </button>
                  <button class="btn btn-sm btn-icon text-danger" title="删除" @click="remove(ep)" v-permission="['webhook:delete']">
                    <IconTrash :size="15" />
                  </button>
                </div>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 右：投递记录 -->
      <div class="col-lg-7">
        <div class="card">
          <div class="card-header d-flex align-items-center justify-content-between">
            <h3 class="card-title mb-0">
              <IconList :size="18" class="me-1" />投递记录
            </h3>
            <button class="btn btn-sm btn-ghost-secondary" :disabled="!selectedId" @click="loadDeliveries">
              <IconRefresh :size="15" />
            </button>
          </div>
          <div class="card-body">
            <div v-if="!selectedId" class="text-secondary text-center py-5">请选择左侧端点</div>
            <div v-else-if="deliveryLoading" class="text-secondary text-center py-3">加载中…</div>
            <div v-else-if="!deliveries.length" class="text-secondary text-center py-3">暂无投递记录</div>
            <div v-else class="table-responsive">
              <table class="table table-sm align-middle">
                <thead>
                  <tr>
                    <th>事件</th>
                    <th>状态</th>
                    <th>HTTP</th>
                    <th>重试</th>
                    <th>时间</th>
                    <th style="width: 80px"></th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="d in deliveries" :key="d.id">
                    <td><code class="small">{{ d.event_type }}</code></td>
                    <td>
                      <span class="badge" :class="statusClass(d.status)">{{ statusLabel(d.status) }}</span>
                    </td>
                    <td>{{ d.status_code || '—' }}</td>
                    <td>{{ d.retry_count }}</td>
                    <td class="text-secondary small">{{ new Date(d.created_at || '').toLocaleString() }}</td>
                    <td>
                      <button
                        v-if="d.status === 'failed'"
                        class="btn btn-sm btn-icon"
                        title="重试"
                        @click="retry(d)"
                       v-permission="['webhook:update']">
                        <IconRotate :size="15" />
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
              <CdPagination
                v-if="deliveryTotal > 10"
                :page="deliveryPage"
                :page-size="10"
                :total="deliveryTotal"
                @update:page="(p) => (deliveryPage = p)"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 表单弹窗 -->
    <CdModal :show="showForm" :title="editing ? '编辑 Webhook' : '新建 Webhook'" size="lg" @close="showForm = false">
      <div class="modal-body">
        <div class="mb-3">
          <label class="form-label">名称 <span class="text-danger">*</span></label>
          <input v-model="form.name" class="form-control" placeholder="如 告警推送" />
        </div>
        <div class="mb-3">
          <label class="form-label">URL <span class="text-danger">*</span></label>
          <input v-model="form.url" class="form-control" placeholder="https://example.com/webhook" />
        </div>
        <div class="mb-3">
          <label class="form-label">签名密钥（可选）</label>
          <input v-model="form.secret" type="password" class="form-control" placeholder="用于 HMAC-SHA256 签名" />
          <div class="form-hint">{{ editing ? '留空则不修改现有密钥' : '设置后将在请求头 X-Signature 中携带 sha256=签名' }}</div>
        </div>
        <div class="mb-3">
          <label class="form-label">事件类型（空=全部）</label>
          <div class="d-flex gap-2 mb-2">
            <input v-model="eventTypeInput" class="form-control form-control-sm" placeholder="如 user.create" @keyup.enter="addEventType" />
            <button class="btn btn-sm btn-outline-secondary" @click="addEventType">添加</button>
          </div>
          <div class="d-flex flex-wrap gap-1 mb-2">
            <span v-for="t in form.event_types" :key="t" class="badge bg-blue-lt">
              {{ t }}
              <button class="btn btn-sm p-0 ms-1 text-danger" @click="removeEventType(t)">×</button>
            </span>
          </div>
          <div class="small text-secondary">
            常用类型：
            <button
              v-for="t in eventTypePresets"
              :key="t"
              class="btn btn-sm btn-link p-0 me-2"
              @click="() => { if (!form.event_types.includes(t)) form.event_types.push(t) }"
            >{{ t }}</button>
          </div>
        </div>
        <div class="row g-3">
          <div class="col-4">
            <label class="form-label">状态</label>
            <select v-model.number="form.enabled" class="form-select">
              <option :value="1">启用</option>
              <option :value="0">停用</option>
            </select>
          </div>
          <div class="col-4">
            <label class="form-label">超时（秒）</label>
            <input v-model.number="form.timeout_sec" type="number" min="1" class="form-control" />
          </div>
          <div class="col-4">
            <label class="form-label">最大重试</label>
            <input v-model.number="form.max_retries" type="number" min="0" max="10" class="form-control" />
          </div>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" @click="showForm = false">取消</button>
        <button class="btn btn-primary" :disabled="saving" @click="save">
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </div>
    </CdModal>
  </div>
</template>
