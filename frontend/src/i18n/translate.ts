import { i18n } from '@/i18n'

/** 在非组件模块中获取翻译函数 */
export function t(key: string, params?: Record<string, unknown>) {
  return i18n.global.t(key, params ?? {})
}
