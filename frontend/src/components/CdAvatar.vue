<script setup lang="ts">
import { computed } from "vue";
import { IconAiAgent } from "@tabler/icons-vue";
import {
  avatarColors,
  avatarInitials,
  hasCustomAvatar,
  type AvatarKind,
} from "@/utils/avatar";

const props = withDefaults(
  defineProps<{
    src?: string | null;
    name?: string;
    kind?: AvatarKind;
    size?: "xs" | "sm" | "md" | "lg";
    title?: string;
  }>(),
  { kind: "user", size: "sm", name: "" },
);

const showImage = computed(() => hasCustomAvatar(props.src));
const initials = computed(() => avatarInitials(props.name || ""));
const colors = computed(() => avatarColors(props.name || "", props.kind));
const showAgentIcon = computed(
  () => !showImage.value && props.kind === "agent",
);

const iconSize = computed(
  () =>
    ({
      xs: 12,
      sm: 16,
      md: 20,
      lg: 24,
    })[props.size],
);

const initialsSize = computed(
  () =>
    ({
      xs: "0.625rem",
      sm: "0.75rem",
      md: "0.875rem",
      lg: "1rem",
    })[props.size],
);

const fallbackStyle = computed(() => ({
  "--cd-avatar-bg": colors.value.bg,
  "--cd-avatar-fg": colors.value.fg,
  background: colors.value.bg,
  color: colors.value.fg,
}));
</script>

<template>
  <span
    class="cd-avatar"
    :class="[
      `cd-avatar--${size}`,
      showImage ? 'cd-avatar--photo' : 'cd-avatar--fallback',
      showAgentIcon ? 'cd-avatar--agent' : 'cd-avatar--user',
    ]"
    :title="title || name"
    :style="!showImage ? fallbackStyle : undefined"
  >
    <img
      v-if="showImage"
      :src="src!"
      :alt="name || ''"
      class="cd-avatar__photo"
    />
    <IconAiAgent v-else-if="showAgentIcon" :size="iconSize" :stroke="1.75" />
    <span
      v-else
      class="cd-avatar__initials"
      :style="{ fontSize: initialsSize }"
      >{{ initials }}</span
    >
  </span>
</template>

<style scoped>
.cd-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border-radius: 50%;
  overflow: hidden;
  line-height: 0;
  font-weight: 600;
  vertical-align: middle;
  box-shadow: none;
  border: none;
}

.cd-avatar--xs {
  width: 1.25rem;
  height: 1.25rem;
}

.cd-avatar--sm {
  width: 2rem;
  height: 2rem;
}

.cd-avatar--md {
  width: 2.5rem;
  height: 2.5rem;
}

.cd-avatar--lg {
  width: 3rem;
  height: 3rem;
}

.cd-avatar--fallback {
  background: var(--cd-avatar-bg, #eef2ff);
  color: var(--cd-avatar-fg, #4f46e5);
}

.cd-avatar__initials {
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  user-select: none;
}

.cd-avatar__photo {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.cd-avatar svg {
  display: block;
  flex-shrink: 0;
}
</style>
