export const pagePermissions: Record<string,string[]> = {
 '/admin/overview':['overview:list'], '/admin/users':['user:list'], '/admin/departments':['department:list'],
 '/admin/roles':['role:list'], '/admin/projects':['project:list'], '/admin/configs':['settings:general','settings:security','settings:email','settings:ldap','settings:oauth','settings:gateway','settings:watermark','settings:license','config:list'],
 '/admin/storages':['storage:list'], '/admin/schedules':['schedule:list'], '/admin/ops':['backup:list'],
 '/admin/logs':['log:list'], '/admin/dicts':['dict:list'], '/admin/custom-fields':['custom_field:list'],
 '/admin/webhooks':['webhook:list'], '/admin/api-call-logs':['apilog:list'],
}
