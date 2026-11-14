<template>
 <section class="my-6 rounded-lg border border-solid border-color-border p-6">
  <h3>{{t('mailDelivery.optout_title')}}</h3><p class="text-desc mb-4">{{t('mailDelivery.optout_hint')}}</p>
  <n-space class="mb-4"><n-input v-model:value="keyword" clearable :placeholder="t('crm.email')" @keyup.enter="search" /><n-button @click="search">{{t('growth.search')}}</n-button></n-space>
  <n-data-table :columns="columns" :data="rows" :loading="loading" :scroll-x="650" />
  <n-pagination v-model:page="page" :page-size="20" :item-count="total" class="mt-4" @update:page="refresh" />
 </section>
</template>
<script setup lang="ts">
import {growth,list} from '@/features/growth/api'
const {t}=useI18n(),keyword=ref(''),rows=ref<any[]>([]),page=ref(1),total=ref(0),loading=ref(false)
const columns=computed(()=>[{key:'email',title:t('crm.email'),minWidth:200},{key:'domain',title:t('mailDelivery.sender_domain'),minWidth:140},{key:'active',title:t('growth.status'),minWidth:140,render:(r:any)=>t(r.active?'mailDelivery.opted_out':'mailDelivery.resubscribed')},{key:'updated_at',title:t('growth.created_at'),minWidth:180}])
async function refresh(){loading.value=true;try{const result=await growth('delivery_suppressions',{keyword:keyword.value,page:page.value});rows.value=list(result.list);total.value=Number(result.total)||0}catch{/* Shared request handler displays the error. */}finally{loading.value=false}}
function search(){page.value=1;return refresh()}
onMounted(refresh)
</script>
