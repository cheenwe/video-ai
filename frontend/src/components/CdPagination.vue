<script setup lang="ts">
import { computed } from "vue";
import { totalPages } from "@/api/pagination";

const props = defineProps<{
  page: number;
  total: number;
  pageSize: number;
  loading?: boolean;
}>();

const emit = defineEmits<{
  "update:page": [number];
}>();

const pages = computed(() => totalPages(props.total, props.pageSize));

const rangeStart = computed(() => {
  if (!props.total) return 0;
  return (props.page - 1) * props.pageSize + 1;
});

const rangeEnd = computed(() =>
  Math.min(props.page * props.pageSize, props.total),
);

const pageItems = computed(() => {
  const total = pages.value;
  const current = props.page;
  const items: Array<number | "…"> = [];
  if (total <= 7) {
    for (let i = 1; i <= total; i++) items.push(i);
    return items;
  }
  items.push(1);
  if (current > 3) items.push("…");
  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  for (let i = start; i <= end; i++) items.push(i);
  if (current < total - 2) items.push("…");
  items.push(total);
  return items;
});

function go(p: number) {
  if (p < 1 || p > pages.value || p === props.page || props.loading) return;
  emit("update:page", p);
}
</script>

<template>
  <div v-if="total > 0" class="cd-pagination">
    <p class="cd-pagination-meta text-secondary small mb-0">
      第 {{ rangeStart }}–{{ rangeEnd }} 条，共 {{ total }} 条 · 每页
      {{ pageSize }} 条
    </p>
    <ul class="pagination pagination-sm mb-0">
      <li class="page-item" :class="{ disabled: page <= 1 || loading }">
        <button type="button" class="page-link" @click="go(page - 1)">
          上一页
        </button>
      </li>
      <li
        v-for="(item, idx) in pageItems"
        :key="`${item}-${idx}`"
        class="page-item"
        :class="{ active: item === page, disabled: item === '…' || loading }"
      >
        <button
          type="button"
          class="page-link"
          :disabled="item === '…'"
          @click="typeof item === 'number' && go(item)"
        >
          {{ item }}
        </button>
      </li>
      <li class="page-item" :class="{ disabled: page >= pages || loading }">
        <button type="button" class="page-link" @click="go(page + 1)">
          下一页
        </button>
      </li>
    </ul>
  </div>
</template>
