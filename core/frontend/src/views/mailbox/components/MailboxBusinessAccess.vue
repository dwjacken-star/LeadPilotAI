<template>
	<n-modal
		:show="active && show"
		@update:show="show = $event"
		preset="card"
		:title="t('workbench.mail_permissions')"
		style="width: min(620px, 94vw)">
		<n-alert type="info">{{ t('workbench.binding_hint') }}</n-alert>
		<n-form label-placement="top" style="margin-top: 20px">
			<n-form-item :label="t('workbench.mailbox')"
				><n-input :value="mailbox" readonly
			/></n-form-item>
			<n-form-item :label="t('workbench.bind_account')"
				><n-select
					v-model:value="accountId"
					filterable
					remote
					clearable
					:options="options"
					:loading="loading"
					:placeholder="t('workbench.select_account')"
					@search="search"
					@scroll="more"
			/></n-form-item>
		</n-form>
		<n-space justify="end"
			><n-button :loading="saving" @click="save">{{
				accountId ? t('workbench.save') : t('workbench.unbind')
			}}</n-button></n-space
		>
	</n-modal>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { workspace } from '@/features/workspace/api'
import { useWorkbenchActivity } from '@/features/workbench/context'
const { t } = useI18n(),
	active = useWorkbenchActivity(),
	show = ref(false),
	mailbox = ref(''),
	accountId = ref<number | null>(null),
	rows = ref<any[]>([]),
	loading = ref(false),
	saving = ref(false)
let page = 1,
	total = 0,
	keyword = '',
	serial = 0
const options = computed(() =>
	rows.value.map(r => ({ value: Number(r.account_id), label: r.username }))
)
async function load(reset = false, initial = false) {
	if (loading.value && !reset) return
	const current = ++serial
	loading.value = true
	try {
		const r = await workspace('webmail_bindings', {
			page,
			keyword,
			data: { mailbox: mailbox.value },
		})
		if (current !== serial) return
		total = r.total
		rows.value = [
			...new Map(
				[
					...(reset
						? rows.value.filter(r => Number(r.account_id) === accountId.value)
						: rows.value),
					...(r.accounts || []),
					...(r.binding?.account_id ? [r.binding] : []),
				].map(x => [x.account_id, x])
			).values(),
		]
		if (initial) accountId.value = r.binding?.account_id ? Number(r.binding.account_id) : null
	} finally {
		if (current === serial) loading.value = false
	}
}
function search(value: string) {
	keyword = value
	page = 1
	void load(true)
}
function more(event: Event) {
	const target = event.target as HTMLElement
	if (
		target.scrollHeight - target.scrollTop - target.clientHeight < 24 &&
		page * 20 < total &&
		!loading.value
	) {
		page++
		void load()
	}
}
async function open(value: string) {
	mailbox.value = value
	keyword = ''
	page = 1
	rows.value = []
	show.value = true
	await load(true, true)
}
async function save() {
	saving.value = true
	try {
		await workspace('save_webmail_binding', {
			data: { mailbox: mailbox.value, account_id: accountId.value || 0 },
		})
		show.value = false
	} finally {
		saving.value = false
	}
}
defineExpose({ open })
</script>
