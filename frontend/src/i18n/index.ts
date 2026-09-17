import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN'
import en from './locales/en'
import zhTW from './locales/zh-TW'

export const LOCALE_STORAGE_KEY = 'baseplatform_locale'

export type AppLocale = 'zh-CN' | 'en' | 'zh-TW'

export const LOCALE_OPTIONS: { value: AppLocale; labelKey: string }[] = [
  { value: 'zh-CN', labelKey: 'language.zhCN' },
  { value: 'en', labelKey: 'language.en' },
  { value: 'zh-TW', labelKey: 'language.zhTW' },
]

function readStoredLocale(): AppLocale {
  const raw = localStorage.getItem(LOCALE_STORAGE_KEY)
  if (raw === 'zh-CN' || raw === 'en' || raw === 'zh-TW') return raw
  const nav = navigator.language
  if (nav.startsWith('zh')) {
    return nav.includes('TW') || nav.includes('HK') || nav.includes('Hant') ? 'zh-TW' : 'zh-CN'
  }
  if (nav.startsWith('en')) return 'en'
  return 'zh-CN'
}

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: readStoredLocale(),
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    en,
    'zh-TW': zhTW,
  },
})

export function setDocumentLang(locale: AppLocale) {
  document.documentElement.lang = locale
}

setDocumentLang(i18n.global.locale.value as AppLocale)

export default i18n
