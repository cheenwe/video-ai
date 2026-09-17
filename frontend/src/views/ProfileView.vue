<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import * as api from "@/api/auth";
import { ApiError, clearToken } from "@/api/client";
import { useToast } from "@/composables/useToast";
const auth = useAuthStore(),
  router = useRouter(),
  toast = useToast(),
  name = ref(""),
  zone = ref("Asia/Shanghai"),
  old = ref(""),
  pwd = ref(""),
  confirmPwd = ref(""),
  busy = ref(false);
const totpEnabled = ref(false),
  totpEnforced = ref(false),
  totpQr = ref(""),
  totpCode = ref(""),
  totpLoading = ref(false);
onMounted(() => {
  name.value = auth.user?.real_name || "";
  zone.value = auth.user?.timezone || "Asia/Shanghai";
  loadTotp();
});
async function save() {
  busy.value = true;
  try {
    auth.user = await api.updateProfile({
      real_name: name.value,
      timezone: zone.value,
    });
    toast.show("个人资料已保存");
  } catch (e) {
    toast.show(String(e));
  } finally {
    busy.value = false;
  }
}
async function change() {
  if (pwd.value !== confirmPwd.value) {
    toast.show("两次密码不一致");
    return;
  }
  busy.value = true;
  try {
    await api.changePassword(old.value, pwd.value);
    clearToken();
    auth.user = null;
    await router.push("/login");
    toast.show("密码已修改，请重新登录");
  } catch (e) {
    toast.show(String(e));
  } finally {
    busy.value = false;
  }
}
async function loadTotp() {
  try {
    const s = await api.totpStatus();
    totpEnabled.value = s.enabled;
    totpEnforced.value = s.enforced;
  } catch {
    /* ignore */
  }
}
async function setupTotp() {
  totpLoading.value = true;
  try {
    const res = await api.totpSetup();
    totpQr.value = res.qr_url;
    toast.show("请使用验证器扫码，再输入验证码确认");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "设置失败");
  } finally {
    totpLoading.value = false;
  }
}
async function confirmTotp() {
  totpLoading.value = true;
  try {
    await api.totpConfirm(totpCode.value.trim());
    totpQr.value = "";
    totpCode.value = "";
    totpEnabled.value = true;
    if (auth.user) {auth.user.totp_enabled = true;auth.user.must_setup_totp=false;}
    toast.show("双因素认证已启用");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "确认失败");
  } finally {
    totpLoading.value = false;
  }
}
async function disableTotp() {
  totpLoading.value = true;
  try {
    await api.totpDisable(totpCode.value.trim());
    totpCode.value = "";
    totpEnabled.value = false;
    if (auth.user) auth.user.totp_enabled = false;
    toast.show("双因素认证已关闭");
  } catch (e) {
    toast.show(e instanceof ApiError ? e.message : "关闭失败");
  } finally {
    totpLoading.value = false;
  }
}
</script>
<template>
  <div class="row g-4">
 <div v-if="auth.user?.must_setup_totp" class="col-12 alert alert-warning">管理员要求启用双因素认证，请先完成下方绑定，再访问管理功能。</div>
    <div class="col-lg-6">
      <form class="card" @submit.prevent="save">
        <div class="card-header"><h3 class="card-title">个人资料</h3></div>
        <div class="card-body">
          <label class="form-label">用户名</label
          ><input
            :value="auth.user?.username"
            class="form-control mb-3"
            disabled
          /><label class="form-label">姓名</label
          ><input
            v-model="name"
            class="form-control mb-3"
            maxlength="64"
          /><label class="form-label">时区</label
          ><select v-model="zone" class="form-select">
            <option
              v-for="z in [
                'Asia/Shanghai',
                'UTC',
                'Asia/Hong_Kong',
                'Asia/Tokyo',
                'America/New_York',
                'Europe/London',
              ]"
              :key="z"
            >
              {{ z }}
            </option>
          </select>
        </div>
        <div class="card-footer">
          <button class="btn btn-primary" :disabled="busy">保存资料</button>
        </div>
      </form>
    </div>
    <div class="col-lg-6">
      <form class="card" @submit.prevent="change">
        <div class="card-header"><h3 class="card-title">修改密码</h3></div>
        <div class="card-body">
          <label class="form-label">原密码</label
          ><input
            v-model="old"
            type="password"
            autocomplete="current-password"
            class="form-control mb-3"
            required
          /><label class="form-label">新密码</label
          ><input
            v-model="pwd"
            type="password"
            autocomplete="new-password"
            class="form-control mb-3"
            required
            minlength="8"
          /><label class="form-label">确认新密码</label
          ><input
            v-model="confirmPwd"
            type="password"
            autocomplete="new-password"
            class="form-control"
            required
          />
        </div>
        <div class="card-footer">
          <button class="btn btn-primary" :disabled="busy">
            修改并重新登录
          </button>
        </div>
      </form>
    </div>
    <div class="col-12">
      <div class="card">
        <div class="card-header"><h3 class="card-title">双因素认证 (2FA)</h3></div>
        <div class="card-body">
          <p class="text-secondary">
            启用后，登录时除账号密码外还需输入验证器 App（如 Google
            Authenticator）生成的动态验证码。
          </p>
          <div
            class="d-flex align-items-center justify-content-between flex-wrap gap-2 mb-3"
          >
            <div>
              <div class="fw-medium">当前状态</div>
              <div class="small mt-1">
                <span :class="totpEnabled ? 'text-success' : 'text-secondary'">
                  {{ totpEnabled ? "已启用" : "未启用" }}
                </span>
                <span v-if="totpEnforced" class="text-warning ms-2">
                  管理员已强制开启，无法关闭
                </span>
              </div>
            </div>
            <button
              v-if="!totpEnabled && !totpQr"
              type="button"
              class="btn btn-outline-primary"
              :disabled="totpLoading"
              @click="setupTotp"
            >
              {{ totpLoading ? "生成中…" : "生成绑定二维码" }}
            </button>
          </div>
          <template v-if="!totpEnabled && totpQr">
            <div class="text-center mb-3">
              <img :src="totpQr" alt="TOTP QR" />
              <p class="text-secondary small mt-2 mb-0">请使用验证器扫码</p>
            </div>
            <label class="form-label">验证码</label>
            <div class="d-flex flex-wrap gap-2 mb-2">
              <input
                v-model="totpCode"
                class="form-control"
                style="max-width: 12rem"
                placeholder="6 位数字"
                maxlength="8"
                autocomplete="one-time-code"
              />
              <button
                type="button"
                class="btn btn-primary"
                :disabled="totpLoading || !totpCode.trim()"
                @click="confirmTotp"
              >
                确认启用
              </button>
            </div>
          </template>
          <template v-else-if="totpEnabled && !totpEnforced">
            <label class="form-label">验证码</label>
            <div class="d-flex flex-wrap gap-2 mb-2">
              <input
                v-model="totpCode"
                class="form-control"
                style="max-width: 12rem"
                placeholder="6 位数字"
                maxlength="8"
                autocomplete="one-time-code"
              />
              <button
                type="button"
                class="btn btn-outline-danger"
                :disabled="totpLoading || !totpCode.trim()"
                @click="disableTotp"
              >
                关闭 2FA
              </button>
            </div>
            <div class="form-text">关闭前需输入当前动态验证码</div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
