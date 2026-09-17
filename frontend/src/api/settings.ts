import { request } from "./client";
export async function fetchPageSize() {
  try {
    const r = await request<{ value: string }>(
      "/api/sys-configs/value?category=system&code=page_size",
    );
    return Number(r.value) || 20;
  } catch {
    return 20;
  }
}
export function getSettings<T>(section: string) {
  return request<T>(`/api/admin/settings/${section}`);
}
export function saveSettings<T>(section: string, data: T) {
  return request<T>(`/api/admin/settings/${section}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export interface CaptchaConfig {
  type: "digit" | "string" | "math";
  length: number;
  noise_level: "low" | "medium" | "high";
  width: number;
  height: number;
  expiry_seconds: number;
  case_sensitive: boolean;
  obfuscate_image: boolean;
}

export function captchaPreview(config: CaptchaConfig) {
  return request<{ image_base64: string }>(
    "/api/admin/settings/captcha/preview",
    { method: "POST", body: JSON.stringify(config) },
  );
}
