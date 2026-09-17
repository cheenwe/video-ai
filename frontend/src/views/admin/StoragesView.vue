<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import {
  IconPlus,
  IconPencil,
  IconTrash,
  IconFolder,
  IconFolderOpen,
  IconFile,
  IconPlugConnected,
  IconArrowLeft,
} from "@tabler/icons-vue";
import { ApiError, fmtDate, fmtSize } from "@/api/client";
import * as storagesApi from "@/api/storages";
import type {
  StorageRow,
  StoragePayload,
  DriverInfo,
  ConfigSchema,
  ConfigSchemaField,
  BrowseItem,
} from "@/api/storages";
import { useToast } from "@/composables/useToast";
import { usePageSize } from "@/composables/usePageSize";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";

const toast = useToast();
const { pageSize, ensurePageSize } = usePageSize();

const items = ref<StorageRow[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const keyword = ref("");

const drivers = ref<DriverInfo[]>([]);
const configSchema = ref<ConfigSchema>({});

const driverLabels: Record<string, string> = {
  local: "本地存储",
  s3: "S3 / 对象存储",
  oss: "阿里云 OSS",
  ftp: "FTP",
  sftp: "SFTP",
  webdav: "WebDAV",
  nfs: "NFS",
  smb: "SMB",
  onedrive: "OneDrive",
  google_drive: "Google Drive",
  baidu_pan: "百度网盘",
};

function driverLabel(driver: string): string {
  return driverLabels[driver] || driver;
}

// 前端搜索过滤（名称/驱动类型/根路径/备注），保持分页逻辑不变
const filteredItems = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return items.value;
  return items.value.filter((s) =>
    [s.name, driverLabel(s.driver), s.root_path, s.remark].some((v) =>
      String(v || "").toLowerCase().includes(kw),
    ),
  );
});

function search() {
  page.value = 1;
}

const driverOptions = computed(() =>
  (drivers.value.length
    ? drivers.value
    : Object.keys(driverLabels).map((d) => ({
        driver: d,
        name: driverLabels[d],
        cloud: false,
      }))
  ).map((d) => ({
    driver: d.driver,
    label: driverLabels[d.driver] || d.name || d.driver,
    cloud: d.cloud,
  })),
);

// 表单弹窗（新建 / 编辑共用）
const showForm = ref(false);
const editing = ref<StorageRow | null>(null);
const saving = ref(false);
const testingForm = ref(false);
const submitted = ref(false);
const form = ref({
  name: "",
  driver: "local",
  root_path: "/",
  sort_order: 0,
  remark: "",
  enabled: true,
  config: {} as Record<string, unknown>,
});

const currentFields = computed<ConfigSchemaField[]>(
  () => configSchema.value[form.value.driver] || [],
);

function resetForm() {
  form.value = {
    name: "",
    driver: "local",
    root_path: "/",
    sort_order: 0,
    remark: "",
    enabled: true,
    config: {},
  };
  editing.value = null;
  submitted.value = false;
}

function openCreate() {
  resetForm();
  showForm.value = true;
}

function openEdit(s: StorageRow) {
  editing.value = s;
  form.value = {
    name: s.name,
    driver: s.driver || "local",
    root_path: s.root_path || "/",
    sort_order: s.sort_order || 0,
    remark: s.remark || "",
    enabled: s.enabled,
    config: { ...(s.config || {}) },
  };
  submitted.value = false;
  showForm.value = true;
}

// 切换驱动类型时清空配置（仅在用户交互时触发，避免回填时被清空）
function onDriverChange() {
  form.value.config = {};
}

// 配置字段访问器（避免对 unknown 使用 v-model）
function configStr(key: string): string {
  const v = form.value.config[key];
  return v == null ? "" : String(v);
}
function configNum(key: string): number {
  const n = Number(form.value.config[key]);
  return Number.isFinite(n) ? n : 0;
}
function configBool(key: string): boolean {
  const v = form.value.config[key];
  return v === true || v === "true";
}
function setConfig(key: string, val: unknown) {
  form.value.config = { ...form.value.config, [key]: val };
}

// 事件绑定辅助：把 target 解析放在 script 中，避免模板内使用 as 断言
function onConfigText(key: string, e: Event) {
  setConfig(key, (e.target as HTMLInputElement).value);
}
function onConfigNum(key: string, e: Event) {
  setConfig(key, Number((e.target as HTMLInputElement).value) || 0);
}
function onConfigBool(key: string, e: Event) {
  setConfig(key, (e.target as HTMLInputElement).checked);
}

