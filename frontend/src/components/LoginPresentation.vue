<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import type {BrandingSettings} from '@/types/branding'
import {footerPoweredByUrl, footerWechat} from '@/types/branding'
const props=defineProps<{settings:BrandingSettings;busy?:boolean;error?:string;preview?:boolean;captchaVisible?:boolean;captchaImage?:string;captchaLoading?:boolean;totpStep?:boolean;notice?:string;rememberUsername?:string;rememberChecked?:boolean}>()
const emit=defineEmits<{submit:[username:string,password:string,captchaAnswer?:string,remember?:boolean];refreshCaptcha:[];submitTotp:[code:string];back:[];usernameBlur:[username:string];updateRemember:[checked:boolean]}>()
const username=ref(''),password=ref(''),visible=ref(false),captchaAnswer=ref(''),totpCode=ref(''),remember=ref(false)
const captchaImageSrc=computed(()=>{const raw=(props.captchaImage||'').trim();if(!raw)return'';if(raw.startsWith('data:'))return raw;return`data:image/png;base64,${raw}`})
const showCaptcha=computed(()=>!!props.captchaVisible&&!props.preview)
const showTotp=computed(()=>!!props.totpStep&&!props.preview)
watch(()=>props.captchaImage,()=>{captchaAnswer.value=''})
watch(()=>props.totpStep,(v)=>{if(!v)totpCode.value=''})
watch(()=>props.rememberUsername,(v)=>{if(v&&v!==username.value)username.value=v})
watch(()=>props.rememberChecked,(v)=>{if(v!==undefined)remember.value=v})
function onSubmit(){if(props.preview||props.busy)return;if(showTotp.value){emit('submitTotp',totpCode.value);return}emit('submit',username.value,password.value,showCaptcha.value?captchaAnswer.value:undefined,remember.value)}
</script>
<template>
 <div class="va-login" :class="{'va-login--preview':preview}">
  <section class="va-intro">
   <span v-if="settings.login_copy.badge" class="va-badge">{{settings.login_copy.badge}}</span>
   <div class="va-intro-main"><h1>{{settings.site_name}}</h1><p v-if="settings.tagline" class="va-tagline">{{settings.tagline}}</p></div>
   <div class="va-graphic" aria-hidden="true"><div class="va-target"><i></i><i></i><i></i><i></i><span></span></div><div class="va-scan"></div></div>
   <p v-if="settings.login_description" class="va-description">{{settings.login_description}}</p>
  </section>
  <section class="va-form">
   <form novalidate @submit.prevent="onSubmit">
    <p v-if="settings.login_copy.eyebrow" class="va-eyebrow">{{settings.login_copy.eyebrow}}</p>
    <h2>{{settings.login_copy.title}}</h2>
    <p v-if="error" role="alert" aria-live="polite" class="alert alert-danger">{{error}}</p>
    <p v-if="notice&&!preview" role="status" aria-live="polite" class="alert alert-success">{{notice}}</p>
    <template v-if="showTotp">
     <div class="mb-4"><label class="form-label" for="totp-code">动态验证码</label>
     <input id="totp-code" v-model="totpCode" class="form-control" placeholder="请输入 6 位动态验证码" maxlength="8" autocomplete="one-time-code" inputmode="numeric" aria-required="true" :disabled="busy"/></div>
     <button type="button" class="va-totp-back" :disabled="busy" @click="emit('back')">返回账号登录</button>
    </template>
    <template v-else>
     <div class="mb-4"><label class="form-label" :for="preview?'preview-username':'username'">{{settings.login_copy.username_label}}</label>
     <input :id="preview?'preview-username':'username'" v-model="username" class="form-control" :placeholder="settings.login_copy.username_placeholder" autocomplete="username" aria-required="true" :disabled="busy||preview" @blur="emit('usernameBlur',username)"/></div>
     <div class="mb-4"><label class="form-label" :for="preview?'preview-password':'password'">{{settings.login_copy.password_label}}</label>
     <div class="va-password"><input :id="preview?'preview-password':'password'" v-model="password" class="form-control" :placeholder="settings.login_copy.password_placeholder" :type="visible?'text':'password'" autocomplete="current-password" aria-required="true" :disabled="busy||preview"/>
     <button type="button" class="va-password-toggle" :disabled="busy||preview" :aria-pressed="visible" @click="visible=!visible">{{visible?settings.login_copy.hide_password:settings.login_copy.show_password}}</button></div></div>
     <div v-if="!preview" class="form-check mb-4"><input id="remember-me" v-model="remember" type="checkbox" class="form-check-input" :disabled="busy" @change="emit('updateRemember',remember)"/><label class="form-check-label" for="remember-me">记住用户名</label></div>
     <div v-if="showCaptcha" class="mb-4"><label class="form-label" for="captcha-answer">验证码</label>
     <div class="va-captcha"><input id="captcha-answer" v-model="captchaAnswer" class="form-control" placeholder="请输入验证码" maxlength="6" autocomplete="off" inputmode="numeric" aria-required="true" :disabled="busy"/>
     <button type="button" class="va-captcha-img" tabindex="-1" title="点击刷新验证码" :disabled="busy" @click="emit('refreshCaptcha')">
      <img v-if="captchaImageSrc" :src="captchaImageSrc" alt="验证码" class="va-captcha-img__pic"/>
      <span v-else class="va-captcha-img__placeholder">{{captchaLoading?'加载中…':'点击获取'}}</span>
     </button></div></div>
    </template>
    <button class="btn btn-primary w-100 va-submit" :disabled="busy||preview">{{busy?settings.login_copy.submitting:showTotp?'验证':settings.login_copy.submit}}</button>
    <p v-if="settings.login_copy.help" class="va-help">{{settings.login_copy.help}}</p>
   </form>
   <p v-if="settings.login_copy.footer" class="va-footer">{{settings.login_copy.footer}}</p>
   <footer class="va-login-foot">
    <a :href="footerPoweredByUrl(settings)" target="_blank" rel="noopener" class="va-login-foot-link">{{settings.footer_powered_by_name||settings.site_name||'Powered by'}}</a><span v-if="settings.footer_icp" class="va-login-foot-sep">·</span><span v-if="settings.footer_icp">{{settings.footer_icp}}</span><span class="va-login-foot-sep">·</span><span>微信：{{footerWechat(settings)}}</span>
   </footer>
  </section>
 </div>
