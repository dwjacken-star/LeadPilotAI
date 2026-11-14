<template>
 <section class="contact-import-panel">
  <h3>{{t('importPreview.title')}}</h3>
  <n-alert type="info" class="preview-tools">{{t('emailVerification.importHint')}}</n-alert>
  <n-form-item :label="t('emailVerification.sender')"><remote-select :model-value="model.validation_sender || null" type="senders" :placeholder="t('emailVerification.senderHint')" @update:model-value="changeSender" /></n-form-item>
  <n-form-item :label="t('repair.filter_keywords')" class="keywords-field">
   <div class="field-content"><n-input v-model:value="keywordText" type="textarea" :rows="3" :placeholder="t('repair.filter_hint')" :disabled="disabled" @update:value="changeKeywords" /><p class="hint">{{t('repair.filter_hint')}}</p><n-space><n-tag v-for="word in model.filter_keywords" :key="word" size="small">{{word}}</n-tag></n-space></div>
  </n-form-item>
  <n-form-item :label="t('importPreview.file')"><div class="field-content"><input :key="uploadKey" type="file" accept=".xlsx" :disabled="disabled || reading" @change="readFile" /><p class="hint">{{t('contacts.group.fileUpload.hint')}}</p><n-space><n-button text type="primary" :loading="downloading" @click="downloadTemplate">{{t('repair.download_excel')}}</n-button><n-button v-if="model.file_data" text @click="clearFile">{{t('importPreview.clear_file')}}</n-button></n-space></div></n-form-item>
  <n-space align="center"><n-button type="primary" :loading="loading || reading" :disabled="disabled || !model.file_data || reading" @click="startPreview">{{t('importPreview.preview')}}</n-button><span class="hint">{{t(allowEmpty?'importPreview.optional':'importPreview.required')}}</span></n-space>
  <div v-if="preview" class="preview-result">
   <n-space><n-tag>{{t('importPreview.total',{count:preview.total})}}</n-tag><n-tag type="success">{{t('importPreview.valid',{count:preview.valid})}}</n-tag><n-tag v-if="preview.queued" type="info">{{t('importJob.queuedCount',{count:preview.queued})}}</n-tag><n-tag v-if="preview.checking" type="info">{{t('importJob.checkingCount',{count:preview.checking})}}</n-tag><n-tag v-if="preview.unconfirmed" type="warning">{{t('importJob.unconfirmedCount',{count:preview.unconfirmed})}}</n-tag><n-tag type="warning">{{t('importPreview.filtered',{count:preview.filtered})}}</n-tag><n-tag type="error">{{t('importPreview.invalid',{count:preview.invalid})}}</n-tag><n-tag>{{t('importPreview.duplicate',{count:preview.duplicate})}}</n-tag></n-space>
   <p class="hint">{{t('importJob.progress',{done:preview.completed,total:preview.total})}}</p>
   <n-progress type="line" :percentage="preview.total ? Math.round(preview.completed*100/preview.total) : 100" :show-indicator="false" />
   <n-space class="preview-tools"><n-select v-model:value="status" :options="statusOptions" style="width:180px" @update:value="changeStatus" /><n-button v-if="preview.invalid" size="small" :loading="downloadingInvalid" @click="downloadInvalid">{{t('importPreview.download_invalid')}}</n-button><n-button v-if="preview.pending" size="small" @click="downloadPending">{{t('emailVerification.downloadPending')}}</n-button></n-space>
   <n-space v-if="preview.unconfirmed" class="preview-tools"><n-button size="small" :loading="loading" :disabled="disabled" @click="retryUnconfirmed">{{t('importJob.retry')}}</n-button><span class="hint">{{t('importJob.retryHint')}}</span></n-space>
   <n-alert v-if="preview.finished" type="info" class="preview-tools">{{t('importJob.finished')}}</n-alert>
   <div class="preview-tools"><n-checkbox v-model:checked="model.keep_pending" :disabled="disabled || !unconfirmedCount">{{t('emailVerification.keepPending',{count:unconfirmedCount})}}</n-checkbox><p class="hint">{{t('emailVerification.keepPendingHint')}}</p></div>
   <n-data-table :columns="columns" :data="preview.rows || []" :row-key="row=>row.row" :max-height="300" :scroll-x="700" :loading="loading" size="small" />
   <n-pagination v-model:page="page" :page-size="20" :item-count="preview.row_total" class="preview-pager" @update:page="() => loadPreview()" />
   <n-alert v-if="preview.pending || preview.invalid || preview.filtered" type="info" class="preview-tools">{{t('emailVerification.importOnlyValid')}}</n-alert>
   <n-alert v-if="preview.finished && !importableCount" type="warning" class="preview-tools">{{t('importPreview.no_valid')}}</n-alert>
  </div>
 </section>
