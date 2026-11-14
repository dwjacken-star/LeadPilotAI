<template>
	<div class="crm-page">
		<header>
			<div>
				<h1>{{ t('crm.title') }}</h1>
				<p class="text-desc">{{ t('crm.intro') }}</p>
			</div>
			<n-button v-if="can('crm:manage')" type="primary" @click="editCustomer()">{{
				t('crm.add')
			}}</n-button>
		</header>
		<n-alert type="info" class="mb-4">{{ t('crm.sync_hint') }}</n-alert>
		<n-space align="center" class="mb-4">
			<n-select
				v-model:value="mailbox"
				:options="mailboxes.map(value => ({ label: value, value }))"
				class="mailbox-select"
				:placeholder="t('crm.mailbox')" />
			<n-button v-if="can('crm:sync')" :loading="syncing" :disabled="!mailbox" @click="syncInbox">{{
				t('crm.sync')
			}}</n-button>
			<n-input
				v-model:value="keyword"
				clearable
				:placeholder="t('crm.search')"
				class="search"
				@keyup.enter="
					() => {
						page = 1
						refresh()
					}
				" />
			<n-select
				v-model:value="stageFilter"
				clearable
				:options="stageOptions"
				:placeholder="t('crm.stage')"
				style="width: 160px"
				@update:value="
					() => {
						page = 1
						refresh()
					}
				" />
			<n-button :loading="loading" @click="refresh">{{ t('common.actions.refresh') }}</n-button>
		</n-space>
		<n-data-table
			:columns="columns"
			:data="rows"
			:loading="loading"
			:row-key="row => row.id"
			:scroll-x="1050" />
		<n-pagination
			v-model:page="page"
			:item-count="total"
			:page-size="20"
			class="mt-4"
			@update:page="refresh" />
		<n-drawer
			:show="workbenchActive && drawer"
			@update:show="drawer = $event"
			width="min(1100px, 96vw)"
			placement="right">
			<n-drawer-content
				:title="detail?.customer?.name || detail?.customer?.email || t('crm.customer')"
				closable>
				<template v-if="detail">
					<n-space justify="space-between" class="mb-4"
						><span
							>{{ detail.customer.email }} · {{ detail.customer.company }} ·
							{{ t('crm.stages.' + detail.customer.stage) }}</span
						><n-space
							><n-button v-if="can('crm:manage')" @click="editCustomer(detail.customer)">{{
								t('common.actions.edit')
							}}</n-button
							><n-button
								v-if="can('crm:delete')"
								type="error"
								:disabled="replying"
								@click="deleteCustomer(detail.customer)"
								>{{ t('common.actions.delete') }}</n-button
							></n-space
						></n-space
					>
					<n-tabs type="line">
						<n-tab-pane name="messages" :tab="t('crm.messages')">
							<n-alert v-if="!detail.messages.length" type="info">{{
								t('crm.no_messages')
							}}</n-alert>
							<div class="mail-layout">
								<div class="message-list">
									<button
										v-for="item in detail.messages"
										:key="item.id"
										type="button"
										:class="{ selected: message?.id === item.id }"
										@click="openMessage(item.id)">
										<strong>{{ item.subject || '—' }}</strong
										><span>{{ item.mailbox }} · {{ t('crm.directions.' + item.direction) }}</span
										><span
											>{{ date(item.created_at) }} · {{ t('crm.statuses.' + item.status) }}</span
										>
									</button>
								</div>
								<section v-if="message" class="mail-content">
									<h3>{{ message.subject }}</h3>
									<p>{{ message.from_address }} → {{ message.to_address }}</p>
									<p v-if="message.error" class="text-red">{{ message.error }}</p>
									<n-spin :show="messageLoading"
										><bt-preview
											:value="message.body_html || plainHTML(message.body_text)"
											height="420"
									/></n-spin>
									<template v-if="message.direction === 'incoming' && can('crm:reply')">
										<n-divider>{{ t('crm.reply') }}</n-divider>
										<p>
											{{
												t('crm.reply_from', {
													mailbox: message.mailbox,
													recipient: message.from_address,
												})
											}}
										</p>
										<AIModelSelect v-if="can('crm:ai_draft')" v-model:value="aiModel" class="mb-3" /><n-space v-if="can('crm:ai_draft')" class="mb-3"
											><n-select
												v-model:value="aiCompanyID"
												:options="aiCompanies.map(c => ({ value: c.id, label: c.name }))"
												:placeholder="t('growth.choose_company')"
												style="width: 280px" /><n-button
												:loading="aiDrafting"
												:disabled="!aiCompanyID"
												@click="generateAIReply"
												>{{ t('growth.ai_reply') }}</n-button
											></n-space
										>
										<n-alert v-if="aiGenerated" type="info" class="mb-3"
											>{{ t('growth.draft_hint') }}
											<p v-if="missingFacts.length">
												{{ t('growth.missing_facts') }}: {{ missingFacts.join('；') }}
											</p></n-alert
										>
										<n-input
											v-model:value="replyText"
											type="textarea"
											:rows="7"
											:placeholder="t('crm.reply_hint')" />
										<n-popconfirm @positive-click="sendReply"
											><template #trigger
												><n-button
													type="primary"
													class="mt-3"
													:loading="replying"
													:disabled="!replyText.trim()"
													>{{ t('crm.send_reply') }}</n-button
												></template
											>{{ t('crm.send_confirm', { email: message.from_address }) }}</n-popconfirm
										>
									</template>
								</section>
							</div>
						</n-tab-pane>
						<n-tab-pane name="followups" :tab="t('crm.followups')">
							<p>{{ detail.customer.note || '—' }}</p>
							<p>{{ t('crm.next_followup') }}: {{ date(detail.customer.next_followup_at) }}</p>
							<n-space v-if="can('crm:manage')" vertical class="mb-4"
								><n-input
									v-model:value="followup"
									type="textarea"
									:rows="4"
									:placeholder="t('crm.followup_hint')" /><n-button
									:loading="savingNote"
									:disabled="!followup.trim()"
									@click="saveFollowup"
									>{{ t('crm.add_followup') }}</n-button
								></n-space
							>
							<n-timeline
								><n-timeline-item
									v-for="note in detail.followups"
									:key="note.id"
									:title="date(note.created_at)"
									:content="note.content"
							/></n-timeline>
						</n-tab-pane>
					</n-tabs>
				</template>
			</n-drawer-content>
		</n-drawer>
		<n-modal
			:show="workbenchActive && editing"
			@update:show="editing = $event"
			preset="card"
			:title="t('crm.customer')"
			style="width: min(620px, 96vw)">
			<n-form label-placement="top">
				<n-form-item :label="t('crm.email')"><n-input v-model:value="draft.email" /></n-form-item>
				<n-form-item :label="t('crm.name')"><n-input v-model:value="draft.name" /></n-form-item>
				<n-grid :cols="2" :x-gap="16"
					><n-form-item-gi :label="t('crm.company')"
						><n-input v-model:value="draft.company" /></n-form-item-gi
					><n-form-item-gi :label="t('crm.country')"
						><n-input v-model:value="draft.country" /></n-form-item-gi
				></n-grid>
				<n-form-item :label="t('crm.stage')"
					><n-select v-model:value="draft.stage" :options="stageOptions"
				/></n-form-item>
				<n-form-item :label="t('crm.next_followup')"
					><n-date-picker v-model:value="nextFollowup" type="datetime" clearable
				/></n-form-item>
				<n-form-item :label="t('crm.notes')"
					><n-input v-model:value="draft.note" type="textarea"
				/></n-form-item>
			</n-form>
			<n-button type="primary" :loading="saving" @click="saveCustomer">{{
				t('common.actions.save')
			}}</n-button>
		</n-modal>
	</div>
