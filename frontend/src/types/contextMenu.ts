import type { Component } from 'vue'

export interface ContextMenuItem {
  id: string
  label: string
  icon?: Component
  /** 条件不满足时禁用 */
  disabled?: boolean
  /** 功能尚未实现，显示「开发中」 */
  dev?: boolean
  danger?: boolean
  dividerAfter?: boolean
}
