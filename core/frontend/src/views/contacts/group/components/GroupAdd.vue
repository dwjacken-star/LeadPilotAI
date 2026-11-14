<template>
 <modal :title="t('contacts.group.add.title')" width="960" :confirm-text="t('importPreview.confirm_group')">
  <div class="group-import-form"><bt-form ref="formRef" :model="form" :rules="rules">
   <n-form-item :label="t('contacts.group.form.name')" path="name"><n-input v-model:value="form.name" /></n-form-item>
   <n-form-item :label="t('repair.list_type')"><n-select v-model:value="form.double_optin" :options="statusOptions" /></n-form-item>
   <contact-import-panel :key="panelKey" ref="importPanel" v-model="importDraft" />
  </bt-form></div>
 </modal>
</template>
<script setup lang="ts">
import {useModal} from '@/hooks/modal/useModal'
import {createGroup} from '@/api/modules/contacts/group'
import ContactImportPanel from '@/features/contacts/ContactImportPanel.vue'
import {emptyContactImport} from '@/features/contacts/import-state'
const {t}=useI18n(),formRef=useTemplateRef('formRef'),importPanel=useTemplateRef('importPanel')
const form=reactive({create_type:2,name:'',description:'',double_optin:0}),importDraft=ref(emptyContactImport()),panelKey=ref(0)
const rules=computed(()=>({name:{required:true,message:t('contacts.group.form.nameRequired'),trigger:'blur'}}))
const statusOptions=computed(()=>[{label:t('repair.single_optin'),value:0},{label:t('repair.double_optin'),value:1}])
const [Modal,modalApi]=useModal({onChangeState:open=>{if(open){Object.assign(form,{create_type:2,name:'',description:'',double_optin:0});importDraft.value=emptyContactImport();panelKey.value++}},onConfirm:async()=>{
 await formRef.value?.validate();if(!await importPanel.value?.validate())return false
 await createGroup({...toRaw(form),...importDraft.value});modalApi.getState<{refresh:()=>void}>()?.refresh()
}})
</script>
<style scoped>.group-import-form{max-height:72vh;overflow:auto;padding:12px 6px}</style>
