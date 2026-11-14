<template>
 <section class="model-registry">
  <header><div><h1>{{t('ai_registry.title')}}</h1><p>{{t('ai_registry.hint')}}</p></div><n-space><n-button :loading="loading" @click="load">{{t('growth.refresh')}}</n-button><n-button type="primary" :loading="saving" @click="save">{{t('growth.save')}}</n-button></n-space></header>
  <n-alert type="info">{{t('ai_registry.migration_hint')}}</n-alert>
  <n-card :title="t('ai_registry.defaults')" class="registry-card"><div class="default-grid"><n-form-item v-for="kind in kinds" :key="kind" :label="t('ai_registry.kinds.'+kind)"><n-select v-model:value="registry.defaults[kind]" clearable filterable :options="choices(kind)" /></n-form-item></div></n-card>
  <n-space class="registry-toolbar"><n-button @click="addProvider">{{t('ai_registry.add_platform')}}</n-button><n-select v-model:value="preset" :options="presets" style="width:230px" /></n-space>
  <n-card v-for="provider in registry.providers" :key="provider.id" :title="provider.name" class="registry-card">
   <template #header-extra><n-space><n-switch v-model:value="provider.enabled" /><n-button size="small" type="error" @click="removeProvider(provider)">{{t('common.actions.delete')}}</n-button></n-space></template>
   <n-form label-placement="top"><div class="provider-grid">
    <n-form-item :label="t('ai_registry.platform_name')"><n-input v-model:value="provider.name" /></n-form-item>
    <n-form-item :label="t('ai_registry.protocol')"><n-select v-model:value="provider.format" :options="formats" /></n-form-item>
    <n-form-item :label="t('growth.api_base')"><n-input v-model:value="provider.base_url" placeholder="https://api.example.com/v1" /></n-form-item>
    <n-form-item :label="t('growth.api_key')"><n-input v-model:value="provider.api_key" type="password" autocomplete="new-password" show-password-on="click" :placeholder="provider.has_key?t('growth.key_saved'):t('growth.key_required')" /></n-form-item>
   </div></n-form>
   <n-space><n-button :loading="discovering===provider.id" @click="discover(provider)">{{t('growth.refresh_models')}}</n-button><n-button @click="provider.models.push(newModel())">{{t('ai_registry.add_model')}}</n-button></n-space>
   <div v-for="(model,index) in provider.models" :key="index" class="registry-model">
    <n-switch v-model:value="model.enabled" />
    <n-input v-model:value="model.id" :placeholder="t('ai_registry.model_id')" /><n-input v-model:value="model.name" :placeholder="t('ai_registry.model_name')" />
    <n-select v-model:value="model.capabilities" multiple :options="capabilities" :placeholder="t('ai_registry.capabilities')" />
    <n-input-number v-model:value="model.max_output" :min="256" :max="131072" :placeholder="t('growth.max_output')" />
    <n-button type="error" secondary @click="provider.models.splice(index,1)">×</n-button>
   </div>
  </n-card>
  <n-card :title="t('ai_registry.budget')" class="registry-card"><div class="default-grid"><n-form-item :label="t('growth.daily_calls')"><n-input-number v-model:value="budget.daily_calls" :min="1" /></n-form-item><n-form-item :label="t('growth.daily_tokens')"><n-input-number v-model:value="budget.daily_tokens" :min="1000" /></n-form-item><n-form-item :label="t('growth.max_output')"><n-input-number v-model:value="budget.max_output" :min="256" :max="16000" /></n-form-item></div></n-card>
 </section>
