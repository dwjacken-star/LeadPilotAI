<template>
	<n-modal :show="active && show" @update:show="show = $event" preset="card"
		:title="t('workbench.mail_permissions')" style="width: min(620px, 94vw)">
		<n-alert type="info">{{ t('workbench.binding_hint') }}</n-alert>
		<n-form label-placement="top" style="margin-top: 20px">
			<n-form-item :label="t('workbench.mailbox')"><n-input :value="mailbox" readonly /></n-form-item>
			<n-form-item :label="t('workbench.bind_account')">
				<n-select v-model:value="accountId" filterable remote clearable :options="options" :loading="loading"
					:placeholder="t('workbench.select_account')" @search="search" @scroll="more"
					@update:show="value => value && !loading && load(true)" />
			</n-form-item>
			<n-form-item v-if="accountId && selectedAccount" :label="t('workbench.binding_department')">
				<n-input :value="departmentName" readonly />
			</n-form-item>
			<n-alert v-if="accountId && !loading && bindingState === 'invalid'" type="warning">
				{{ t('workbench.binding_invalid_account') }}
			</n-alert>
			<template v-if="needsGrant && !loading">
				<n-alert type="warning" class="binding-notice">
					{{ t('workbench.binding_grant_required', { department: departmentName }) }}
				</n-alert>
				<n-checkbox v-model:checked="grantDepartment" class="binding-notice">
					{{ t('workbench.binding_grant_checkbox', { department: departmentName }) }}
				</n-checkbox>
			</template>
			<n-alert v-else-if="accountId && bindingState === 'ready' && !loading" type="success">
				{{ t('workbench.binding_granted') }}
			</n-alert>
		</n-form>
		<n-space justify="end" class="binding-notice">
			<n-button :loading="loading" @click="load(true)">{{ t('workbench.binding_refresh') }}</n-button>
			<n-button type="primary" :loading="saving" :disabled="!canSave" @click="save">
				{{ accountId ? t('workbench.save') : t('workbench.unbind') }}
			</n-button>
		</n-space>
	</n-modal>
</template>
<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { workspace } from '@/features/workspace/api'
import { useWorkbenchActivity } from '@/features/workbench/context'
import { bindingFlag, mailboxBindingState, mailboxBindingPayload } from '@/features/workspace/mailbox-binding'
const { t } = useI18n()
const active = useWorkbenchActivity(), show = ref(false), mailbox = ref(''), accountId = ref<number | null>(null)
const rows = ref<any[]>([]), loading = ref(false), saving = ref(false), grantDepartment = ref(false)
let page = 0, total = 0, keyword = '', serial = 0, alive = true
let timer: ReturnType<typeof setTimeout> | undefined
const selectedAccount = computed(() => rows.value.find(r => Number(r.account_id) === accountId.value))
const bindingState = computed(() => accountId.value && !selectedAccount.value ? 'invalid' : mailboxBindingState(selectedAccount.value))
const needsGrant = computed(() => bindingState.value === 'needs_grant')
const departmentName = computed(() => selectedAccount.value?.dept_name || `#${selectedAccount.value?.dept_id || ''}`)
const canSave = computed(() => !!mailbox.value && !loading.value && (!accountId.value || bindingState.value === 'ready' || (needsGrant.value && grantDepartment.value)))
const options = computed(() => rows.value.map(r => ({ value: Number(r.account_id), label: `${r.username || '#' + r.account_id}${r.dept_name ? ' · ' + r.dept_name : ''}`, disabled: !bindingFlag(r.eligible) })))
watch(() => [accountId.value, selectedAccount.value?.dept_id], () => { grantDepartment.value = false })
async function load(reset = false, initial = false) {
	if (loading.value && !reset) return
	const current = ++serial, requestedPage = reset ? 1 : page + 1
	loading.value = true
	try {
		const r = await workspace('webmail_bindings', { page: requestedPage, keyword, data: { mailbox: mailbox.value, selected_account_id: accountId.value || 0 } })
		if (!alive || current !== serial) return
		total = Number(r.total) || 0
		page = requestedPage
		const candidates = [
			...(reset ? rows.value.filter(row => Number(row.account_id) === accountId.value) : rows.value),
			...(r.accounts || []), ...(r.binding?.account_id ? [r.binding] : []), ...(r.selected_account?.account_id ? [r.selected_account] : []),
		]
		rows.value = [...new Map(candidates.map(row => [Number(row.account_id), row])).values()]
		if (initial) accountId.value = r.binding?.account_id ? Number(r.binding.account_id) : null
	} catch { /* API interceptor displays failures; keep the editor open. */ }
	finally { if (alive && current === serial) loading.value = false }
}
function search(value: string) {
	clearTimeout(timer); keyword = value; serial++
	timer = setTimeout(() => void load(true), 250)
}
function more(event: Event) {
	const target = event.target as HTMLElement
	if (target.scrollHeight - target.scrollTop - target.clientHeight < 24 && page * 20 < total && !loading.value) void load()
}
async function open(value: string) {
	serial++; clearTimeout(timer)
	mailbox.value = value; keyword = ''; page = 0; rows.value = []; accountId.value = null; grantDepartment.value = false; show.value = true
	await load(true, true)
}
async function save() {
	if (!canSave.value || saving.value) return
	saving.value = true
	try {
		const account = accountId.value ? selectedAccount.value : null
		await workspace('save_webmail_binding', { data: mailboxBindingPayload(mailbox.value, account, grantDepartment.value) })
		show.value = false
	} catch { await load(true) }
	finally { saving.value = false }
}
onBeforeUnmount(() => { alive = false; serial++; clearTimeout(timer) })
defineExpose({ open })
</script>
<style scoped>
.binding-notice { margin-top: 16px; }
</style>
