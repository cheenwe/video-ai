<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import {
  IconPlus,
  IconPencil,
  IconTrash,
  IconRefresh,
  IconForms,
} from "@tabler/icons-vue";
import { ApiError } from "@/api/client";
import * as cfApi from "@/api/customFields";
import type { CustomFieldDef, CustomFieldModule, CustomFieldType } from "@/api/customFields";
import * as dictApi from "@/api/dict";
import type { SysDict } from "@/api/dict";
import { useToast } from "@/composables/useToast";
import CdModal from "@/components/CdModal.vue";

const toast = useToast();

const modules = ref<CustomFieldModule[]>([]);
const selectedModule = ref<string>("");
const items = ref<CustomFieldDef[]>([]);
const loading = ref(false);
const dicts = ref<SysDict[]>([]);
const keyword = ref("");

// 前端搜索过滤（字段编码/字段名称/类型/字典/描述），保持加载逻辑不变
const filteredItems = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return items.value;
  return items.value.filter((d) =>
    [
      d.field_code,
      d.field_name,
      fieldTypeLabel(d.field_type),
      d.dict_code,
      d.description,
    ].some((v) => String(v || "").toLowerCase().includes(kw)),
  );
});

function search() {
  // 列表为前端过滤，无需重新加载
}

const fieldTypeOptions: { value: CustomFieldType; label: string }[] = [
  { value: "text", label: "单行文本" },
  { value: "textarea", label: "多行文本" },
  { value: "number", label: "数字" },
  { value: "date", label: "日期" },
  { value: "datetime", label: "日期时间" },
  { value: "select", label: "下拉单选" },
  { value: "multi_select", label: "下拉多选" },
];

function fieldTypeLabel(t: string) {
  return fieldTypeOptions.find((o) => o.value === t)?.label || t;
}

const showForm = ref(false);
const editing = ref<CustomFieldDef | null>(null);
const saving = ref(false);
const form = ref({
  module: "",
  field_code: "",
  field_name: "",
  field_type: "text" as CustomFieldType,
  dict_code: "",
  description: "",
  required: 0,
  sort_order: 0,
  enabled: 1,
});

const isSelectType = computed(() => form.value.field_type === "select" || form.value.field_type === "multi_select");

async function loadModules() {
  try {
    modules.value = await cfApi.listCustomFieldModules();
    if (modules.value.length && !selectedModule.value) {
      selectedModule.value = modules.value[0].code;
    }
  } catch {
    modules.value = [];
  }
}

async function loadDicts() {
  try {
    const res = await dictApi.listDicts();
    dicts.value = res.items;
  } catch {
    dicts.value = [];
  }
}