</template>
<script setup lang="ts">
import {uiText} from '@/i18n/labels'
import {ref,computed,onMounted} from 'vue'
import {useI18n} from 'vue-i18n'
import {growth} from '@/features/growth/api'
import {Message} from '@/utils'
const {t}=useI18n(),registry=ref<any>({version:0,providers:[],defaults:{}}),budget=ref<any>({daily_calls:100,daily_tokens:200000,max_output:4000}),loading=ref(false),saving=ref(false),discovering=ref(''),preset=ref('custom')
const kinds=['text','chat','vision','image','embedding']
const capabilities=computed(()=>kinds.map(value=>({value,label:t('ai_registry.kinds.'+value)})))
const formats=computed(()=>[{value:'responses',label:uiText('ui.p9b7e851b6ec8')},{value:'chat_completions',label:uiText('ui.pfe15120ed512')}])
const presets=[{value:'custom',label:uiText('ui.pc41a16e6f524')},{value:'openai',label:uiText('ui.p5cf924a99cad')},{value:'qwen',label:'Qwen'},{value:'deepseek',label:'DeepSeek'},{value:'gemini',label:uiText('ui.p2f9610fee8e3')},{value:'kimi',label:'Kimi'},{value:'grok',label:'Grok'}]
const bases:Record<string,string>={openai:'https://api.openai.com/v1',qwen:'https://dashscope-intl.aliyuncs.com/compatible-mode/v1',deepseek:'https://api.deepseek.com',gemini:'https://generativelanguage.googleapis.com/v1beta/openai',kimi:'https://api.moonshot.ai/v1',grok:'https://api.x.ai/v1'}
function newModel(){return {id:'',name:'',enabled:true,capabilities:['text','chat'],max_output:4000}}
function addProvider(){registry.value.providers.push({id:'provider_'+crypto.randomUUID().replaceAll('-',''),name:presets.find(p=>p.value===preset.value)?.label||'',enabled:false,format:preset.value==='openai'?'responses':'chat_completions',base_url:bases[preset.value]||'',api_key:'',models:[]})}
function choices(kind:string){return registry.value.providers.flatMap((p:any)=>p.enabled&&p.base_url&&(p.api_key?.trim()||p.has_key)?(p.models||[]).filter((m:any)=>m.enabled&&m.capabilities.some((cap:string)=>cap===kind||(kind==='text'&&['chat','vision'].includes(cap)))).map((m:any)=>({value:'ai::'+p.id+'::'+m.id,label:p.name+' · '+(m.name||m.id)})):[])}
function removeProvider(p:any){if(!window.confirm(t('ai_registry.confirm_remove')))return;registry.value.providers=registry.value.providers.filter((x:any)=>x.id!==p.id);for(const kind of kinds)if(registry.value.defaults[kind]?.startsWith('ai::'+p.id+'::'))registry.value.defaults[kind]=''}
async function load(){loading.value=true;try{const [r,b]=await Promise.all([growth('model_registry'),growth('settings')]);registry.value=r;budget.value=b.config;for(const p of r.providers)p.api_key=''}finally{loading.value=false}}
async function discover(p:any){discovering.value=p.id;try{const models=await growth('discover_registry_models',{data:p});const existing=new Set(p.models.map((m:any)=>m.id));p.models.push(...models.filter((m:any)=>!existing.has(m.id)))}finally{discovering.value=''}}
async function save(){if(saving.value)return;if(!budget.value.daily_calls||budget.value.daily_tokens<1000||budget.value.max_output<256||budget.value.max_output>16000){Message.error(t('ai_registry.budget'));return};saving.value=true;try{for(const kind of kinds)if(!choices(kind).some((m:any)=>m.value===registry.value.defaults[kind]))registry.value.defaults[kind]='';await growth('save_model_registry',{data:registry.value});await growth('save_settings',{data:budget.value});await load();Message.success(t('growth.settings_saved'))}finally{saving.value=false}}
onMounted(()=>void load().catch(()=>{/* Request interceptor displays errors. */}))
</script>
<style scoped>.model-registry{max-width:1500px;margin:auto;padding:24px}.model-registry header{display:flex;justify-content:space-between;gap:20px;align-items:center;margin-bottom:20px}.model-registry h1{font-size:25px;margin:0 0 8px}.model-registry header p{color:var(--color-text-3);line-height:1.7}.registry-card{margin:18px 0}.registry-toolbar{margin-top:18px}.default-grid,.provider-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px 20px}.provider-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.registry-model{display:grid;grid-template-columns:40px minmax(150px,1fr) minmax(140px,1fr) minmax(220px,1.5fr) 145px 36px;gap:10px;align-items:center;padding:12px 0;border-bottom:1px solid var(--color-border-1)}@media(max-width:1100px){.registry-model{grid-template-columns:40px 1fr 1fr}.default-grid{grid-template-columns:1fr 1fr}}@media(max-width:650px){.model-registry{padding:16px}.model-registry header{flex-direction:column;align-items:flex-start}.provider-grid,.default-grid{grid-template-columns:1fr}.registry-model{grid-template-columns:1fr}}</style>
