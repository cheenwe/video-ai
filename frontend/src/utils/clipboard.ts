/** 复制文本到剪贴板；Clipboard API 不可用时回退到 execCommand。 */
export async function copyToClipboard(text: string): Promise<boolean> {
  const value = text ?? ''
  if (!value) return false

  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return true
    } catch {
      // 非安全上下文 / 权限拒绝时走回退
    }
  }

  return copyWithExecCommand(value)
}

function copyWithExecCommand(text: string): boolean {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.setAttribute('readonly', '')
  ta.style.position = 'fixed'
  ta.style.top = '0'
  ta.style.left = '0'
  ta.style.width = '1px'
  ta.style.height = '1px'
  ta.style.padding = '0'
  ta.style.border = 'none'
  ta.style.outline = 'none'
  ta.style.boxShadow = 'none'
  ta.style.background = 'transparent'
  ta.style.opacity = '0'
  document.body.appendChild(ta)

  const selection = document.getSelection()
  const previousRange = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null

  ta.focus()
  ta.select()
  ta.setSelectionRange(0, text.length)

  let ok = false
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }

  document.body.removeChild(ta)

  if (previousRange && selection) {
    selection.removeAllRanges()
    selection.addRange(previousRange)
  }

  return ok
}
