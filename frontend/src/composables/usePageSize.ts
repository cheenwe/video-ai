import { ref } from 'vue'
import { fetchPageSize } from '@/api/settings'

let cached: number | null = null

export function usePageSize() {
  const pageSize = ref(20)

  async function ensurePageSize() {
    if (cached === null) {
      cached = await fetchPageSize()
    }
    pageSize.value = cached
    return pageSize.value
  }

  async function refreshPageSize() {
    cached = await fetchPageSize()
    pageSize.value = cached
    return cached
  }

  function invalidatePageSize() {
    cached = null
  }

  return { pageSize, ensurePageSize, refreshPageSize, invalidatePageSize }
}