</template>
<script setup lang="ts">
import {instance} from '@/api'
import {downloadFile} from '@/api/modules/public'
import {readUpload} from '@/utils/response'
import {Message} from '@/utils'
import {emptyContactImport,parseImportKeywords,importableContactCount} from './import-state'
import RemoteSelect from '@/features/workspace/RemoteSelect.vue'
import type {ContactImportDraft} from './import-state'
const model=defineModel<ContactImportDraft>({default:emptyContactImport})
const {allowEmpty=true,disabled=false}=defineProps<{allowEmpty?:boolean;disabled?:boolean}>()
const {t,te}=useI18n()
const keywordText=ref(model.value.filter_keywords.join('\n')),preview=ref<any>(null),page=ref(1),status=ref('all'),loading=ref(false),reading=ref(false),downloading=ref(false),downloadingInvalid=ref(false),uploadKey=ref(0)
const invalidUpload=ref(false)
const jobId=ref('')
const unconfirmedCount=computed(()=>Number(preview.value?.unconfirmed || 0))
const importableCount=computed(()=>importableContactCount(preview.value,model.value.keep_pending))
let serial=0,fileSerial=0,alive=true
let validationPoll:ReturnType<typeof setTimeout>|undefined
function invalidate(){clearTimeout(validationPoll);serial++;jobId.value='';preview.value=null;model.value.preview_token='';model.value.skip_invalid=true;model.value.keep_pending=false;loading.value=false;page.value=1;status.value='all'}
function changeKeywords(text:string){model.value.filter_keywords=parseImportKeywords(text);invalidate()}
function changeSender(value:string|null){model.value.validation_sender=value||'';invalidate();if(model.value.file_data)void startPreview()}
function clearFile(resetInput=true){invalidate();fileSerial++;reading.value=false;invalidUpload.value=false;model.value.file_data='';model.value.file_type='';if(resetInput)uploadKey.value++}
async function readFile(event:Event){
 const file=(event.target as HTMLInputElement).files?.[0];clearFile(false);if(!file)return
 if(!file.name.toLowerCase().endsWith('.xlsx') || file.size>5*1024*1024){invalidUpload.value=true;Message.error(t('importPreview.file_limit'));return}
 const current=++fileSerial;reading.value=true
 try{const content=await readUpload(file,true);if(current!==fileSerial)return;model.value.file_data=content;model.value.file_type='xlsx';await startPreview()}catch{invalidUpload.value=true;Message.error(t('repair.file_read_failed'))}finally{if(current===fileSerial)reading.value=false}
}
async function loadPreview(silent=false,retry=false){
 if(!model.value.file_data)return
 const current=++serial;if(!silent)loading.value=true
 clearTimeout(validationPoll)
 try{
  const body=jobId.value?{job_id:jobId.value,retry_unconfirmed:retry}:{async:true,file_data:model.value.file_data,filter_keywords:model.value.filter_keywords,validation_sender:model.value.validation_sender}
  const result=await instance.post('/contact/group/preview_import',{...body,page:page.value,status:status.value})
  if(current!==serial || !alive)return
  jobId.value=result.job_id;preview.value=result;model.value.preview_token=result.token;model.value.validation_sender=result.validation_sender||model.value.validation_sender
  if(retry && !result.retried)Message.info(t('importJob.retryLater'))
  if(result.active>0)validationPoll=setTimeout(()=>void loadPreview(true),2500)
 }catch{if(current===serial && alive && preview.value?.active>0)validationPoll=setTimeout(()=>void loadPreview(true),10000)}finally{if(current===serial)loading.value=false}
}
async function startPreview(){page.value=1;status.value='all';await loadPreview()}
function changeStatus(){page.value=1;void loadPreview()}
async function retryUnconfirmed(){model.value.keep_pending=false;await loadPreview(false,true)}
async function downloadTemplate(){downloading.value=true;try{await downloadFile({file_path:'./template/example_recipients.xlsx'})}catch{/* Request interceptor shows the error. */}finally{downloading.value=false}}
async function downloadInvalid(){if(!jobId.value)return;downloadingInvalid.value=true;try{await instance.post('/contact/group/preview_import',{job_id:jobId.value,download_invalid:true},{responseType:'blob'})}catch{/* Request interceptor shows the error. */}finally{downloadingInvalid.value=false}}
async function downloadPending(){if(!jobId.value)return;try{await instance.post('/contact/group/preview_import',{job_id:jobId.value,download_pending:true},{responseType:'blob'})}catch{/* Request interceptor shows the error. */}}
const statusLabel=(value:string)=>['queued','checking','unconfirmed'].includes(value)?t('importJob.status.'+value):value==='pending'?t('emailVerification.pending'):t('importPreview.status.'+value)
const statusOptions=computed(()=>['all','valid','queued','checking','unconfirmed','filtered','invalid','duplicate'].map(value=>({value,label:statusLabel(value)})))
const columns=computed(()=>[
 {key:'row',title:t('importPreview.row'),width:70},
 {key:'email',title:t('crm.email'),minWidth:220},
 {key:'name',title:t('crm.name'),minWidth:120},
 {key:'status',title:t('importPreview.row_status'),width:110,render:(row:any)=>statusLabel(row.status)},
 {key:'reason',title:t('importPreview.reason'),minWidth:200,render:(row:any)=>row.reason?t(te('importJob.codes.'+row.reason)?'importJob.codes.'+row.reason:te('emailVerification.codes.'+row.reason)?'emailVerification.codes.'+row.reason:'importPreview.reasons.'+row.reason,{keyword:row.keyword || ''}):'—'},
])
async function validate(){
 if(invalidUpload.value){Message.error(t('importPreview.file_limit'));return false}
 if(reading.value || loading.value){Message.error(t('importPreview.wait'));return false}
 if(!model.value.file_data){if(allowEmpty)return true;Message.error(t('repair.xlsx_required'));return false}
 if(!preview.value || !model.value.preview_token){Message.error(t('repair.import_preview_required'));return false}
 if(!importableCount.value){Message.error(t('importPreview.no_valid'));return false}
 return true
}
watch(()=>model.value,()=>{keywordText.value=model.value.filter_keywords.join('\n');invalidate()})
onBeforeUnmount(()=>{alive=false;serial++;fileSerial++;clearTimeout(validationPoll)})
defineExpose({validate})
</script>
<style scoped>
.contact-import-panel{width:100%;padding:18px;border:1px solid #bdd8cc;border-radius:10px;background:rgba(23,111,101,.035)}h3{font-size:16px;margin:0 0 16px}.field-content{width:100%}.hint{font-size:12px;line-height:1.6;color:var(--color-text-3);margin:6px 0}.keywords-field{margin-bottom:12px}.preview-result{margin-top:18px}.preview-tools{margin:14px 0}.preview-pager{margin-top:12px;justify-content:flex-end}.skip-invalid{margin-top:14px}
</style>
