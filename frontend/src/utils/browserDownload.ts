/** 浏览器原生下载：点击即出现在下载栏，带进度/速度。 */
export function triggerBrowserDownload(url: string) {
  const a = document.createElement('a')
  a.href = url
  a.rel = 'noopener'
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  a.remove()
}

export function downloadBlob(blob: Blob, filename = 'download') {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 30_000)
}

/** POST 表单提交，供需 JSON body 的下载接口（鉴权走 query token）。 */
export function postBrowserDownload(
  url: string,
  fields: Record<string, string | string[]>,
) {
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = url
  form.style.display = 'none'
  for (const [key, value] of Object.entries(fields)) {
    const values = Array.isArray(value) ? value : [value]
    for (const v of values) {
      const input = document.createElement('input')
      input.type = 'hidden'
      input.name = key
      input.value = v
      form.appendChild(input)
    }
  }
  document.body.appendChild(form)
  form.submit()
  form.remove()
}
