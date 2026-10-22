import type { ComposerTranslation } from 'vue-i18n'

/** SysLog.action → i18n message key under `logs.actions.*` */
export const LOG_ACTION_I18N_KEYS: Record<string, string> = {
  login: 'logs.actions.login',
  'login.oauth': 'logs.actions.loginOauth',
  logout: 'logs.actions.logout',

  'auth.register': 'logs.actions.authRegister',
  'auth.profile.update': 'logs.actions.authProfileUpdate',
  'auth.change_password': 'logs.actions.authChangePassword',
  'auth.forgot_password': 'logs.actions.authForgotPassword',
  'auth.reset_password': 'logs.actions.authResetPassword',
  'auth.login_rate_limited': 'logs.actions.authLoginRateLimited',
  'auth.totp.setup': 'logs.actions.authTotpSetup',
  'auth.totp.enable': 'logs.actions.authTotpEnable',
  'auth.totp.disable': 'logs.actions.authTotpDisable',
  'auth.avatar.upload': 'logs.actions.authAvatarUpload',
  'auth.avatar.preset': 'logs.actions.authAvatarPreset',
  'auth.avatar.delete': 'logs.actions.authAvatarDelete',

  'user.create': 'logs.actions.userCreate',
  'user.update': 'logs.actions.userUpdate',
  'user.delete': 'logs.actions.userDelete',
  'user.enabled': 'logs.actions.userEnabled',
  'user.disabled': 'logs.actions.userDisabled',
  'user.reset_password': 'logs.actions.userResetPassword',
  'user.avatar.upload': 'logs.actions.userAvatarUpload',
  'user.avatar.preset': 'logs.actions.userAvatarPreset',
  'user.avatar.delete': 'logs.actions.userAvatarDelete',

  'token.create': 'logs.actions.tokenCreate',
  'token.update': 'logs.actions.tokenUpdate',
  'token.delete': 'logs.actions.tokenDelete',
  'token.admin_create': 'logs.actions.tokenAdminCreate',
  'token.admin_delete': 'logs.actions.tokenAdminDelete',

  'file.upload': 'logs.actions.fileUpload',
  'file.delete': 'logs.actions.fileDelete',
  'file.mkdir': 'logs.actions.fileMkdir',
  'file.rename': 'logs.actions.fileRename',
  'file.copy': 'logs.actions.fileCopy',
  'file.move': 'logs.actions.fileMove',
  'file.cloud_share': 'logs.actions.fileCloudShare',

  'share.create': 'logs.actions.shareCreate',
  'pickup.create': 'logs.actions.pickupCreate',

  'storage.create': 'logs.actions.storageCreate',
  'storage.update': 'logs.actions.storageUpdate',
  'storage.delete': 'logs.actions.storageDelete',
  'storage.oauth.authorize': 'logs.actions.storageOauthAuthorize',

  'storage_sync.create': 'logs.actions.storageSyncCreate',
  'storage_sync.update': 'logs.actions.storageSyncUpdate',
  'storage_sync.delete': 'logs.actions.storageSyncDelete',
  'storage_sync.run.trigger': 'logs.actions.storageSyncRunTrigger',
  'storage_sync.run.start': 'logs.actions.storageSyncRunStart',
  'storage_sync.run.success': 'logs.actions.storageSyncRunSuccess',
  'storage_sync.run.failed': 'logs.actions.storageSyncRunFailed',

  'agent.create': 'logs.actions.agentCreate',
  'agent.update': 'logs.actions.agentUpdate',
  'agent.delete': 'logs.actions.agentDelete',
  'agent.chat': 'logs.actions.agentChat',
  'agent.file.upload': 'logs.actions.agentFileUpload',
  'agent.avatar.upload': 'logs.actions.agentAvatarUpload',
  'agent.avatar.preset': 'logs.actions.agentAvatarPreset',
  'agent.avatar.delete': 'logs.actions.agentAvatarDelete',

  'department.create': 'logs.actions.departmentCreate',
  'department.update': 'logs.actions.departmentUpdate',
  'department.delete': 'logs.actions.departmentDelete',

  'permission.mount.create': 'logs.actions.permissionMountCreate',
  'permission.mount.update': 'logs.actions.permissionMountUpdate',
  'permission.mount.delete': 'logs.actions.permissionMountDelete',
  'permission.acl.create': 'logs.actions.permissionAclCreate',
  'permission.acl.update': 'logs.actions.permissionAclUpdate',
  'permission.acl.delete': 'logs.actions.permissionAclDelete',

  'sys_config.create': 'logs.actions.sysConfigCreate',
  'sys_config.update': 'logs.actions.sysConfigUpdate',
  'sys_config.delete': 'logs.actions.sysConfigDelete',

  'plugin.register': 'logs.actions.pluginRegister',
  'plugin.update': 'logs.actions.pluginUpdate',
  'plugin.delete': 'logs.actions.pluginDelete',

  'site.branding.update': 'logs.actions.siteBrandingUpdate',
  'extension.update': 'logs.actions.extensionUpdate',
  'qrcode.create': 'logs.actions.qrcodeCreate',

  'schedule.job.create': 'logs.actions.scheduleJobCreate',
  'schedule.job.update': 'logs.actions.scheduleJobUpdate',
  'schedule.job.delete': 'logs.actions.scheduleJobDelete',
  'schedule.job.run': 'logs.actions.scheduleJobRun',
  'schedule.command.create': 'logs.actions.scheduleCommandCreate',
  'schedule.command.update': 'logs.actions.scheduleCommandUpdate',
  'schedule.command.delete': 'logs.actions.scheduleCommandDelete',

  'ops.maintenance.update': 'logs.actions.opsMaintenanceUpdate',
  'ops.cleanup': 'logs.actions.opsCleanup',
  'ops.backup.create': 'logs.actions.opsBackupCreate',
  'ops.backup.delete': 'logs.actions.opsBackupDelete',
  'ops.logs.config': 'logs.actions.opsLogsConfig',

  'nvr.agent.create': 'logs.actions.nvrAgentCreate',
  'nvr.agent.update': 'logs.actions.nvrAgentUpdate',
  'nvr.agent.delete': 'logs.actions.nvrAgentDelete',
  'nvr.config.update': 'logs.actions.nvrConfigUpdate',
  'nvr.record.start': 'logs.actions.nvrRecordStart',
  'nvr.record.stop': 'logs.actions.nvrRecordStop',
  'nvr.archive.start': 'logs.actions.nvrArchiveStart',
  'nvr.recording.download': 'logs.actions.nvrRecordingDownload',

  'camera.create': 'logs.actions.cameraCreate',
  'camera.update': 'logs.actions.cameraUpdate',
  'camera.archive': 'logs.actions.cameraArchive',
  'camera.group.create': 'logs.actions.cameraGroupCreate',
  'boundary.create': 'logs.actions.boundaryCreate',
  'boundary.publish': 'logs.actions.boundaryPublish',
  'deployment.create': 'logs.actions.deploymentCreate',
  'deployment.start': 'logs.actions.deploymentStart',
  'deployment.stop': 'logs.actions.deploymentStop',
  'event.confirm': 'logs.actions.eventConfirm',
  'event.close': 'logs.actions.eventClose',
  'access.create': 'logs.actions.accessCreate',
}

type TranslateFn = ComposerTranslation

/** Resolve SysLog action code (e.g. share.create) to localized label. */
export function logActionLabel(action: string, t: TranslateFn): string {
  const a = (action || '').trim()
  if (!a) return '—'

  if (a.startsWith('settings.') && a.endsWith('.update')) {
    const section = a.slice('settings.'.length, -'.update'.length)
    return String(t('logs.actions.settingsUpdate', { section }))
  }

  const scheduleRun = /^schedule\.(.+)\.(success|failed|skipped)$/.exec(a)
  if (scheduleRun) {
    const [, name, status] = scheduleRun
    if (status === 'success') return String(t('logs.actions.scheduleRunSuccess', { name }))
    if (status === 'failed') return String(t('logs.actions.scheduleRunFailed', { name }))
    return String(t('logs.actions.scheduleRunSkipped', { name }))
  }

  const msgKey = LOG_ACTION_I18N_KEYS[a]
  if (msgKey) return String(t(msgKey))
  return a
}