</template>
<style scoped>
.va-login{min-height:100dvh;display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);background:#fff;color:#182433}
.va-intro{padding:clamp(32px,6vw,100px);background:radial-gradient(ellipse at 30% 20%,#2f6a5344,transparent 70%),#143d32;color:#fff;display:flex;flex-direction:column;justify-content:center;position:relative;overflow:hidden}
.va-badge{align-self:start;padding:8px 12px;border:1px solid #ffffff35;border-radius:6px;font-size:11px;letter-spacing:.12em;white-space:pre-wrap;overflow-wrap:anywhere;margin-bottom:40px}
.va-intro h1{font-size:clamp(32px,4vw,64px);line-height:1.12;letter-spacing:-.035em;font-weight:650;margin:0 0 20px;white-space:pre-wrap;overflow-wrap:anywhere}
.va-tagline{font-size:clamp(16px,1.6vw,22px);color:#d4e5dd;white-space:pre-wrap;overflow-wrap:anywhere}
.va-graphic{height:180px;border:1px solid #ffffff20;border-radius:16px;margin:34px 0;background:linear-gradient(90deg,#ffffff08 1px,transparent 1px),linear-gradient(#ffffff08 1px,transparent 1px);background-size:30px 30px;position:relative;overflow:hidden}
.va-target{position:absolute;width:95px;height:95px;left:calc(50% - 48px);top:40px;transform:rotate(-8deg)}
.va-target i{position:absolute;width:22px;height:22px;border-color:#92bea3;border-style:solid}.va-target i:nth-child(1){top:0;left:0;border-width:2px 0 0 2px}.va-target i:nth-child(2){top:0;right:0;border-width:2px 2px 0 0}.va-target i:nth-child(3){bottom:0;left:0;border-width:0 0 2px 2px}.va-target i:nth-child(4){bottom:0;right:0;border-width:0 2px 2px 0}.va-target span{position:absolute;inset:25px;border:1px solid #ffffff50;border-radius:50%}.va-scan{position:absolute;left:0;right:0;top:50%;height:1px;background:linear-gradient(90deg,transparent,#a0d3b4,transparent)}
.va-description{font-size:14px;line-height:1.9;opacity:.75;margin:0;white-space:pre-wrap;overflow-wrap:anywhere}
.va-form{display:flex;flex-direction:column;justify-content:center;padding:clamp(24px,5vw,80px);gap:40px;min-width:0;min-height:0}.va-form form{max-width:400px;width:100%;margin:auto}.va-login-foot{margin-top:auto;max-width:400px;width:100%;text-align:center;padding-top:20px;font-size:12px;color:#84918c;white-space:pre-wrap;overflow-wrap:anywhere}.va-eyebrow{font-size:13px;color:#84918c;margin-bottom:10px;white-space:pre-wrap;overflow-wrap:anywhere}.va-form h2{font-size:30px;margin-bottom:32px;white-space:pre-wrap;overflow-wrap:anywhere}.form-label{white-space:pre-wrap;overflow-wrap:anywhere}.form-control{min-height:46px}.va-password{display:flex;align-items:stretch;border:1px solid var(--tblr-border-color,#dadfe5);border-radius:var(--tblr-border-radius,4px);overflow:hidden}.va-password .form-control{border:0;min-width:0;flex:1}.va-password:focus-within{outline:2px solid var(--tblr-primary);outline-offset:1px}.va-password-toggle{border:0;background:#f7f9f8;color:var(--tblr-primary);font-size:12px;padding:8px 12px;max-width:45%;white-space:pre-wrap;overflow-wrap:anywhere}.va-captcha{display:flex;align-items:stretch;border:1px solid var(--tblr-border-color,#dadfe5);border-radius:var(--tblr-border-radius,4px);overflow:hidden}.va-captcha .form-control{border:0;min-width:0;flex:1}.va-captcha:focus-within{outline:2px solid var(--tblr-primary);outline-offset:1px}.va-captcha-img{border:0;background:#f7f9f8;cursor:pointer;padding:0;display:flex;align-items:center;justify-content:center;min-width:96px;max-width:130px;flex:0 0 96px;overflow:hidden}.va-captcha-img:disabled{cursor:default}.va-captcha-img__pic{display:block;height:38px;width:auto;max-width:100%}.va-captcha-img__placeholder{font-size:11px;color:#84918c;padding:0 8px;text-align:center;white-space:pre-wrap;overflow-wrap:anywhere}.va-totp-back{background:transparent;border:0;color:var(--tblr-primary);font-size:13px;padding:8px 0;cursor:pointer;margin-bottom:16px;white-space:pre-wrap;overflow-wrap:anywhere}.va-totp-back:disabled{cursor:default;opacity:.6}.va-submit{min-height:46px;white-space:pre-wrap;overflow-wrap:anywhere}.va-help,.va-footer{font-size:12px;line-height:1.8;color:#84918c;white-space:pre-wrap;overflow-wrap:anywhere}.va-help{margin:24px 0 0}.va-footer{max-width:400px;width:100%;margin:0}.alert{white-space:pre-wrap;overflow-wrap:anywhere}
.va-login-foot-link{color:#4a9d7a;text-decoration:none}.va-login-foot-link:hover{text-decoration:underline}
.va-login-foot-sep{margin:0 8px;color:#c3ccc7}
.va-login--preview{min-height:0;grid-template-columns:1fr;border:1px solid #e1e6e4;border-radius:12px;overflow:hidden}.va-login--preview .va-intro{padding:24px}.va-login--preview h1{font-size:26px}.va-login--preview .va-badge{margin-bottom:20px}.va-login--preview .va-graphic{height:80px;margin:12px 0}.va-login--preview .va-target{height:48px;width:48px;top:15px}.va-login--preview .va-form{padding:24px;gap:20px}.va-login--preview .va-form h2{font-size:24px}.va-login--preview .va-tagline{font-size:16px}
@media(max-width:760px){.va-login{grid-template-columns:1fr}.va-intro{padding:28px 24px}.va-intro h1{font-size:32px;margin-bottom:10px}.va-badge{margin-bottom:16px}.va-graphic{display:none}.va-description{margin-top:8px}.va-form{padding:32px 24px;justify-content:start}.va-form h2{font-size:26px}}
</style>
