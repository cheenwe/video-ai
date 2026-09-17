/** 通用表单校验（登录注册 / 管理端建用户共用） */

export function isValidEmail(value: string): boolean {
  const s = value.trim()
  if (!s || s.length > 128) return false
  // 要求本地部分 + @ + 含点的域名，避免浏览器 type=email 过宽
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s)
}

export function isValidUsername(value: string): boolean {
  const s = value.trim()
  if (s.length < 3 || s.length > 64) return false
  return /^[a-zA-Z0-9._-]+$/.test(s)
}