</template>
<script setup lang="tsx">
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { NButton } from 'naive-ui'
import { instance } from '@/api'
import { can } from '@/features/workspace/api'
import { Message, confirm } from '@/utils'
import { growth, list } from '@/features/growth/api'
const { t, locale } = useI18n()
const route = useRoute(),
	aiCompanies = ref<any[]>([]),
	aiCompanyID = ref<number | null>(null),
	aiModel=ref(''),
 aiDrafting = ref(false),
	aiGenerated = ref(false),
	missingFacts = ref<string[]>([])
async function generateAIReply() {
	if (!message.value || !aiCompanyID.value) return
	const id = message.value.id
	aiDrafting.value = true
	try {
		const result = await growth('reply_draft', { id, company_id: aiCompanyID.value,data:{model:aiModel.value} })
		if (message.value?.id !== id) return
		const doc = new DOMParser().parseFromString(result.html, 'text/html')
		replyText.value = Array.from(doc.querySelectorAll('p'))
			.map(p => p.textContent || '')
			.join('\n\n')
		missingFacts.value = list(result.missing_facts)
		aiGenerated.value = true
	} catch {
		/* The shared API displays errors. */
	} finally {
		aiDrafting.value = false
	}
}
const api = (action: string, extra: Record<string, any> = {}) =>
	instance.post('/crm/action', { action, ...extra }) as Promise<any>
