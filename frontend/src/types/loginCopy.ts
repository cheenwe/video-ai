export const LOGIN_COPY_FIELDS = [
  {
    "key": "badge",
    "label": "品牌角标",
    "required": false
  },
  {
    "key": "eyebrow",
    "label": "表单上方说明",
    "required": false
  },
  {
    "key": "title",
    "label": "表单标题",
    "required": true
  },
  {
    "key": "username_label",
    "label": "用户名标签",
    "required": true
  },
  {
    "key": "username_placeholder",
    "label": "用户名占位提示",
    "required": false
  },
  {
    "key": "password_label",
    "label": "密码标签",
    "required": true
  },
  {
    "key": "password_placeholder",
    "label": "密码占位提示",
    "required": false
  },
  {
    "key": "submit",
    "label": "登录按钮",
    "required": true
  },
  {
    "key": "submitting",
    "label": "登录中按钮",
    "required": true
  },
  {
    "key": "show_password",
    "label": "显示密码按钮",
    "required": true
  },
  {
    "key": "hide_password",
    "label": "隐藏密码按钮",
    "required": true
  },
  {
    "key": "help",
    "label": "账号帮助说明",
    "required": false
  },
  {
    "key": "footer",
    "label": "页脚说明",
    "required": false
  },
  {
    "key": "username_required",
    "label": "用户名为空提示",
    "required": true
  },
  {
    "key": "password_required",
    "label": "密码为空提示",
    "required": true
  },
  {
    "key": "invalid_credentials",
    "label": "账号认证失败提示",
    "required": true
  },
  {
    "key": "rate_limited",
    "label": "登录限流提示",
    "required": true
  },
  {
    "key": "service_unavailable",
    "label": "网络或服务异常提示",
    "required": true
  },
  {
    "key": "generic_error",
    "label": "其他登录失败提示",
    "required": true
  }
] as const

export type LoginCopyKey = typeof LOGIN_COPY_FIELDS[number]["key"]
export type LoginCopy = Record<LoginCopyKey, string>
export const DEFAULT_LOGIN_COPY: LoginCopy = {
  "badge": "VIDEO INTELLIGENCE",
  "eyebrow": "管理控制台",
  "title": "登录账号",
  "username_label": "用户名",
  "username_placeholder": "请输入用户名",
  "password_label": "密码",
  "password_placeholder": "请输入密码",
  "submit": "登录",
  "submitting": "正在登录…",
  "show_password": "显示密码",
  "hide_password": "隐藏密码",
  "help": "",
  "footer": "",
  "username_required": "请输入用户名",
  "password_required": "请输入密码",
  "invalid_credentials": "用户名或密码错误，或账号不可用",
  "rate_limited": "登录尝试过于频繁，请稍后再试。",
  "service_unavailable": "暂时无法连接服务，请稍后重试。",
  "generic_error": "登录失败，请稍后重试。"
}