const formError = computed(() => {
  if (!form.value.name.trim()) return "请填写名称";
  if (!form.value.driver) return "请选择驱动类型";
  for (const f of currentFields.value) {
    if (f.required && f.type !== "bool") {
      const v = form.value.config[f.key];
      if (v === undefined || v === null || v === "")
        return `请填写「${f.label}」`;
    }
  }
  return "";
});

async function load() {
  loading.value = true;
  try {
    const res = await storagesApi.listStorages({
      page: page.value,
      page_size: pageSize.value,
    });
    items.value = res.items;
    total.value = res.total;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

watch(page, () => load());

async function loadDrivers() {
  try {
    drivers.value = await storagesApi.listDrivers();
  } catch {
    drivers.value = [];
  }
}

async function loadConfigSchema() {
  try {
    configSchema.value = await storagesApi.getConfigSchema();
  } catch {
    configSchema.value = {};
  }
}

async function save() {
  submitted.value = true;
  if (formError.value) {
    toast.show(formError.value);
    return;
  }
  // 组装配置：密码字段留空时不发送，避免覆盖原密码
  const config: Record<string, unknown> = {};
  for (const f of currentFields.value) {
    const v = form.value.config[f.key];
    if (f.type === "password" && (v === "" || v === undefined || v === null))
      continue;
    if (v !== undefined && v !== null) config[f.key] = v;
  }
  const payload: StoragePayload = {
    name: form.value.name.trim(),
    driver: form.value.driver,
    root_path: form.value.root_path || "/",
    enabled: form.value.enabled,
    sort_order: form.value.sort_order || 0,
    remark: form.value.remark || undefined,
    config,
  };
  saving.value = true;
  try {
    if (editing.value) {
      await storagesApi.updateStorage(editing.value.id, payload);
      toast.show("已保存");
    } else {
      await storagesApi.createStorage(payload);
      toast.show("已创建");
    }
    showForm.value = false;
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

// 列表行：测试已有存储源连接
const testingId = ref<string | null>(null);
async function testExisting(s: StorageRow) {
  testingId.value = s.id;
  try {
    const res = await storagesApi.testStorage(s.id);
    toast.show(res.ok ? "连接成功" : "连接失败");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "测试失败");
  } finally {
    testingId.value = null;
  }
}

// 表单内：按当前配置测试连接
async function testFormConfig() {
  testingForm.value = true;
  try {
    const res = await storagesApi.testStorageConfig(
      form.value.driver,
      form.value.config,
    );
    toast.show(res.ok ? "连接成功" : "连接失败");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "测试失败");
  } finally {
    testingForm.value = false;
  }
}

// 删除
const deletingId = ref<string | null>(null);
async function remove(s: StorageRow) {
  if (!confirm(`删除存储「${s.name}」？`)) return;
  deletingId.value = s.id;
  try {
    await storagesApi.deleteStorage(s.id);
    toast.show("已删除");
    const lastPage = Math.max(
      1,
      Math.ceil(Math.max(0, total.value - 1) / pageSize.value),
    );
    if (page.value > lastPage) page.value = lastPage;
    else load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  } finally {
    deletingId.value = null;
  }
}

// 浏览目录弹窗
const showBrowse = ref(false);
const browseTarget = ref<StorageRow | null>(null);
const browsePath = ref("/");
const browseItems = ref<BrowseItem[]>([]);
const browseLoading = ref(false);

function openBrowse(s: StorageRow) {
  browseTarget.value = s;
  browsePath.value = "/";
  browseItems.value = [];
  showBrowse.value = true;
  loadBrowse("/");
}

async function loadBrowse(path: string) {
  if (!browseTarget.value) return;
  browseLoading.value = true;
  try {
    const res = await storagesApi.browseStorage(browseTarget.value.id, path);
    browsePath.value = res.path || path;
    browseItems.value = res.items || [];
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "浏览失败");
    browseItems.value = [];
  } finally {
    browseLoading.value = false;
  }
}

function browseParent(): string {
  const s = browsePath.value.replace(/\/+$/, "");
  if (!s) return "/";
  const idx = s.lastIndexOf("/");
  if (idx <= 0) return "/";
  return s.slice(0, idx) + "/";
}

function browseEnter(item: BrowseItem) {
  if (!item.is_dir) return;
  const base = browsePath.value.endsWith("/")
    ? browsePath.value
    : browsePath.value + "/";
  loadBrowse(base + item.name);
}

onMounted(async () => {
  await ensurePageSize();
  await Promise.all([loadDrivers(), loadConfigSchema()]);
  load();
});
</script>

<template>
  <div>
    <div
      class="d-flex justify-content-between align-items-center mb-3 flex-wrap gap-2"
    >
      <div>
        <h3 class="mb-0">存储对接</h3>
        <span class="text-secondary small">管理本地与多云存储源对接</span>
      </div>
      <div class="d-flex gap-2">
        <input
          v-model="keyword"
          class="form-control form-control-sm"
          placeholder="搜索存储..."
          style="max-width: 14rem"
          @keyup.enter="search"
        />
        <button
          type="button"
          class="btn btn-sm btn-primary d-inline-flex align-items-center gap-1"
          @click="openCreate"
         v-permission="['storage:create']">
          <IconPlus :size="16" /> 新建存储
        </button>
        <button
          type="button"
          class="btn btn-sm"
          :disabled="loading"
          @click="load"
        >
          刷新
        </button>
      </div>
    </div>

    <div class="card">
      <div class="table-responsive">
        <table class="table table-vcenter card-table cd-admin-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>驱动类型</th>
              <th>根路径</th>
              <th>状态</th>
              <th>排序</th>
              <th>备注</th>
              <th class="w-1 text-end">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in filteredItems" :key="s.id">
              <td class="fw-medium">{{ s.name }}</td>
              <td>
                <span class="badge bg-secondary-lt">{{
                  driverLabel(s.driver)
                }}</span>
              </td>
              <td class="text-secondary small">{{ s.root_path || "/" }}</td>
              <td>
                <span
                  class="badge"
                  :class="s.enabled ? 'bg-success-lt' : 'bg-secondary-lt'"
                >
                  {{ s.enabled ? "启用" : "禁用" }}
                </span>
              </td>
              <td>{{ s.sort_order }}</td>
              <td class="text-secondary small">{{ s.remark || "—" }}</td>
              <td class="text-end text-nowrap">
                <button
                  type="button"
                  class="btn btn-sm btn-ghost-secondary"
                  :disabled="testingId === s.id"
                  title="测试连接"
                  @click="testExisting(s)"
                 v-permission="['storage:create']">
                  <span
                    v-if="testingId === s.id"
                    class="spinner-border spinner-border-sm"
                  />
                  <IconPlugConnected v-else :size="16" />
                </button>
                <button
                  type="button"
                  class="btn btn-sm btn-ghost-secondary"
                  title="浏览目录"
                  @click="openBrowse(s)"
                >
                  <IconFolderOpen :size="16" />
                </button>
                <button
                  type="button"
                  class="btn btn-sm btn-ghost-secondary"
                  title="编辑"
                  @click="openEdit(s)"
                 v-permission="['storage:update']">
                  <IconPencil :size="16" />
                </button>
                <button
                  type="button"
                  class="btn btn-sm btn-ghost-danger"
                  :disabled="deletingId === s.id"
                  title="删除"
                  @click="remove(s)"
                 v-permission="['storage:delete']">
                  <IconTrash :size="16" />
                </button>
              </td>
            </tr>
            <tr v-if="!items.length && !loading">
              <td colspan="7" class="text-secondary text-center py-4">
                暂无存储
              </td>
            </tr>
            <tr v-else-if="!filteredItems.length && !loading">
              <td colspan="7" class="text-secondary text-center py-4">
                未找到匹配的存储
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="card-footer">
        <CdPagination
          v-model:page="page"
          :total="total"
          :page-size="pageSize"
          :loading="loading"
        />
      </div>
    </div>

    <!-- 新建 / 编辑弹窗 -->
    <CdModal
      :show="showForm"
      :title="editing ? '编辑存储' : '新建存储'"
      size="lg"
      @close="showForm = false"
    >
      <form novalidate @submit.prevent="save">
        <div class="modal-body">
          <div
            v-if="submitted && formError"
            class="alert alert-danger py-2 small mb-3"
            role="alert"
          >
            {{ formError }}
          </div>
          <div class="row g-3">
            <div class="col-md-6">
              <label class="form-label required">名称</label>
              <input
                v-model="form.name"
                class="form-control"
                maxlength="128"
                placeholder="例如 my-s3"
              />
            </div>
            <div class="col-md-6">
              <label class="form-label required">驱动类型</label>
              <select
                v-model="form.driver"
                class="form-select"
                @change="onDriverChange"
              >
                <option
                  v-for="opt in driverOptions"
                  :key="opt.driver"
                  :value="opt.driver"
                >
                  {{ opt.label }}
                </option>
              </select>
            </div>
            <div class="col-md-6">
              <label class="form-label">根路径</label>
              <input
                v-model="form.root_path"
                class="form-control"
                placeholder="/"
              />
            </div>
            <div class="col-md-6">
              <label class="form-label">排序</label>
              <input
                v-model.number="form.sort_order"
                type="number"
                min="0"
                class="form-control"
              />
            </div>
            <div class="col-md-6">
              <label class="form-label">备注</label>
              <input v-model="form.remark" class="form-control" />
            </div>
            <div class="col-md-6 d-flex align-items-end">
              <div class="form-check">
                <input
                  v-model="form.enabled"
                  class="form-check-input"
                  type="checkbox"
                  id="storage-enabled"
                />
                <label class="form-check-label" for="storage-enabled"
                  >启用</label
                >
              </div>
            </div>
          </div>

          <hr v-if="currentFields.length" class="my-3" />
          <div v-if="currentFields.length" class="row g-3">
            <div class="col-12">
              <h6 class="mb-2">驱动配置</h6>
            </div>
            <div v-for="f in currentFields" :key="f.key" class="col-md-6">
              <label class="form-label" :class="{ required: f.required }">
                {{ f.label }}
              </label>
              <div v-if="f.type === 'bool'" class="form-check">
                <input
                  class="form-check-input"
                  type="checkbox"
                  :id="`cfg-${f.key}`"
                  :checked="configBool(f.key)"
                  @change="onConfigBool(f.key, $event)"
                />
                <label class="form-check-label" :for="`cfg-${f.key}`"
                  >启用</label
                >
              </div>
              <input
                v-else-if="f.type === 'int'"
                class="form-control"
                type="number"
                :value="configNum(f.key)"
                @input="onConfigNum(f.key, $event)"
              />
              <input
                v-else
                class="form-control"
                :type="f.type === 'password' ? 'password' : 'text'"
                :value="configStr(f.key)"
                autocomplete="off"
                @input="onConfigText(f.key, $event)"
              />
              <div v-if="f.type === 'password'" class="form-hint">
                编辑时留空表示不修改原密码
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button
            type="button"
            class="btn btn-sm me-auto"
            :disabled="testingForm"
            @click="testFormConfig"
           v-permission="['storage:create']">
            <span
              v-if="testingForm"
              class="spinner-border spinner-border-sm me-1"
            />
            测试连接
          </button>
          <button type="button" class="btn btn-sm" @click="showForm = false">
            取消
          </button>
          <button
            type="submit"
            class="btn btn-sm btn-primary"
            :disabled="saving"
          >
            <span v-if="saving" class="spinner-border spinner-border-sm me-1" />
            {{ editing ? "保存" : "创建" }}
          </button>
        </div>
      </form>
    </CdModal>

    <!-- 浏览目录弹窗 -->
    <CdModal
      :show="showBrowse"
      :title="`浏览目录 · ${browseTarget?.name || ''}`"
      size="lg"
      @close="showBrowse = false"
    >
      <div class="modal-body">
        <div class="d-flex align-items-center gap-2 mb-2 flex-wrap">
          <button
            type="button"
            class="btn btn-sm"
            :disabled="browsePath === '/' || browseLoading"
            @click="loadBrowse(browseParent())"
          >
            <IconArrowLeft :size="16" class="me-1" /> 返回上级
          </button>
          <button
            type="button"
            class="btn btn-sm"
            :disabled="browseLoading"
            @click="loadBrowse(browsePath)"
          >
            <IconFolder :size="16" class="me-1" /> 刷新
          </button>
          <span class="text-secondary small text-break"
            >路径：{{ browsePath }}</span
          >
        </div>
        <div class="table-responsive">
          <table class="table table-vcenter card-table cd-admin-table">
            <thead>
              <tr>
                <th>名称</th>
                <th class="w-1">大小</th>
                <th class="w-1">修改时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="browseLoading">
                <td colspan="3" class="text-secondary text-center py-4">
                  <span class="spinner-border spinner-border-sm me-1" />
                  加载中…
                </td>
              </tr>
              <template v-else>
                <tr
                  v-for="(it, idx) in browseItems"
                  :key="`${it.name}-${idx}`"
                >
                  <td>
                    <button
                      v-if="it.is_dir"
                      type="button"
                      class="btn btn-link btn-sm p-0 text-start d-inline-flex align-items-center gap-1"
                      @click="browseEnter(it)"
                    >
                      <IconFolder :size="16" />
                      <span class="fw-medium">{{ it.name }}</span>
                    </button>
                    <span
                      v-else
                      class="d-inline-flex align-items-center gap-1"
                    >
                      <IconFile :size="16" />
                      {{ it.name }}
                    </span>
                  </td>
                  <td class="text-secondary small">
                    {{ it.is_dir ? "—" : fmtSize(it.size) }}
                  </td>
                  <td class="text-secondary small">
                    {{ fmtDate(it.mod_time) }}
                  </td>
                </tr>
                <tr v-if="!browseItems.length">
                  <td colspan="3" class="text-secondary text-center py-4">
                    空目录
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-sm" @click="showBrowse = false">
          关闭
        </button>
      </div>
    </CdModal>
  </div>
</template>
