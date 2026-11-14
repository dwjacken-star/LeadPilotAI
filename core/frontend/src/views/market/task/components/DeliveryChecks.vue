<template>
 <modal :title="t('emailVerification.details')" :width="1100" :footer="false">
  <n-alert type="info" class="mb-4">{{t('emailVerification.strictHint')}}</n-alert>
  <n-space class="mb-4"><n-select v-model:value="state" :options="options" style="width:200px" @update:value="()=>{page=1;load()}" /><n-button @click="load">{{t('common.actions.refresh')}}</n-button><n-button v-if="can('campaign:publish')" type="primary" @click="recheck">{{t('emailVerification.recheck')}}</n-button></n-space>
  <n-data-table :columns="columns" :data="rows" :loading="loading" :scroll-x="1000" :max-height="450" />
  <n-pagination v-model:page="page" :page-size="20" :item-count="total" @update:page="load" class="mt-4" />
 </modal>
</template>
<script setup lang="ts">
import {useModal} from '@/hooks/modal/useModal'
import {workspace,can} from '@/features/workspace/api'
import {confirm} from '@/utils'
import {formatWorkspaceCell} from '@/features/workspace/display'
const {t,te}=useI18n()
const rows=ref<any[]>([]),page=ref(1),total=ref(0),loading=ref(false),state=ref(''),taskID=ref(0)
let timer:ReturnType<typeof setTimeout>|undefined,serial=0,opened=false
const states=['','validation_hold','failed','unknown','retry','accepted','skipped']
const options=computed(()=>states.map(value=>({value,label:value===''?t('emailVerification.all'):value==='validation_hold'?t('emailVerification.held'):formatWorkspaceCell('deliveries','delivery_state',{delivery_state:value},t)})))
const columns=computed(()=>[
 {key:'recipient',title:t('workspaceCopy.w174'),minWidth:210},
 {key:'delivery_state',title:t('workspaceCopy.w175'),minWidth:120,render:(row:any)=>row.delivery_state==='validation_hold'?t('emailVerification.held'):row.remote_status==='bounced'?t('workspaceCopy.w398'):formatWorkspaceCell('deliveries','delivery_state',row,t)},
 {key:'verification_code',title:t('emailVerification.result'),minWidth:220,render:(row:any)=>row.verification_code&&te('emailVerification.codes.'+row.verification_code)?t('emailVerification.codes.'+row.verification_code):'—'},
 {key:'last_error',title:t('workspaceCopy.w177'),minWidth:300,ellipsis:{tooltip:true},render:(row:any)=>row.remote_status==='bounced'&&row.remote_error?row.remote_error:formatWorkspaceCell('deliveries','last_error',row,t)},
 {key:'verification_checked_at',title:t('emailVerification.checkedAt'),minWidth:170,render:(row:any)=>row.verification_checked_at?new Date(row.verification_checked_at).toLocaleString():'—'}
])
async function load(){clearTimeout(timer);if(!opened)return;const request=++serial;loading.value=true;try{const data=await workspace('list',{type:'deliveries',id:taskID.value,page:page.value,page_size:20,data:{delivery_state:state.value}});if(request===serial){rows.value=data.list||[];total.value=data.total||0}}catch{/* Request interceptor displays failures. */}finally{if(request===serial){loading.value=false;if(opened)timer=setTimeout(load,3000)}}}
function recheck(){confirm({title:t('emailVerification.recheck'),content:t('emailVerification.releaseConfirm'),onConfirm:async()=>{await workspace('task_control',{id:taskID.value,data:{operation:'recheck'}});await load()}})}
const [Modal,modalApi]=useModal({onChangeState(show){opened=show;serial++;clearTimeout(timer);if(show){taskID.value=modalApi.getState<{taskID:number}>().taskID;state.value='';page.value=1;void load()}}})
onBeforeUnmount(()=>{opened=false;serial++;clearTimeout(timer)})
</script>
