// 验证码图片混淆还原：与后端 services/captcha.go 的 xorObfuscate 完全对应。
// 后端下发前用 xorshift32 生成的 keystream 对 PNG 字节做异或；XOR 对称，
// 这里用相同的种子与算法即可还原。仅用于提高直接 OCR 还原的门槛。
const CAPTCHA_SALT = 'va-cap-v1'

function fnv1a(str: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i) & 0xff
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h >>> 0
}

// deobfuscateCaptchaImage 接收混淆后的 base64，返回还原后的纯 base64（无 data: 前缀）。
export function deobfuscateCaptchaImage(id: string, b64: string): string {
  const binary = atob(b64)
  const len = binary.length
  const bytes = new Uint8Array(len)
  for (let i = 0; i < len; i++) bytes[i] = binary.charCodeAt(i)

  let x = fnv1a(CAPTCHA_SALT + id) >>> 0
  if (x === 0) x = 0x1a2b3c4d
  for (let i = 0; i < len; i++) {
    x = (x ^ (x << 13)) >>> 0
    x = (x ^ (x >>> 17)) >>> 0
    x = (x ^ (x << 5)) >>> 0
    bytes[i] ^= x & 0xff
  }

  let out = ''
  for (let i = 0; i < len; i++) out += String.fromCharCode(bytes[i])
  return btoa(out)
}
