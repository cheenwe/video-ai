import { shallowRef, watchEffect, type ObjectDirective, type ShallowRef } from 'vue'
import { useAuthStore } from '@/stores/auth'
type Rule = string | string[]
const states = new WeakMap<HTMLElement,{rule:ShallowRef<Rule>;stop:()=>void}>()
// Presentation only: every action remains authorized again by the backend.
export const permission: ObjectDirective<HTMLElement,Rule> = {
 beforeMount(el,binding) {
  const auth=useAuthStore(),rule=shallowRef(binding.value),display=el.style.display
  const stop=watchEffect(()=>{
   const rules=Array.isArray(rule.value)?rule.value:[rule.value]
   const allowed=rules.every(code=>code==='@super'?auth.isSuperAdmin:auth.can(code))
   el.style.display=allowed?display:'none'
  })
  states.set(el,{rule,stop})
 },
 updated(el,binding){const state=states.get(el);if(state)state.rule.value=binding.value},
 beforeUnmount(el){states.get(el)?.stop();states.delete(el)},
}
