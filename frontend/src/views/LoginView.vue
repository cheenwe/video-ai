<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {useRoute, useRouter} from 'vue-router'
import {useAuthStore} from '@/stores/auth'
import {useBrandingStore} from '@/stores/branding'
import {ApiError} from '@/api/client'
import * as authApi from '@/api/auth'
import {resolvePostLoginPath} from '@/utils/authRedirect'
import {deobfuscateCaptchaImage} from '@/utils/captcha'
import {loadSavedLogin,saveLogin} from '@/composables/useLoginRemember'
import type {LoginCopyKey} from '@/types/loginCopy'
import LoginPresentation from '@/components/LoginPresentation.vue'
const auth=useAuthStore(),branding=useBrandingStore(),route=useRoute(),router=useRouter()
const captchaEnabled=ref(false),captchaForced=ref(false),captchaId=ref(''),captchaImage=ref(''),captchaLoading=ref(false)
const savedLogin=loadSavedLogin()
const rememberUsername=savedLogin?.username||''
const rememberChecked=!!savedLogin
const loginStep=ref<'credentials'|'totp'>('credentials'),tempToken=ref('')
const errorKey=ref<LoginCopyKey|null>(null),errorMsg=ref(''),successMsg=ref('')
const error=computed(()=>errorKey.value?branding.settings.login_copy[errorKey.value]:errorMsg.value)
const showCaptcha=computed(()=>captchaEnabled.value||captchaForced.value)
function isCaptchaRequiredPayload(data:unknown):boolean{return!!data&&typeof data==='object'&&(data as{captcha_required?:boolean}).captcha_required===true}
async function refreshCaptcha(){if(!showCaptcha.value)return;captchaLoading.value=true;try{const c=await authApi.fetchCaptcha();captchaId.value=c.captcha_id;const raw=(c.image_base64||'').trim();captchaImage.value=c.obfuscated&&raw&&!raw.startsWith('data:')?deobfuscateCaptchaImage(c.captcha_id,raw):raw}catch{}finally{captchaLoading.value=false}}
async function syncCaptchaForUsername(username:string){const name=username.trim();if(!name||captchaEnabled.value)return;try{const cfg=await authApi.fetchAuthConfig(name);if(cfg.captcha_enabled){const wasForced=captchaForced.value;captchaForced.value=true;if(!wasForced||!captchaId.value)await refreshCaptcha()}}catch{}}
function mapErrorKey(e:unknown):LoginCopyKey{if(e instanceof ApiError){if(e.code===429)return'rate_limited';if(e.code>=500)return'service_unavailable';if([400,401,403,422].includes(e.code))return'invalid_credentials'}return'generic_error'}
async function submitLogin(username:string,password:string,captchaAnswer?:string,remember?:boolean){errorKey.value=null;errorMsg.value='';successMsg.value='';if(!username.trim()){errorKey.value='username_required';return}if(!password){errorKey.value='password_required';return}try{const res=await auth.login(username.trim(),password,{captcha_id:showCaptcha.value?captchaId.value:undefined,captcha_answer:showCaptcha.value?(captchaAnswer||'').trim():undefined});saveLogin(username.trim(),password,!!remember);if(res&&'need_totp'in res&&res.need_totp){loginStep.value='totp';tempToken.value=res.temp_token;successMsg.value='请输入动态验证码';captchaForced.value=false;return}captchaForced.value=false;await router.push(auth.isAdmin?resolvePostLoginPath(route.query.redirect):'/profile')}catch(e){errorKey.value=mapErrorKey(e);if(e instanceof ApiError&&isCaptchaRequiredPayload(e.data))captchaForced.value=true;if(showCaptcha.value||captchaForced.value)await refreshCaptcha()}}
async function submitTotp(code:string){errorKey.value=null;errorMsg.value='';if(!code.trim()){errorMsg.value='请输入动态验证码';return}try{await auth.verifyTOTP(tempToken.value,code.trim());await router.push(auth.isAdmin?resolvePostLoginPath(route.query.redirect):'/profile')}catch(e){errorKey.value=mapErrorKey(e)}}
function back(){loginStep.value='credentials';tempToken.value='';successMsg.value='';errorMsg.value='';errorKey.value=null}
onMounted(async()=>{try{const cfg=await authApi.fetchAuthConfig();captchaEnabled.value=cfg.captcha_enabled;if(captchaEnabled.value)await refreshCaptcha()}catch{}})
</script>
<template><LoginPresentation :settings="branding.settings" :busy="auth.loading" :error="error" :captcha-visible="showCaptcha" :captcha-image="captchaImage" :captcha-loading="captchaLoading" :totp-step="loginStep==='totp'" :notice="successMsg" :remember-username="rememberUsername" :remember-checked="rememberChecked" @submit="submitLogin" @refresh-captcha="refreshCaptcha" @submit-totp="submitTotp" @back="back" @username-blur="syncCaptchaForUsername"/></template>
