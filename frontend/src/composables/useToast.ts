import { ref } from 'vue'

const visible = ref(false)
const message = ref('')
let timer: ReturnType<typeof setTimeout> | null = null

export function useToast() {
  function show(msg: string, ms = 2200) {
    message.value = msg
    visible.value = true
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      visible.value = false
    }, ms)
  }
  return { visible, message, show }
}

/** 在路由守卫等非组件上下文显示提示 */
export function showToast(msg: string, ms = 2200) {
  useToast().show(msg, ms)
}
