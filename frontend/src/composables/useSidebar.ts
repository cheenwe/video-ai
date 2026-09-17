import { ref, watch } from 'vue'

const STORAGE_KEY = 'va_sidebar_collapsed'

const collapsed = ref(localStorage.getItem(STORAGE_KEY) === '1')
/** 小屏抽屉导航是否打开 */
const mobileOpen = ref(false)

watch(collapsed, (v) => {
  localStorage.setItem(STORAGE_KEY, v ? '1' : '0')
})

export function useSidebar() {
  function toggle() {
    collapsed.value = !collapsed.value
  }

  function expand() {
    collapsed.value = false
  }

  function openMobile() {
    mobileOpen.value = true
  }

  function closeMobile() {
    mobileOpen.value = false
  }

  function toggleMobile() {
    mobileOpen.value = !mobileOpen.value
  }

  return {
    collapsed,
    mobileOpen,
    toggle,
    expand,
    openMobile,
    closeMobile,
    toggleMobile,
  }
}
