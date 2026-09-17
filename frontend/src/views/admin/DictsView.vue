<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { IconPlus, IconPencil, IconTrash, IconSearch, IconRefresh, IconList } from "@tabler/icons-vue";
import { ApiError } from "@/api/client";
import * as dictApi from "@/api/dict";
import type { SysDict, SysDictItem } from "@/api/dict";
import { useToast } from "@/composables/useToast";
import { usePageSize } from "@/composables/usePageSize";
import CdModal from "@/components/CdModal.vue";
import CdPagination from "@/components/CdPagination.vue";

const toast = useToast();
const { pageSize, ensurePageSize } = usePageSize();

// ---------- 字典列表 ----------
const dicts = ref<SysDict[]>([]);
const dictTotal = ref(0);
const dictPage = ref(1);
const dictLoading = ref(false);
const dictKeyword = ref("");

const selectedDictId = ref<number | null>(null);

async function loadDicts() {
  dictLoading.value = true;
  try {
    const res = await dictApi.listDicts({
      keyword: dictKeyword.value || undefined,
      page: dictPage.value,
      page_size: pageSize.value,
    });
    dicts.value = res.items;
    dictTotal.value = res.total;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    dictLoading.value = false;
  }
}

function selectDict(id: number) {
  selectedDictId.value = id;
  loadItems();
}

// ---------- 字典项列表 ----------
const items = ref<SysDictItem[]>([]);
const itemsLoading = ref(false);

async function loadItems() {
  if (!selectedDictId.value) {
    items.value = [];
    return;
  }
  itemsLoading.value = true;
  try {
    const res = await dictApi.listDictItems(selectedDictId.value);
    items.value = res.items;
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "加载失败");
  } finally {
    itemsLoading.value = false;
  }
}

// ---------- 字典表单 ----------
const showDictForm = ref(false);
const dictEditing = ref<SysDict | null>(null);
const dictSaving = ref(false);
const dictForm = ref({ code: "", name: "", description: "", sort_order: 0, enabled: 1 });

function openCreateDict() {
  dictEditing.value = null;
  dictForm.value = { code: "", name: "", description: "", sort_order: 0, enabled: 1 };
  showDictForm.value = true;
}

function openEditDict(d: SysDict) {
  dictEditing.value = d;
  dictForm.value = { code: d.code, name: d.name, description: d.description || "", sort_order: d.sort_order, enabled: d.enabled };
  showDictForm.value = true;
}

async function saveDict() {
  if (!dictForm.value.code.trim() || !dictForm.value.name.trim()) {
    toast.show("编码和名称不能为空");
    return;
  }
  dictSaving.value = true;
  try {
    if (dictEditing.value) {
      await dictApi.updateDict(dictEditing.value.id, {
        name: dictForm.value.name,
        description: dictForm.value.description,
        sort_order: dictForm.value.sort_order,
        enabled: dictForm.value.enabled,
      });
      toast.show("更新成功");
    } else {
      await dictApi.createDict(dictForm.value);
      toast.show("创建成功");
    }
    showDictForm.value = false;
    loadDicts();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    dictSaving.value = false;
  }
}

async function removeDict(d: SysDict) {
  if (!confirm(`确定删除字典「${d.name}」吗？`)) return;
  try {
    await dictApi.deleteDict(d.id);
    toast.show("已删除");
    if (selectedDictId.value === d.id) {
      selectedDictId.value = null;
      items.value = [];
    }
    loadDicts();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

// ---------- 字典项表单 ----------
const showItemForm = ref(false);
const itemEditing = ref<SysDictItem | null>(null);
const itemSaving = ref(false);
const itemForm = ref({ code: "", name: "", value: "", sort_order: 0, enabled: 1, remark: "" });

function openCreateItem() {
  if (!selectedDictId.value) return;
  itemEditing.value = null;
  itemForm.value = { code: "", name: "", value: "", sort_order: 0, enabled: 1, remark: "" };
  showItemForm.value = true;
}

function openEditItem(it: SysDictItem) {
  itemEditing.value = it;
  itemForm.value = { code: it.code, name: it.name, value: it.value, sort_order: it.sort_order, enabled: it.enabled, remark: it.remark || "" };
  showItemForm.value = true;
}

async function saveItem() {
  if (!itemForm.value.code.trim() || !itemForm.value.name.trim() || !itemForm.value.value.trim()) {
    toast.show("编码、名称和值不能为空");
    return;
  }
  if (!selectedDictId.value) return;
  itemSaving.value = true;
  try {
    if (itemEditing.value) {
      await dictApi.updateDictItem(selectedDictId.value, itemEditing.value.id, {
        name: itemForm.value.name,
        value: itemForm.value.value,
        sort_order: itemForm.value.sort_order,
        enabled: itemForm.value.enabled,
        remark: itemForm.value.remark,
      });
      toast.show("更新成功");
    } else {
      await dictApi.createDictItem(selectedDictId.value, itemForm.value);
      toast.show("创建成功");
    }
    showItemForm.value = false;
    loadItems();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "保存失败");
  } finally {
    itemSaving.value = false;
  }
}

async function removeItem(it: SysDictItem) {
  if (!selectedDictId.value) return;
  if (!confirm(`确定删除字典项「${it.name}」吗？`)) return;
  try {
    await dictApi.deleteDictItem(selectedDictId.value, it.id);
    toast.show("已删除");
    loadItems();
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "删除失败");
  }
}

