<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { IconPlus, IconPencil, IconTrash } from "@tabler/icons-vue";
import { ApiError } from "@/api/client";
import * as deptApi from "@/api/departments";
import type { DepartmentRow } from "@/api/departments";
import { useToast } from "@/composables/useToast";
import { usePageSize } from "@/composables/usePageSize";
import { useAuthStore } from "@/stores/auth";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";

const toast = useToast();
const auth = useAuthStore();
const { pageSize, ensurePageSize } = usePageSize();
const departments = ref<DepartmentRow[]>([]);
const allDepartments = ref<DepartmentRow[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const keyword = ref("");
const showForm = ref(false);
const editing = ref<DepartmentRow | null>(null);

const form = ref({
  name: "",
  parent_id: "" as string | number,
  sort_order: 0,
  remark: "",
});

const parentOptions = computed(() =>
  allDepartments.value.filter(
    (d) => !editing.value || d.id !== editing.value.id,
  ),
);

function resetForm() {
  form.value = {
    name: "",
    parent_id: "" as string | number,
    sort_order: 0,
    remark: "",
  };
  editing.value = null;
}

async function load() {
  loading.value = true;
  try {
    const [res, all] = await Promise.all([
      deptApi.listDepartments({ page: page.value, page_size: pageSize.value }),
      deptApi.listAllDepartments(),
    ]);
    departments.value = res.items;
    total.value = res.total;
    allDepartments.value = all;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    loading.value = false;
  }
}

watch(page, () => load());

const filteredDepartments = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return departments.value;
  return departments.value.filter(
    (d) =>
      d.name.toLowerCase().includes(kw) ||
      (d.remark || "").toLowerCase().includes(kw),
  );
});

function search() {
  page.value = 1;
  load();
}

function openCreate() {
  resetForm();
  showForm.value = true;
}

function openEdit(d: DepartmentRow) {
  editing.value = d;
  form.value = {
    name: d.name,
    parent_id: d.parent_id || "",
    sort_order: d.sort_order || 0,
    remark: d.remark || "",
  };
  showForm.value = true;
}

async function save() {
  const payload = {
    name: form.value.name,
    parent_id: form.value.parent_id ? Number(form.value.parent_id) : 0,
    sort_order: form.value.sort_order,
    remark: form.value.remark || undefined,
  };
  try {
    if (editing.value) {
      await deptApi.updateDepartment(editing.value.id, payload);
      toast.show("已保存");
    } else {
      await deptApi.createDepartment(payload);
      toast.show("已添加");
    }
    showForm.value = false;
    resetForm();
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  }
}

async function remove(d: DepartmentRow) {
  if (!confirm(`删除部门「${d.name}」？`)) return;
  try {
    await deptApi.deleteDepartment(d.id);
    toast.show("已删除");
    load();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

function parentName(id?: number | null) {
  if (!id) return "—";
  return (
    allDepartments.value.find((d) => String(d.id) === String(id))?.name || id
  );
}

onMounted(async () => {
  await ensurePageSize();
  load();
});
</script>

<template>
  <div>
    <div class="d-flex gap-2 mb-3 flex-wrap align-items-center">
      <input
        v-model="keyword"
        class="form-control form-control-sm"
        placeholder="搜索部门名称/备注"
        style="max-width: 14rem"
        @keyup.enter="search"
      />
      <button
        type="button"
        class="btn btn-sm btn-primary d-inline-flex align-items-center gap-1"
        @click="openCreate"
       v-permission="['department:create']">
        <IconPlus :size="16" /> 添加部门
      </button>
      <button
        type="button"
        class="btn btn-sm"
        :disabled="loading"
        @click="load"
      >
        刷新
      </button>
      <span class="text-secondary small">维护组织结构与用户归属</span>
    </div>

    <div class="card">
      <div class="table-responsive">
        <table class="table table-vcenter card-table cd-admin-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>上级部门</th>
              <th>排序</th>
              <th>备注</th>
              <th class="w-1"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="d in filteredDepartments" :key="d.id">
              <td class="fw-medium">{{ d.name }}</td>
              <td>{{ parentName(d.parent_id) }}</td>
              <td>{{ d.sort_order }}</td>
              <td class="text-secondary small">{{ d.remark || "—" }}</td>
              <td class="text-end text-nowrap">
                <button
                  type="button"
                  class="btn btn-sm btn-ghost-secondary"
                  @click="openEdit(d)"
                 v-permission="['department:update']">
                  <IconPencil :size="16" />
                </button>
                <button
                  v-if="auth.isSuperAdmin"
                  type="button"
                  class="btn btn-sm btn-ghost-danger"
                  @click="remove(d)"
                 v-permission="['department:delete','@super']">
                  <IconTrash :size="16" />
                </button>
              </td>
            </tr>
            <tr v-if="!departments.length && !loading">
              <td colspan="5" class="text-secondary text-center py-4">
                暂无部门
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

    <CdModal
      :show="showForm"
      :title="editing ? '编辑部门' : '添加部门'"
      @close="showForm = false"
    >
      <form @submit.prevent="save">
        <div class="modal-body">
          <div class="mb-3">
            <label class="form-label">部门名称</label>
            <input v-model="form.name" class="form-control" required />
          </div>
          <div class="row g-2">
            <div class="col-sm-6">
              <label class="form-label">上级部门</label>
              <select v-model="form.parent_id" class="form-select">
                <option value="">无（顶级）</option>
                <option v-for="d in parentOptions" :key="d.id" :value="d.id">
                  {{ d.name }}
                </option>
              </select>
            </div>
            <div class="col-sm-6">
              <label class="form-label">排序</label>
              <input
                v-model.number="form.sort_order"
                class="form-control"
                type="number"
                min="0"
              />
            </div>
          </div>
          <div class="mb-3 mt-2">
            <label class="form-label">备注</label>
            <input v-model="form.remark" class="form-control" />
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-sm" @click="showForm = false">
            取消
          </button>
          <button type="submit" class="btn btn-sm btn-primary">保存</button>
        </div>
      </form>
    </CdModal>
  </div>
</template>
