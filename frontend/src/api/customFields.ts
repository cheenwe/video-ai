import { request } from './client'
import type { SysDictItem } from './dict'

// 支持的字段类型
export type CustomFieldType =
  | 'text'
  | 'textarea'
  | 'number'
  | 'date'
  | 'datetime'
  | 'select'
  | 'multi_select'

export interface CustomFieldDef {
  id: number
  module: string
  field_code: string
  field_name: string
  field_type: CustomFieldType
  dict_code?: string
  description?: string
  required: number
  sort_order: number
  enabled: number
  created_at?: string
  updated_at?: string
}

export interface CustomFieldWithValue extends CustomFieldDef {
  value: string
  options?: SysDictItem[]
}

export interface CustomFieldModule {
  code: string
  name: string
  builtin?: number // 1=内置推荐，0=自定义
}

// ---- 管理员：字段定义 CRUD ----

export function listCustomFieldDefs(params?: { module?: string; keyword?: string }) {
  const q = new URLSearchParams()
  if (params?.module) q.set('module', params.module)
  if (params?.keyword) q.set('keyword', params.keyword)
  const qs = q.toString()
  return request<{ items: CustomFieldDef[]; total: number }>(
    `/api/custom-fields${qs ? `?${qs}` : ''}`,
  )
}

export function createCustomFieldDef(body: {
  module: string
  field_code: string
  field_name: string
  field_type?: CustomFieldType
  dict_code?: string
  description?: string
  required?: number
  sort_order?: number
  enabled?: number
}) {
  return request<CustomFieldDef>('/api/custom-fields', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function updateCustomFieldDef(
  id: number,
  body: Partial<{
    field_name: string
    field_type: CustomFieldType
    dict_code: string
    description: string
    required: number
    sort_order: number
    enabled: number
  }>,
) {
  return request<CustomFieldDef>(`/api/custom-fields/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })
}

export function deleteCustomFieldDef(id: number) {
  return request<null>(`/api/custom-fields/${id}`, { method: 'DELETE' })
}

// ---- 通用：模块列表与值读写 ----

export function listCustomFieldModules() {
  return request<CustomFieldModule[]>('/api/cf-data/modules')
}

/** 获取某模块启用字段定义（用于新建表单渲染，无值） */
export function getCustomFieldDefinitions(module: string) {
  return request<{ fields: CustomFieldWithValue[] }>(
    `/api/cf-data/${encodeURIComponent(module)}/definitions`,
  )
}

/** 获取某模块某实体的字段定义 + 当前值（用于编辑表单/详情页） */
export function getCustomFieldValues(module: string, entityId: number) {
  return request<{ fields: CustomFieldWithValue[] }>(
    `/api/cf-data/${encodeURIComponent(module)}/${entityId}`,
  )
}

/** 批量获取多实体的字段值（用于列表页动态列渲染） */
export function getCustomFieldBatchValues(module: string, entityIds: number[]) {
  const ids = entityIds.join(',')
  return request<{
    fields: CustomFieldDef[]
    values: Record<string, { field_id: number; value: string }[]>
  }>(`/api/cf-data/${encodeURIComponent(module)}/batch?entity_ids=${ids}`)
}

/** 保存某模块某实体的字段值 */
export function saveCustomFieldValues(
  module: string,
  entityId: number,
  values: { field_id: number; value: string }[],
) {
  return request<null>(
    `/api/cf-data/${encodeURIComponent(module)}/${entityId}`,
    {
      method: 'PUT',
      body: JSON.stringify({ values }),
    },
  )
}

// ---- 值序列化辅助 ----

/** 将前端表单中 multi_select 的数组值序列化为存储字符串 */
export function encodeFieldValue(value: unknown, type: CustomFieldType): string {
  if (type === 'multi_select') {
    if (Array.isArray(value)) {
      return JSON.stringify(value as string[])
    }
    if (typeof value === 'string' && value) {
      // 已经是序列化字符串或空字符串直接返回
      return value
    }
    return ''
  }
  return value == null ? '' : String(value)
}

/** 将存储字符串反序列化为前端表单中的值（multi_select 返回数组，其余返回字符串） */
export function decodeFieldValue(
  raw: string,
  type: CustomFieldType,
): string | string[] {
  if (type === 'multi_select') {
    if (!raw) return []
    try {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr)) return arr as string[]
    } catch {
      // 容错：非 JSON，按逗号分隔
      return raw
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
    }
  }
  return raw || ''
}

/**
 * 将存储值格式化为人类可读字符串（用于列表单元格和详情页只读展示）。
 * - select/multi_select：把 code 映射为字典项 name
 * - 其余：直接返回 raw
 */
export function formatFieldValue(
  raw: string,
  type: CustomFieldType,
  options?: { code: string; name: string }[],
): string {
  if (!raw) return '—'
  if (type === 'select') {
    const hit = options?.find((o) => o.code === raw)
    return hit ? hit.name : raw
  }
  if (type === 'multi_select') {
    const arr = decodeFieldValue(raw, type) as string[]
    if (!Array.isArray(arr) || arr.length === 0) return '—'
    const labels = arr.map((c) => options?.find((o) => o.code === c)?.name || c)
    return labels.join('、')
  }
  return raw
}
