// @vitest-environment jsdom
import {it,expect} from 'vitest'
import {createApp,h,nextTick,withDirectives} from 'vue'
import {createPinia} from 'pinia'
import {permission} from './permission'
import {useAuthStore} from '@/stores/auth'
import type {UserInfo} from '@/api/auth'
it('updates actions when grants change and reserves privileged actions',async()=>{
 const pinia=createPinia(),root=document.createElement('div');document.body.appendChild(root)
 const app=createApp({render(){return h('div',[withDirectives(h('button',{id:'edit'},'edit'),[[permission,'project:update']]),withDirectives(h('button',{id:'roles'},'roles'),[[permission,['role:update','@super']]])])}});app.use(pinia)
 const auth=useAuthStore(pinia);auth.user={is_superadmin:false,permissions:['project:update','role:update']} as UserInfo
 app.mount(root);await nextTick();expect((root.querySelector('#edit') as HTMLElement).style.display).not.toBe('none');expect((root.querySelector('#roles') as HTMLElement).style.display).toBe('none')
 auth.user.permissions=[];await nextTick();expect((root.querySelector('#edit') as HTMLElement).style.display).toBe('none')
 auth.user.is_superadmin=true;await nextTick();expect((root.querySelector('#roles') as HTMLElement).style.display).not.toBe('none')
 app.unmount();root.remove()
})