async function load() {
  if (!selectedModule.value) {
    items.value = [];
    return;
  }
  loading.value = true;
  try {
    const res = await cfApi.listCustomFieldDefs({ module: selectedModule.value });
    items.value = res.items;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

function resetForm() {
  form.value = {
    module: selectedModule.value || "",
    field_code: "",
    field_name: "",
    field_type: "text",
    dict_code: "",
    description: "",
    required: 0,
    sort_order: 0,
    enabled: 1,
  };
}

function openCreate() {
  editing.value = null;
  resetForm();
  showForm.value = true;
}

function openEdit(d: CustomFieldDef) {
  editing.value = d;
  form.value = {
    module: d.module,
    field_code: d.field_code,
    field_name: d.field_name,
    field_type: d.field_type,
    dict_code: d.dict_code || "",
    description: d.description || "",
    required: d.required,
    sort_order: d.sort_order,
    enabled: d.enabled,
  };
  showForm.value = true;
}

async function save() {
  if (!form.value.module.trim()) {
    toast.show("请填写所属模块");
    return;
  }
  if (!/^[a-z0-9_-]{1,64}$/.test(form.value.module.trim())) {
    toast.show("模块编码仅允许小写字母、数字、下划线、短横线，1-64 位");
    return;
  }
  if (!form.value.field_code.trim() || !form.value.field_name.trim()) {
    toast.show("字段编码和名称不能为空");
    return;
  }
  saving.value = true;
  try {
    if (editing.value) {
      await cfApi.updateCustomFieldDef(editing.value.id, {
        field_name: form.value.field_name.trim(),
        field_type: form.value.field_type,
        dict_code: isSelectType.value ? form.value.dict_code : "",
        description: form.value.description,
        required: form.value.required,
        sort_order: form.value.sort_order,
        enabled: form.value.enabled,
      });
      toast.show("更新成功");
    } else {
      await cfApi.createCustomFieldDef({
        module: form.value.module.trim(),
        field_code: form.value.field_code.trim(),
        field_name: form.value.field_name.trim(),
        field_type: form.value.field_type,
        dict_code: isSelectType.value ? form.value.dict_code : undefined,
        description: form.value.description,
        required: form.value.required,
        sort_order: form.value.sort_order,
        enabled: form.value.enabled,
      });
      toast.show("创建成功");
      // 创建新模块后刷新模块列表，让新模块出现在下拉里
      loadModules();
    }
    showForm.value = false;
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

async function remove(d: CustomFieldDef) {
  if (!confirm(`确定删除字段「${d.field_name}」吗？该字段已存的值也会被清除。`)) return;
  try {
    await cfApi.deleteCustomFieldDef(d.id);
    toast.show("已删除");
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

watch(selectedModule, load);

onMounted(async () => {
  await loadModules();
  await loadDicts();
  if (selectedModule.value) load();
});
</script>

<template>
  <div>
    <div class="d-flex align-items-center justify-content-between gap-2 mb-4">
      <div>
        <h2>自定义字段</h2>
        <p class="text-secondary mb-0">
          按模块定义扩展字段，业务表单（用户、项目等）将自动渲染并保存
        </p>
      </div>
      <button class="btn btn-primary btn-sm" :disabled="!selectedModule" @click="openCreate" v-permission="['custom_field:create']">
        <IconPlus :size="16" class="me-1" />新建字段
      </button>
    </div>

    <div class="card">
      <div class="card-header d-flex align-items-center gap-2">
        <IconForms :size="18" class="me-1" />
        <label class="form-label mb-0 me-2">模块</label>
        <input
          v-model="selectedModule"
          list="cf-module-list"
          class="form-control form-control-sm"
          style="max-width: 16rem"
          placeholder="选择或输入模块名"
        />
        <datalist id="cf-module-list">
          <option v-for="m in modules" :key="m.code" :value="m.code">{{ m.name }}</option>
        </datalist>
        <input
          v-model="keyword"
          class="form-control form-control-sm ms-auto"
          style="max-width: 14rem"
          placeholder="搜索字段..."
          @keyup.enter="search"
        />
        <button class="btn btn-sm btn-ghost-secondary" @click="load">
          <IconRefresh :size="16" />
        </button>
      </div>
      <div class="card-body">
        <div v-if="!selectedModule" class="text-secondary text-center py-4">请选择模块</div>
        <div v-else-if="loading" class="text-secondary text-center py-3">加载中…</div>
        <div v-else-if="!items.length" class="text-secondary text-center py-3">
          该模块暂无自定义字段
        </div>
        <div v-else-if="!filteredItems.length" class="text-secondary text-center py-3">
          未找到匹配的字段
        </div>
        <div v-else class="table-responsive">
          <table class="table table-sm align-middle">
            <thead>
              <tr>
                <th>字段编码</th>
                <th>字段名称</th>
                <th>类型</th>
                <th>预览</th>
                <th>字典来源</th>
                <th>必填</th>
                <th>排序</th>
                <th>状态</th>
                <th>描述</th>
                <th style="width: 110px"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="d in filteredItems" :key="d.id">
                <td><code>{{ d.field_code }}</code></td>
                <td>{{ d.field_name }}</td>
                <td>{{ fieldTypeLabel(d.field_type) }}</td>
                <td class="cd-cf-preview">
                  <input
                    v-if="d.field_type === 'text'"
                    class="form-control form-control-sm"
                    placeholder="预览"
                    disabled
                  />
                  <input
                    v-else-if="d.field_type === 'number'"
                    type="number"
                    class="form-control form-control-sm"
                    placeholder="0"
                    disabled
                  />
                  <input
                    v-else-if="d.field_type === 'date'"
                    type="date"
                    class="form-control form-control-sm"
                    disabled
                  />
                  <input
                    v-else-if="d.field_type === 'datetime'"
                    type="datetime-local"
                    class="form-control form-control-sm"
                    disabled
                  />
                  <textarea
                    v-else-if="d.field_type === 'textarea'"
                    class="form-control form-control-sm"
                    rows="1"
                    placeholder="预览"
                    disabled
                  ></textarea>
                  <select
                    v-else-if="d.field_type === 'select'"
                    class="form-select form-select-sm"
                    disabled
                  >
                    <option>请选择</option>
                  </select>
                  <select
                    v-else-if="d.field_type === 'multi_select'"
                    class="form-select form-select-sm"
                    multiple
                    size="2"
                    disabled
                  >
                    <option>选项 A</option>
                    <option>选项 B</option>
                  </select>
                </td>
                <td>
                  <code v-if="d.dict_code">{{ d.dict_code }}</code>
                  <span v-else class="text-secondary">—</span>
                </td>
                <td>
                  <span class="badge" :class="d.required ? 'bg-red-lt' : 'bg-secondary-lt'">
                    {{ d.required ? '必填' : '否' }}
                  </span>
                </td>
                <td>{{ d.sort_order }}</td>
                <td>
                  <span class="badge" :class="d.enabled ? 'bg-green-lt' : 'bg-secondary-lt'">
                    {{ d.enabled ? '启用' : '停用' }}
                  </span>
                </td>
                <td class="text-secondary small">{{ d.description || '—' }}</td>
                <td>
                  <button class="btn btn-sm btn-icon" title="编辑" @click="openEdit(d)" v-permission="['custom_field:update']">
                    <IconPencil :size="15" />
                  </button>
                  <button class="btn btn-sm btn-icon text-danger" title="删除" @click="remove(d)" v-permission="['custom_field:delete','@super']">
                    <IconTrash :size="15" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <CdModal :show="showForm" :title="editing ? '编辑字段' : '新建字段'" @close="showForm = false">
      <div class="modal-body">
        <div class="row g-3">
          <div class="col-md-6">
            <label class="form-label">所属模块 <span class="text-danger">*</span></label>
            <input
              v-model="form.module"
              list="cf-form-module-list"
              class="form-control"
              :disabled="!!editing"
              placeholder="选择或输入新模块名（如 user / project / asset）"
            />
            <datalist id="cf-form-module-list">
              <option v-for="m in modules" :key="m.code" :value="m.code">{{ m.name }}</option>
            </datalist>
            <div class="form-hint">小写字母、数字、下划线、短横线，1-64 位</div>
          </div>
          <div class="col-md-6">
            <label class="form-label">字段编码 <span class="text-danger">*</span></label>
            <input v-model="form.field_code" class="form-control" :disabled="!!editing" placeholder="如 job_number" />
            <div class="form-hint">同模块下唯一，创建后不可修改</div>
          </div>
          <div class="col-md-6">
            <label class="form-label">字段名称 <span class="text-danger">*</span></label>
            <input v-model="form.field_name" class="form-control" placeholder="如 工号" />
          </div>
          <div class="col-md-6">
            <label class="form-label">字段类型 <span class="text-danger">*</span></label>
            <select v-model="form.field_type" class="form-select">
              <option v-for="o in fieldTypeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </div>
          <div v-if="isSelectType" class="col-md-6">
            <label class="form-label">选项字典 <span class="text-danger">*</span></label>
            <select v-model="form.dict_code" class="form-select">
              <option value="">请选择</option>
              <option v-for="d in dicts" :key="d.id" :value="d.code">{{ d.name }}（{{ d.code }}）</option>
            </select>
            <div class="form-hint">select/multi_select 必须绑定字典作为选项来源</div>
          </div>
          <div class="col-md-6">
            <label class="form-label">排序</label>
            <input v-model.number="form.sort_order" type="number" class="form-control" />
          </div>
          <div class="col-md-6">
            <label class="form-label">状态</label>
            <select v-model.number="form.enabled" class="form-select">
              <option :value="1">启用</option>
              <option :value="0">停用</option>
            </select>
          </div>
          <div class="col-md-6 d-flex align-items-end">
            <label class="form-check">
              <input v-model.number="form.required" class="form-check-input" type="checkbox" :true-value="1" :false-value="0" />
              <span class="form-check-label">必填</span>
            </label>
          </div>
          <div class="col-12">
            <label class="form-label">描述</label>
            <textarea v-model="form.description" class="form-control" rows="2"></textarea>
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

<style scoped>
.cd-cf-preview {
  min-width: 140px;
}
.cd-cf-preview .form-control,
.cd-cf-preview .form-select {
  max-width: 180px;
}
</style>
