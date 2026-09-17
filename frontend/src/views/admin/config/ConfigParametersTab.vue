<script setup lang="ts">
import { onMounted, ref } from "vue";
import { IconDotsVertical, IconPlus, IconRefresh } from "@tabler/icons-vue";
import { useToast } from "@/composables/useToast";
import { usePageSize } from "@/composables/usePageSize";
import * as configs from "@/api/sysConfig";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";
import { useAuthStore } from "@/stores/auth";

const toast = useToast();
const auth = useAuthStore();
const { pageSize, ensurePageSize } = usePageSize();

const items = ref<configs.SysConfigRow[]>([]);
const total = ref(0);
const page = ref(1);
const keyword = ref("");
const show = ref(false);
const editing = ref<number | null>(null);
const busy = ref(false);
const form = ref({
  category: "custom",
  code: "",
  value: "",
  status: "active",
  version: "1",
});

async function load() {
  try {
    const r = await configs.listSysConfigs({
      page: page.value,
      page_size: pageSize.value,
      keyword: keyword.value,
    });
    items.value = r.items;
    total.value = r.total;
  } catch (e) {
    toast.show(String(e));
  }
}

function search() {
  page.value = 1;
  load();
}

function edit(r?: configs.SysConfigRow) {
  editing.value = r?.id ?? null;
  form.value = r
    ? { ...r }
    : { category: "custom", code: "", value: "", status: "active", version: "1" };
  show.value = true;
}

async function saveConfig() {
  busy.value = true;
  try {
    if (editing.value) await configs.updateSysConfig(editing.value, form.value);
    else await configs.createSysConfig(form.value);
    show.value = false;
    await load();
    toast.show("参数已保存");
  } catch (e) {
    toast.show(String(e));
  } finally {
    busy.value = false;
  }
}

async function remove(r: configs.SysConfigRow) {
  if (!confirm(`删除参数 ${r.code}？`)) return;
  try {
    await configs.deleteSysConfig(r.id);
    await load();
  } catch (e) {
    toast.show(String(e));
  }
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
        placeholder="搜索参数编码或分类"
        style="max-width: 16rem"
        @keyup.enter="search"
      />
      <button type="button" class="btn btn-sm btn-primary d-inline-flex align-items-center gap-1" @click="edit()">
        <IconPlus :size="16" /> 添加参数
      </button>
      <button type="button" class="btn btn-sm btn-ghost-secondary" title="刷新" @click="load">
        <IconRefresh :size="16" />
      </button>
    </div>

    <div class="card">
      <div class="table-responsive">
        <table class="table table-vcenter card-table cd-admin-table">
          <thead>
            <tr>
              <th>分类</th>
              <th>编码</th>
              <th>值</th>
              <th>状态</th>
              <th class="w-1"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in items" :key="r.id">
              <td>
                <span class="badge bg-secondary-lt">{{ r.category }}</span>
              </td>
              <td>{{ r.code }}</td>
              <td class="text-truncate" style="max-width: 260px" :title="r.value">
                {{ r.value }}
              </td>
              <td>
                <span :class="r.status === 'active' ? 'badge bg-success-lt' : 'badge bg-secondary-lt'">
                  {{ r.status === 'active' ? '启用' : '禁用' }}
                </span>
              </td>
              <td class="text-end text-nowrap">
                <div class="dropdown">
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost-secondary dropdown-toggle d-inline-flex align-items-center gap-1"
                    data-bs-toggle="dropdown"
                    aria-expanded="false"
                  >
                    <IconDotsVertical :size="16" />
                  </button>
                  <ul class="dropdown-menu dropdown-menu-end">
                    <li>
                      <button type="button" class="dropdown-item" @click="edit(r)">编辑</button>
                    </li>
                    <template v-if="auth.isSuperAdmin">
                      <li><hr class="dropdown-divider" /></li>
                      <li>
                        <button type="button" class="dropdown-item text-danger" @click="remove(r)">删除</button>
                      </li>
                    </template>
                  </ul>
                </div>
              </td>
            </tr>
            <tr v-if="!items.length">
              <td colspan="5" class="text-secondary text-center py-4">暂无自定义参数</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="card-footer">
        <CdPagination
          v-model:page="page"
          :total="total"
          :page-size="pageSize"
          @update:page="load"
        />
      </div>
    </div>

    <CdModal :show="show" :title="editing ? '编辑参数' : '添加参数'" @close="show = false">
      <form @submit.prevent="saveConfig">
        <div class="modal-body">
          <div class="mb-3">
            <label class="form-label">分类</label>
            <input v-model="form.category" class="form-control" :disabled="!!editing" required />
          </div>
          <div class="mb-3">
            <label class="form-label">编码</label>
            <input v-model="form.code" class="form-control" :disabled="!!editing" required />
          </div>
          <div class="mb-3">
            <label class="form-label">值</label>
            <textarea v-model="form.value" class="form-control" rows="3" />
          </div>
          <div class="mb-3">
            <label class="form-label">状态</label>
            <select v-model="form.status" class="form-select">
              <option value="active">启用</option>
              <option value="disabled">禁用</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn" @click="show = false">取消</button>
          <button type="submit" class="btn btn-primary" :disabled="busy">保存</button>
        </div>
      </form>
    </CdModal>
  </div>
</template>
