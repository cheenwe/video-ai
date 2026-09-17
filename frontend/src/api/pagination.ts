export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export function totalPages(total: number, pageSize: number) {
  return Math.max(1, Math.ceil(total / pageSize))
}
