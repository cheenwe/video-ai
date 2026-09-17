<script setup lang="ts">
const props = defineProps<{
  show: boolean;
  title: string;
  size?: "sm" | "md" | "lg" | "full";
}>();

const emit = defineEmits<{ close: [] }>();

function onBackdropClick() {
  if (props.size === "full") return;
  emit("close");
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="show"
      class="modal modal-blur show d-block cd-modal-backdrop"
      :class="{ 'cd-modal-backdrop--full': size === 'full' }"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      tabindex="-1"
      @click.self="onBackdropClick"
    >
      <div
        class="modal-dialog"
        :class="{
          'modal-sm': size === 'sm',
          'modal-lg': size === 'lg',
          'modal-dialog-centered': size !== 'full',
          'cd-modal-dialog--full': size === 'full',
        }"
      >
        <div
          class="modal-content"
          :class="{ 'cd-modal-content--full': size === 'full' }"
        >
          <div class="modal-header">
            <h5 class="modal-title text-truncate me-2">{{ title }}</h5>
            <div class="cd-modal-header-actions">
              <slot name="header-actions" />
            </div>
            <button
              type="button"
              class="btn-close"
              aria-label="关闭"
              @click="emit('close')"
            />
          </div>
          <slot />
          <div v-if="$slots.footer" class="modal-footer"><slot name="footer" /></div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
