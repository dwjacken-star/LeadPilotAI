<template>
 <div class="ai-model-choice" :style="$attrs.style as any">
  <n-select :value="platform" :options="platforms" :disabled="disabled" :loading="loading" filterable :placeholder="t('ai_registry.platform')" :aria-label="t('ai_registry.platform')" @update:value="choosePlatform" @focus="load" />
  <n-select :value="normalized" :options="available" :disabled="disabled || !platform" :loading="loading" filterable clearable :placeholder="t('ai_registry.model')" :aria-label="t('ai_registry.model')" @update:value="value=$event" @focus="load" />
 </div>
</template>
<script setup lang="ts">
import {computed,ref,onMounted,onBeforeUnmount,watch} from 'vue'
import {useI18n} from 'vue-i18n'
import {growth} from '@/features/growth/api'
defineOptions({inheritAttrs:false})
const props=withDefaults(defineProps<{kind?:string;disabled?:boolean;options?:any[]}>(),{kind:'text',disabled:false})
const value=defineModel<string|null>('value'),{t}=useI18n(),models=ref<any[]>([]),aliases=ref<Record<string,string>>({}),loading=ref(false),chosen=ref<string|null>(null)
const normalized=computed(()=>aliases.value[value.value||'']||value.value||null)
const platform=computed(()=>{const match=models.value.find(m=>m.value===normalized.value);return match?.provider_id||chosen.value})
const platforms=computed(()=>[...new Map(models.value.map(m=>[m.provider_id,{value:m.provider_id,label:m.provider_name}])).values()])
const available=computed(()=>models.value.filter(m=>m.provider_id===platform.value).map(m=>({value:m.value,label:m.model_name||m.model_id})))
function choosePlatform(id:string){chosen.value=id;value.value=models.value.find(m=>m.provider_id===id)?.value||null}
let request=0
async function load(){const current=++request;loading.value=true;try{const r=await growth('model_catalog',{data:{kind:props.kind}});if(current!==request)return;models.value=(r.models||[]).filter((m:any)=>!m.disabled);aliases.value=r.legacy_aliases||{};if(!value.value&&!chosen.value&&r.model){const preferred=models.value.find(m=>m.value===r.model);if(preferred){chosen.value=preferred.provider_id;value.value=preferred.value}}}catch{if(current===request)models.value=[]}finally{if(current===request)loading.value=false}}
defineExpose({reload:load})
onBeforeUnmount(()=>request++)
watch(()=>props.kind,()=>void load());onMounted(()=>void load())
</script>
<style scoped>.ai-model-choice{display:grid;grid-template-columns:minmax(140px,.8fr) minmax(180px,1.2fr);gap:10px;min-width:0;width:100%}@media(max-width:650px){.ai-model-choice{grid-template-columns:1fr}}</style>