const rows = ref<any[]>([]),
	total = ref(0),
	page = ref(1),
	keyword = ref(''),
	stageFilter = ref<string | null>(null),
	loading = ref(false)
const mailboxes = ref<string[]>([]),
	mailbox = ref<string | null>(null),
	syncing = ref(false)
const drawer = ref(false),
	detail = ref<any>(null),
	message = ref<any>(null),
	messageLoading = ref(false),
	replyText = ref(''),
	replying = ref(false),
	requestKey = ref('')
const editing = ref(false),
	draft = ref<any>({}),
	nextFollowup = ref<number | null>(null),
	saving = ref(false),
	followup = ref(''),
	savingNote = ref(false)
let listSerial = 0,
	messageSerial = 0
const date = (value: any) => (value ? new Date(value).toLocaleString(locale.value) : '—')
const plainHTML = (value = '') =>
	'<pre style="white-space:pre-wrap;font:14px/1.8 sans-serif">' +
	String(value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') +
	'</pre>'
const stageOptions = computed(() =>
	['new', 'inquiry', 'quoted', 'negotiating', 'won', 'lost'].map(value => ({
		value,
		label: t('crm.stages.' + value),
	}))
)
const columns = computed(() => [
	{
		key: 'name',
		title: t('crm.customer'),
		minWidth: 180,
		render: (row: any) =>
			h(
				NButton,
				{ text: true, type: 'primary', onClick: () => openCustomer(row.id) },
				{ default: () => row.name || row.email }
			),
	},
	{ key: 'email', title: t('crm.email'), minWidth: 220 },
	{ key: 'company', title: t('crm.company'), minWidth: 150 },
	{
		key: 'stage',
		title: t('crm.stage'),
		minWidth: 100,
		render: (row: any) => t('crm.stages.' + row.stage),
	},
	{
		key: 'next_followup_at',
		title: t('crm.next_followup'),
		minWidth: 180,
		render: (row: any) => date(row.next_followup_at),
	},
	{
		key: 'last_activity_at',
		title: t('crm.last_activity'),
		minWidth: 180,
		render: (row: any) => date(row.last_activity_at),
	},
	{
		key: 'actions',
		title: t('common.columns.actions'),
		width: 160,
		fixed: 'right',
		render: (row: any) =>
			h('div', { class: 'flex gap-3 justify-end' }, [
				can('crm:manage')
					? h(
							NButton,
							{ text: true, type: 'primary', onClick: () => editCustomer(row) },
							{ default: () => t('common.actions.edit') }
						)
					: null,
				can('crm:delete')
					? h(
							NButton,
							{
								text: true,
								type: 'error',
								disabled: replying.value,
								onClick: () => deleteCustomer(row),
							},
							{ default: () => t('common.actions.delete') }
						)
					: null,
			]),
	},
])
async function refresh() {
	const serial = ++listSerial
	loading.value = true
	try {
		const result = await api('list', {
			page: page.value,
			keyword: keyword.value,
			stage: stageFilter.value || '',
		})
		if (serial === listSerial) {
			rows.value = Array.isArray(result?.list) ? result.list : []
			total.value = Number(result?.total) || 0
		}
	} catch {
		/* API displays the error. */
	} finally {
		if (serial === listSerial) loading.value = false
	}
}
async function openCustomer(id: number) {
	try {
		detail.value = await api('detail', { id })
		detail.value.messages = Array.isArray(detail.value.messages) ? detail.value.messages : []
		detail.value.followups = Array.isArray(detail.value.followups) ? detail.value.followups : []
		drawer.value = true
		message.value = null
		replyText.value = ''
		if (detail.value.messages[0]) await openMessage(detail.value.messages[0].id)
	} catch {
		/* API displays the error. */
	}
}
async function openMessage(id: number) {
	aiGenerated.value = false
	missingFacts.value = []
	const serial = ++messageSerial
	messageLoading.value = true
	replyText.value = ''
	requestKey.value = crypto.randomUUID()
	try {
		const result = await api('message', { id })
		if (serial === messageSerial) message.value = result
	} catch {
		/* API displays the error. */
	} finally {
		if (serial === messageSerial) messageLoading.value = false
	}
}
async function syncInbox() {
	if (!mailbox.value) return
	syncing.value = true
	try {
		const count = await api('sync', { data: { mailbox: mailbox.value } })
		Message.success(t('crm.synced', { count }))
		await refresh()
	} catch {
		/* API displays the error. */
	} finally {
		syncing.value = false
	}
}
function editCustomer(row?: any) {
	draft.value = {
		id: 0,
		email: '',
		name: '',
		company: '',
		country: '',
		stage: 'inquiry',
		note: '',
		...row,
	}
	nextFollowup.value = row?.next_followup_at ? new Date(row.next_followup_at).getTime() : null
	editing.value = true
}
async function saveCustomer() {
	saving.value = true
	try {
		await api('save', {
			id: draft.value.id,
			data: {
				...draft.value,
				next_followup_at: nextFollowup.value ? new Date(nextFollowup.value).toISOString() : '',
			},
		})
		editing.value = false
		await refresh()
		if (detail.value?.customer?.id === draft.value.id) await openCustomer(draft.value.id)
	} catch {
		/* API displays the error. */
	} finally {
		saving.value = false
	}
}
async function saveFollowup() {
	savingNote.value = true
	try {
		const id = detail.value.customer.id
		await api('followup', { id, data: { content: followup.value } })
		followup.value = ''
		await openCustomer(id)
	} catch {
		/* API displays the error. */
	} finally {
		savingNote.value = false
	}
}
function deleteCustomer(row: any) {
	confirm({
		title: t('crm.delete_title'),
		content: t('crm.delete_confirm', { email: row.email }),
		confirmType: 'error',
		onConfirm: async () => {
			await api('delete', { id: row.id })
			if (detail.value?.customer?.id === row.id) {
				messageSerial++
				drawer.value = false
				detail.value = null
				message.value = null
			}
			if (rows.value.length === 1 && page.value > 1) page.value--
			Message.success(t('crm.deleted'))
			await refresh()
		},
	})
}
async function sendReply() {
	if (replying.value || !message.value || !replyText.value.trim()) return false
	replying.value = true
	const customer = detail.value.customer.id,
		parent = message.value.id,
		draftText = replyText.value
	let failed = false
	try {
		await api('reply', {
			id: parent,
			data: { html: plainHTML(draftText), request_key: requestKey.value },
		})
		replyText.value = ''
		Message.success(t('crm.reply_recorded'))
	} catch {
		failed = true
		Message.warning(t('crm.unknown_warning'))
	} finally {
		replying.value = false
		requestKey.value = crypto.randomUUID()
		await openCustomer(customer)
		if (failed) {
			await openMessage(parent)
			replyText.value = draftText
		}
		await refresh()
	}
}
onMounted(async () => {
	await refresh()
	if (can('crm:ai_draft')) {
		try {
			aiCompanies.value = list(await growth('companies'))
			aiCompanyID.value = aiCompanies.value[0]?.id || null
		} catch {
			/* The shared API displays errors. */
		}
	}
	const customer = Number(route.query.customer),
		requestedMessage = Number(route.query.message)
	if (customer > 0) {
		await openCustomer(customer)
		if (requestedMessage > 0 && detail.value?.messages.some((m: any) => m.id === requestedMessage))
			await openMessage(requestedMessage)
	}
	try {
		const result = await api('mailboxes')
		mailboxes.value = Array.isArray(result) ? result : []
		mailbox.value = mailboxes.value[0] || null
	} catch {
		/* API displays the error. */
	}
})
onBeforeUnmount(() => {
	listSerial++
	messageSerial++
})
</script>
<style scoped>
.crm-page {
	padding: 24px;
	min-width: 0;
}
.crm-page header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 20px;
}
.crm-page h1 {
	font-size: 24px;
	margin: 0 0 8px;
}
.mailbox-select {
	width: 280px;
}
.search {
	width: 240px;
}
.mail-layout {
	display: grid;
	grid-template-columns: 270px minmax(0, 1fr);
	gap: 20px;
}
.message-list {
	max-height: 75vh;
	overflow: auto;
}
.message-list button {
	display: flex;
	flex-direction: column;
	gap: 8px;
	text-align: left;
	width: 100%;
	padding: 14px;
	border: 1px solid var(--color-border-1);
	background: transparent;
	color: inherit;
	cursor: pointer;
	margin-bottom: 8px;
	border-radius: 6px;
}
.message-list button.selected {
	border-color: #176f65;
	background: #176f650c;
}
.message-list span {
	font-size: 11px;
	word-break: break-all;
}
.mail-content {
	min-width: 0;
}
.mail-content > p {
	word-break: break-word;
}
.text-red {
	color: #d03050;
}
@media (max-width: 800px) {
	.mail-layout {
		grid-template-columns: 1fr;
	}
	.message-list {
		max-height: 240px;
	}
}
</style>
