import { DEFAULT_LOGIN_COPY, type LoginCopy } from "./loginCopy"
export interface BrandingSettings {
  login_copy: LoginCopy
  site_name: string
  tagline: string
  primary_color: string
  logo_url: string
  favicon_url: string
  logo_text: string
  login_description?: string
  footer_powered_by_name?: string
  footer_icp?: string
  footer_powered_by_url?: string
  footer_wechat?: string
  share_ad_image_url?: string
  share_ad_url?: string
  pickup_enabled?: string
}

/** 登录页页脚固定项，不可后台配置时的默认值 */
export const LOGIN_FOOTER_POWERED_BY_URL = 'https://baseplatform'
export const LOGIN_FOOTER_WECHAT = 'amwahaha'
/** 分享页广告默认跳转（广告图片未配置时右侧展示登录页左侧英雄区，仍可点击） */
export const DEFAULT_SHARE_AD_URL = 'https://nextfile.cn/'

export function footerPoweredByUrl(settings: Pick<BrandingSettings, 'footer_powered_by_url'>) {
  return settings.footer_powered_by_url?.trim() || LOGIN_FOOTER_POWERED_BY_URL
}

export function footerWechat(settings: Pick<BrandingSettings, 'footer_wechat'>) {
  return settings.footer_wechat?.trim() || LOGIN_FOOTER_WECHAT
}

export function shareAdUrl(settings: Pick<BrandingSettings, 'share_ad_url'>) {
  return settings.share_ad_url?.trim() || DEFAULT_SHARE_AD_URL
}

export function isPickupEnabled(settings: PickupEnabledSource): boolean {
  const v = settings.pickup_enabled
  if (v === false || v === 'false' || v === '0' || v === '') return false
  if (v === true || v === 'true' || v === '1') return true
  return true
}

type PickupEnabledSource = { pickup_enabled?: string | boolean }

export const DEFAULT_BRANDING: BrandingSettings = {
  login_copy: {...DEFAULT_LOGIN_COPY},
  site_name: 'Video AI',
  tagline: '视频智能分析管理平台',
  primary_color: '#176b4d',
  logo_url: '',
  favicon_url: '',
  logo_text: '',
  login_description:
    '统一管理视频源、分析任务与现场事件',
  footer_powered_by_name: 'Video AI',
  footer_icp: '',
  share_ad_image_url: '',
  share_ad_url: DEFAULT_SHARE_AD_URL,
  pickup_enabled: 'true',
}

export const COLOR_PRESETS = [
  { name: '松针绿', value: '#176b4d' },
  { name: '天蓝', value: '#2563eb' },
  { name: '靛青', value: '#4f46e5' },
  { name: '紫罗兰', value: '#7c3aed' },
  { name: '樱粉', value: '#db2777' },
  { name: '玫红', value: '#e11d48' },
  { name: '珊瑚', value: '#f97316' },
  { name: '琥珀', value: '#d97706' },
  { name: '青绿', value: '#0d9488' },
  { name: '翡翠', value: '#10b981' },
  { name: '湖蓝', value: '#0891b2' },
  { name: '藏青', value: '#1e3a8a' },
  { name: '石墨', value: '#475569' },
]
