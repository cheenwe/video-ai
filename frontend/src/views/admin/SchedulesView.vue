<script setup lang="ts">
import {onMounted,onUnmounted,ref} from 'vue'
import {request,fmtDate} from '@/api/client'
import {useAuthStore} from '@/stores/auth'
import {useToast} from '@/composables/useToast'
import CdModal from '@/components/CdModal.vue'
const auth=useAuthStore(),toast=useToast()
type Job={id:number;name:string;builtin_key:string;cron:string;timezone:string;enabled:number;is_builtin:number;last_status:string;last_error:string;next_run_at?:string;run_count:number}
type Run={id:number;status:string;trigger:string;started_at:string;duration_ms:number;detail:string}
const jobs=ref<Job[]>([]),runs=ref<Run[]>([]),loading=ref(false),busy=ref(false),error=ref(''),show=ref(false),showRuns=ref(false),editing=ref<number|null>(null)
const actions:Record<string,string>={database_check:'数据库检查',session_cleanup:'过期会话清理',backup:'数据库与配置备份'}
const statuses:Record<string,string>={running:'运行中',success:'成功',failed:'失败',skipped:'跳过'}
const form=ref({name:'',builtin_key:'database_check',cron:'0 3 * * *',timezone:'Asia/Shanghai',enabled:0})
async function load(){if(loading.value)return;loading.value=true;try{jobs.value=(await request<{items:Job[]}>('/api/admin/schedule/jobs')).items;error.value=''}catch(e){error.value=String(e)}finally{loading.value=false}}
function edit(j?:Job){editing.value=j?.id??null;form.value=j?{name:j.name,builtin_key:j.builtin_key,cron:j.cron,timezone:j.timezone,enabled:j.enabled}:{name:'',builtin_key:'database_check',cron:'0 3 * * *',timezone:'Asia/Shanghai',enabled:0};show.value=true}
async function save(){busy.value=true;try{const body=editing.value?{name:form.value.name,cron:form.value.cron,timezone:form.value.timezone,enabled:form.value.enabled}:{...form.value,job_type:'builtin'};await request('/api/admin/schedule/jobs'+(editing.value?'/'+editing.value:''),{method:editing.value?'PUT':'POST',body:JSON.stringify(body)});show.value=false;toast.show('任务已保存');await load()}catch(e){toast.show(String(e))}finally{busy.value=false}}
async function act(j:Job,action:string){busy.value=true;try{if(action==='run'){await request(`/api/admin/schedule/jobs/${j.id}/run`,{method:'POST'});toast.show('已开始执行')}else{await request(`/api/admin/schedule/jobs/${j.id}`,{method:'PUT',body:JSON.stringify({enabled:j.enabled?0:1})});toast.show('任务状态已更新')}await load()}catch(e){toast.show(String(e))}finally{busy.value=false}}
async function history(j:Job){try{runs.value=(await request<{items:Run[]}>(`/api/admin/schedule/jobs/${j.id}/runs`)).items;showRuns.value=true}catch(e){toast.show(String(e))}}
async function remove(j:Job){if(!confirm(`删除任务「${j.name}」？执行记录仍保留。`))return;busy.value=true;try{await request(`/api/admin/schedule/jobs/${j.id}`,{method:'DELETE'});await load()}catch(e){toast.show(String(e))}finally{busy.value=false}}
let timer:ReturnType<typeof setInterval>|undefined
onMounted(()=>{load();timer=setInterval(()=>{if(!show.value&&!busy.value)load()},5000)})
onUnmounted(()=>{if(timer)clearInterval(timer)})
</script>
<template>
 <div class="d-flex justify-content-between mb-3"><div><h3>计划任务</h3><p class="text-secondary mb-0">按任务时区执行；重启后跳过错过的执行，运行中的任务不重复启动。</p></div><div class="d-flex gap-2"><button class="btn" :disabled="loading" @click="load">刷新</button><button v-if="auth.can('schedule:create')" class="btn btn-primary" @click="edit()">新建任务</button></div></div>
 <div v-if="error" class="alert alert-danger" role="alert">{{error}}</div>
 <div class="card"><div class="table-responsive"><table class="table table-vcenter"><thead><tr><th>任务 / 动作</th><th>Cron / 时区</th><th>计划状态</th><th>下次执行</th><th>最近结果</th><th>操作</th></tr></thead><tbody>
 <tr v-for="j in jobs" :key="j.id"><td>{{j.name}}<small class="d-block text-secondary">{{actions[j.builtin_key]||'旧版动作（需重新配置）'}}</small></td><td><code>{{j.cron}}</code><small class="d-block">{{j.timezone}}</small></td><td>{{j.enabled?'已启用':'已停用'}}</td><td>{{j.next_run_at?fmtDate(j.next_run_at):'—'}}</td><td>{{statuses[j.last_status]||'未执行'}}<small v-if="j.last_error" class="d-block text-danger">{{j.last_error}}</small></td><td><div class="d-flex gap-1 flex-wrap"><button v-if="auth.can('schedule:update') && actions[j.builtin_key]" class="btn btn-sm" :disabled="busy||j.last_status==='running'" @click="edit(j)">编辑</button><button v-if="auth.can('schedule:update') && actions[j.builtin_key]" class="btn btn-sm" :disabled="busy||j.last_status==='running'" @click="act(j,'toggle')">{{j.enabled?'停用':'启用'}}</button><button v-if="auth.can('schedule:run')" class="btn btn-sm" :disabled="busy||!j.enabled||j.last_status==='running'" @click="act(j,'run')">立即执行</button><button class="btn btn-sm" @click="history(j)">记录</button><button v-if="auth.can('schedule:delete')&&!j.is_builtin" class="btn btn-sm text-danger" :disabled="busy||j.last_status==='running'" @click="remove(j)">删除</button></div></td></tr>
 <tr v-if="!jobs.length"><td colspan="6" class="text-center py-4">{{loading?'正在加载…':'暂无任务'}}</td></tr>
 </tbody></table></div></div>
 <CdModal :show="show" @close="show=false" :title="editing?'编辑任务':'新建任务'"><form class="modal-body" id="schedule-form" @submit.prevent="save"><label class="form-label" for="job-name">名称</label><input id="job-name" v-model="form.name" required maxlength="100" class="form-control mb-3"/><label class="form-label" for="job-action">内置动作</label><select id="job-action" v-model="form.builtin_key" :disabled="!!editing" class="form-select mb-3"><option v-for="(label,key) in actions" :key="key" :value="key">{{label}}</option></select><label class="form-label" for="job-cron">Cron（分 时 日 月 周）</label><input id="job-cron" v-model="form.cron" required class="form-control mb-3"/><label class="form-label" for="job-zone">时区</label><input id="job-zone" v-model="form.timezone" required class="form-control mb-3" placeholder="Asia/Shanghai"/><label class="form-check"><input v-model="form.enabled" type="checkbox" :true-value="1" :false-value="0" class="form-check-input"/><span class="form-check-label">启用任务</span></label></form><template #footer><button class="btn" @click="show=false">取消</button><button type="submit" form="schedule-form" class="btn btn-primary" :disabled="busy">保存任务</button></template></CdModal>
 <CdModal :show="showRuns" @close="showRuns=false" title="最近100次执行记录"><div class="modal-body"><div v-if="!runs.length" class="text-secondary">暂无执行记录</div><div v-for="r in runs" :key="r.id" class="border-bottom py-2"><div>{{fmtDate(r.started_at)}} · {{r.trigger==='manual'?'手动':'计划'}} · {{statuses[r.status]||r.status}} · {{r.duration_ms}} ms</div><small>{{r.detail}}</small></div></div></CdModal>
</template>