// ---------- 生命周期 ----------
onMounted(async () => {
  await ensurePageSize();
  loadDicts();
});

watch(dictPage, loadDicts);
</script>

<template>
  <div>
    <div class="d-flex align-items-center justify-content-between gap-2 mb-4">
      <div>
        <h2>字典管理</h2>
        <p class="text-secondary mb-0">维护系统字典类型与字典项，供业务表单下拉选择</p>
      </div>
    </div>

    <div class="row g-4">
      <!-- 左：字典类型列表 -->
      <div class="col-lg-5">
        <div class="card">
          <div class="card-header d-flex align-items-center justify-content-between">
            <h3 class="card-title mb-0">字典类型</h3>
            <button class="btn btn-primary btn-sm" @click="openCreateDict" v-permission="['dict:create']">
              <IconPlus :size="16" class="me-1" />新建
            </button>
          </div>
          <div class="card-body">
            <div class="input-group mb-3">
              <span class="input-group-text"><IconSearch :size="16" /></span>
              <input v-model="dictKeyword" class="form-control" placeholder="搜索编码或名称" @keyup.enter="loadDicts" />
              <button class="btn btn-outline-secondary" @click="loadDicts">
                <IconRefresh :size="16" />
              </button>
            </div>
            <div v-if="dictLoading" class="text-secondary text-center py-3">加载中…</div>
            <div v-else-if="!dicts.length" class="text-secondary text-center py-3">暂无字典</div>
            <div v-else class="list-group list-group-flush">
              <button
                v-for="d in dicts"
                :key="d.id"
                class="list-group-item list-group-item-action d-flex align-items-center justify-content-between"
                :class="{ active: selectedDictId === d.id }"
                @click="selectDict(d.id)"
              >
                <div class="text-start">
                  <div class="fw-semibold">{{ d.name }}</div>
                  <div class="small text-secondary">
                    <code>{{ d.code }}</code>
                    <span v-if="d.description" class="ms-2">{{ d.description }}</span>
                  </div>
                </div>
                <div class="d-flex align-items-center gap-1" @click.stop>
                  <span class="badge" :class="d.enabled ? 'bg-green-lt' : 'bg-secondary-lt'">
                    {{ d.enabled ? '启用' : '停用' }}
                  </span>
                  <button class="btn btn-sm btn-icon" title="编辑" @click="openEditDict(d)" v-permission="['dict:update']">
                    <IconPencil :size="15" />
                  </button>
                  <button class="btn btn-sm btn-icon text-danger" title="删除" @click="removeDict(d)" v-permission="['dict:delete','@super']">
                    <IconTrash :size="15" />
                  </button>
                </div>
              </button>
            </div>
            <CdPagination
              v-if="dictTotal > pageSize"
              :page="dictPage"
              :page-size="pageSize"
              :total="dictTotal"
              @update:page="(p) => (dictPage = p)"
            />
          </div>
        </div>
      </div>

      <!-- 右：字典项列表 -->
      <div class="col-lg-7">
        <div class="card">
          <div class="card-header d-flex align-items-center justify-content-between">
            <h3 class="card-title mb-0">
              <IconList :size="18" class="me-1" />
              字典项
              <span v-if="selectedDictId" class="text-secondary fs-6 fw-normal ms-2">
                {{ dicts.find(d => d.id === selectedDictId)?.name }}
              </span>
            </h3>
            <button class="btn btn-primary btn-sm" :disabled="!selectedDictId" @click="openCreateItem" v-permission="['dict:create']">
              <IconPlus :size="16" class="me-1" />新建项
            </button>
          </div>
          <div class="card-body">
            <div v-if="!selectedDictId" class="text-secondary text-center py-5">
              请选择左侧字典类型
            </div>
            <div v-else-if="itemsLoading" class="text-secondary text-center py-3">加载中…</div>
            <div v-else-if="!items.length" class="text-secondary text-center py-3">暂无字典项</div>
            <div v-else class="table-responsive">
              <table class="table table-sm align-middle">
                <thead>
                  <tr>
                    <th>编码</th>
                    <th>名称</th>
                    <th>值</th>
                    <th>排序</th>
                    <th>状态</th>
                    <th>备注</th>
                    <th style="width: 110px"></th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="it in items" :key="it.id">
                    <td><code>{{ it.code }}</code></td>
                    <td>{{ it.name }}</td>
                    <td>{{ it.value }}</td>
                    <td>{{ it.sort_order }}</td>
                    <td>
                      <span class="badge" :class="it.enabled ? 'bg-green-lt' : 'bg-secondary-lt'">
                        {{ it.enabled ? '启用' : '停用' }}
                      </span>
                    </td>
                    <td class="text-secondary small">{{ it.remark || '—' }}</td>
                    <td>
                      <button class="btn btn-sm btn-icon" title="编辑" @click="openEditItem(it)" v-permission="['dict:update']">
                        <IconPencil :size="15" />
                      </button>
                      <button class="btn btn-sm btn-icon text-danger" title="删除" @click="removeItem(it)" v-permission="['dict:delete','@super']">
                        <IconTrash :size="15" />
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 字典表单弹窗 -->
    <CdModal :show="showDictForm" :title="dictEditing ? '编辑字典' : '新建字典'" @close="showDictForm = false">
      <div class="modal-body">
        <div class="mb-3">
          <label class="form-label">编码 <span class="text-danger">*</span></label>
          <input v-model="dictForm.code" class="form-control" :disabled="!!dictEditing" placeholder="如 alert_level" />
          <div class="form-hint">全局唯一，创建后不可修改</div>
        </div>
        <div class="mb-3">
          <label class="form-label">名称 <span class="text-danger">*</span></label>
          <input v-model="dictForm.name" class="form-control" placeholder="如 告警级别" />
        </div>
        <div class="mb-3">
          <label class="form-label">描述</label>
          <textarea v-model="dictForm.description" class="form-control" rows="2"></textarea>
        </div>
        <div class="row g-3">
          <div class="col-6">
            <label class="form-label">排序</label>
            <input v-model.number="dictForm.sort_order" type="number" class="form-control" />
          </div>
          <div class="col-6">
            <label class="form-label">状态</label>
            <select v-model.number="dictForm.enabled" class="form-select">
              <option :value="1">启用</option>
              <option :value="0">停用</option>
            </select>
          </div>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" @click="showDictForm = false">取消</button>
        <button class="btn btn-primary" :disabled="dictSaving" @click="saveDict">
          {{ dictSaving ? '保存中…' : '保存' }}
        </button>
      </div>
    </CdModal>

    <!-- 字典项表单弹窗 -->
    <CdModal :show="showItemForm" :title="itemEditing ? '编辑字典项' : '新建字典项'" @close="showItemForm = false">
      <div class="modal-body">
        <div class="mb-3">
          <label class="form-label">编码 <span class="text-danger">*</span></label>
          <input v-model="itemForm.code" class="form-control" :disabled="!!itemEditing" placeholder="如 high" />
          <div class="form-hint">同一字典下唯一，创建后不可修改</div>
        </div>
        <div class="mb-3">
          <label class="form-label">名称 <span class="text-danger">*</span></label>
          <input v-model="itemForm.name" class="form-control" placeholder="如 高" />
        </div>
        <div class="mb-3">
          <label class="form-label">值 <span class="text-danger">*</span></label>
          <input v-model="itemForm.value" class="form-control" placeholder="如 3" />
        </div>
        <div class="row g-3">
          <div class="col-6">
            <label class="form-label">排序</label>
            <input v-model.number="itemForm.sort_order" type="number" class="form-control" />
          </div>
          <div class="col-6">
            <label class="form-label">状态</label>
            <select v-model.number="itemForm.enabled" class="form-select">
              <option :value="1">启用</option>
              <option :value="0">停用</option>
            </select>
          </div>
        </div>
        <div class="mt-3">
          <label class="form-label">备注</label>
          <input v-model="itemForm.remark" class="form-control" />
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" @click="showItemForm = false">取消</button>
        <button class="btn btn-primary" :disabled="itemSaving" @click="saveItem">
          {{ itemSaving ? '保存中…' : '保存' }}
        </button>
      </div>
    </CdModal>
  </div>
</template>
