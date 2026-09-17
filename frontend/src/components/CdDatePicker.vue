<script setup lang="ts">
import { computed } from "vue";
import { VueDatePicker } from "@vuepic/vue-datepicker";
import "@vuepic/vue-datepicker/dist/main.css";

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    type?: "date" | "datetime";
    placeholder?: string;
    size?: "sm" | "md";
    disabled?: boolean;
  }>(),
  {
    modelValue: "",
    type: "datetime",
    placeholder: "",
    size: "sm",
    disabled: false,
  }
);

const emit = defineEmits<{
  "update:modelValue": [string];
  change: [string];
}>();

const inner = computed<Date | null>(() =>
  props.modelValue ? new Date(props.modelValue) : null
);

function onUpdate(v: Date | null) {
  const s = v ? v.toISOString() : "";
  emit("update:modelValue", s);
  emit("change", s);
}
</script>

<template>
  <VueDatePicker
    :model-value="inner"
    @update:model-value="onUpdate"
    :type="type === 'datetime' ? 'datetime' : 'date'"
    :placeholder="placeholder"
    :disabled="disabled"
    clearable
    :enable-time-picker="type === 'datetime'"
    :format="type === 'datetime' ? 'yyyy-MM-dd HH:mm' : 'yyyy-MM-dd'"
    input-class-name="cd-dp-input"
    :class="{ 'cd-dp--sm': size === 'sm' }"
    teleport
    auto-apply
  />
</template>

<style>
/* 让 vue-datepicker 输入框与 Bootstrap form-control-sm 视觉一致 */
.cd-dp--sm .dp__input {
  height: 31px;
  font-size: 0.875rem;
  padding: 0 10px;
  border-radius: 4px;
  border-color: #d6dbe0;
}
.cd-dp--sm .dp__input_wrap {
  display: block;
}
.cd-dp-input {
  width: 100%;
}
</style>
