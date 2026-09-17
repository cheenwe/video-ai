<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { LOCALE_OPTIONS, type AppLocale, setDocumentLang } from "@/i18n";

const props = withDefaults(
  defineProps<{
    inline?: boolean;
  }>(),
  { inline: false },
);

const { locale, t } = useI18n();

const current = computed({
  get: () => locale.value as AppLocale,
  set: (val: AppLocale) => {
    locale.value = val;
    localStorage.setItem("cd_locale", val);
    setDocumentLang(val);
  },
});

const options = computed(() =>
  LOCALE_OPTIONS.map((opt) => ({
    value: opt.value,
    label: t(opt.labelKey),
  })),
);
</script>

<template>
  <label class="cd-lang-switch" :class="{ 'cd-lang-switch--inline': inline }">
    <select
      v-model="current"
      class="form-select form-select-sm cd-lang-switch__select"
      :class="{ 'cd-lang-switch__select--inline': inline }"
      :aria-label="t('language.label')"
    >
      <option v-for="opt in options" :key="opt.value" :value="opt.value">
        {{ opt.label }}
      </option>
    </select>
  </label>
</template>

<style scoped>
.cd-lang-switch {
  display: block;
  margin: 0;
}

.cd-lang-switch--inline {
  max-width: 16rem;
}

.cd-lang-switch__select {
  min-width: 7.5rem;
  border-radius: 999px;
  box-shadow: 0 2px 12px rgba(15, 23, 42, 0.1);
  background: rgba(255, 255, 255, 0.96);
  border-color: rgba(15, 23, 42, 0.1);
}

.cd-lang-switch__select--inline {
  border-radius: var(--tblr-border-radius);
  box-shadow: none;
  background: var(--tblr-bg-forms);
  width: 100%;
}
</style>
