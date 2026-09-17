<template>
  <RouterView />
  <div
    class="toast-host toast-host--global"
    :class="{ show: toast.visible.value }"
  >
    {{ toast.message.value }}
  </div>
</template>

<script setup lang="ts">
import { RouterView } from "vue-router";
import { useToast } from "@/composables/useToast";

const toast = useToast();
</script>
