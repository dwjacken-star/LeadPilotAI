<template>
 <section class="my-6 rounded-lg border border-solid border-color-border p-6">
  <h3>{{t('mailDelivery.identity_title')}}</h3><p class="text-desc mb-4">{{t('mailDelivery.identity_hint')}}</p>
  <n-form label-placement="top"><n-form-item label="From"><n-input v-model:value="identity.from" placeholder="sales@example.com" /></n-form-item><n-form-item label="Reply-To"><n-input v-model:value="identity.reply_to" placeholder="sales@example.com" /></n-form-item></n-form>
  <n-button type="primary" :loading="saving" @click="save">{{t('common.actions.save')}}</n-button>
 </section>
</template>
<script setup lang="ts">
import {growth} from '@/features/growth/api'
import {setSystemConfigKey} from '@/api/modules/settings/common'
import {Message} from '@/utils'
const {t}=useI18n(),identity=ref({from:'',reply_to:''}),saving=ref(false)
onMounted(async()=>{try{identity.value=await growth('delivery_identity')}catch{/* Shared request handler displays the error. */}})
async function save(){saving.value=true;try{await setSystemConfigKey({key:'outbound_identity',value:JSON.stringify(identity.value)});Message.success(t('mailDelivery.saved'))}catch{/* Shared request handler displays the error. */}finally{saving.value=false}}
</script>
