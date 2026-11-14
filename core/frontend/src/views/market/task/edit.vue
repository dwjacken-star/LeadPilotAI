<template>
	<div class="task-edit-container">
		<div class="composer-scroll">
			<n-breadcrumb class="mb-20px"
				><n-breadcrumb-item
					><router-link to="/market/task">{{
						t('market.task.title')
					}}</router-link></n-breadcrumb-item
				><n-breadcrumb-item>{{
					isEdit ? t('market.task.edit.editTitle') : t('market.task.edit.addTitle')
				}}</n-breadcrumb-item></n-breadcrumb
			>
			<div class="composer-heading">
				<div>
					<h1>{{ t('campaignComposer.title') }}</h1>
					<p>{{ t('campaignComposer.subtitle') }}</p>
				</div>
				<n-tag :bordered="false" type="success">{{ $t('ui.pf200130c8604') }}</n-tag>
			</div>
			<bt-form
				ref="formRef"
				:model="form"
				:rules="rules"
				class="task-edit-box"
				label-placement="top">
				<div class="composer-settings">
					<n-card :title="t('campaignComposer.content_source')">
						<n-radio-group
							:value="contentMode"
							:disabled="!!generating"
							@update:value="changeContentMode"
							><n-radio-button value="template">{{
								t('campaignComposer.use_template')
							}}</n-radio-button
							><n-radio-button value="ai">{{
								t('campaignComposer.use_ai')
							}}</n-radio-button></n-radio-group
						>
						<n-form-item
							v-show="contentMode === 'template'"
							:label="t('market.task.edit.template')"
							path="template_id"
							class="mt-16px"
							><div class="template-picker">
								<template-select
									v-if="ready"
									v-model:value="form.template_id"
									:auto-select="!route.query.task_id"
									@update:content="selectTemplateContent"
									@list-ready="templatesReady" /><n-button
									v-if="can('template:create')"
									text
									type="primary"
									@click="handleGoTemplate"
									>{{ t('common.actions.create') }}</n-button
								>
							</div></n-form-item
						>
						<p class="composer-hint">{{ t('campaignComposer.snapshot_hint') }}</p>
						<n-divider />
						<div class="assistant-title">{{ t('campaignComposer.assistant') }}</div>
						<div class="ai-market">
							<n-form-item :label="t('growth.country_code')"
								><n-select
									:value="ai.country"
									filterable
									:options="choices.countries"
									:disabled="!!generating"
									@update:value="changeCountry" /></n-form-item
							><n-form-item :label="t('growth.language')"
								><n-select
									v-model:value="ai.language"
									filterable
									:options="choices.languages"
									:disabled="!!generating"
							/></n-form-item>
						</div>
						<n-form-item :label="t('growth.model')"
							><div class="model-picker">
								<AIModelSelect
									v-model:value="ai.model"
									filterable
									:options="availableModels"
									:loading="modelsLoading"
									:disabled="!!generating" /><n-button
									:loading="modelsLoading"
									:disabled="!!generating"
									@click="loadAuthorModels"
									>{{ t('campaignComposer.refresh') }}</n-button
								>
							</div></n-form-item
						>
						<n-alert v-if="!aiEnabled && !modelsLoading" type="warning" class="mb-16px">{{
							t('campaignComposer.config_required')
						}}</n-alert>
						<p v-if="modelWarning" class="composer-hint">{{ modelWarning }}</p>
						<n-form-item :label="t('campaignComposer.requirement')"
							><n-input
								class="requirement-input"
								v-model:value="ai.requirement"
								type="textarea"
								:autosize="{ minRows: 5, maxRows: 12 }"
								maxlength="8000"
								show-count
								:placeholder="t('campaignComposer.requirement_placeholder')"
								:disabled="!!generating"
						/></n-form-item>
						<div class="generate-actions">
							<n-button
								type="primary"
								:loading="generating === 'all'"
								:disabled="!aiEnabled || !!generating"
								@click="generate('all')"
								>{{ t('campaignComposer.generate_all') }}</n-button
							><n-button
								:loading="generating === 'body'"
								:disabled="!aiEnabled || !!generating"
								@click="generate('body')"
								>{{ t('campaignComposer.generate_body') }}</n-button
							>
						</div>
						<p class="composer-hint">{{ t('campaignComposer.ai_hint') }}</p>
					</n-card>
					<n-card :title="t('campaignComposer.audience')">
						<n-form-item :label="t('market.task.edit.from')"
							><remote-select v-model="form.senders" type="senders" multiple :disabled="isEdit"
						/></n-form-item>
						<p class="composer-hint">{{ t('repair.rotation_hint') }}</p>
						<n-form-item :label="t('repair.cc_email')"
							><n-input v-model:value="form.cc_email" :placeholder="t('repair.cc_hint')"
						/></n-form-item>
						<n-form-item :label="t('market.task.edit.recipients')" path="group_ids"
							><div class="full-width">
								<remote-select
									v-model="form.group_ids"
									type="groups"
									multiple
									:disabled="isEdit" /><i18n-t
									tag="p"
									class="composer-hint"
									keypath="market.task.edit.recipientsCount"
									scope="global"
									><template #count
										><b>{{ recipientsCount }}</b></template
									></i18n-t
								>
							</div></n-form-item
						>
						<n-form-item :label="t('market.task.edit.tagLogic')"
							><n-select
								v-model:value="form.tag_logic"
								:options="logicOptions"
								:disabled="isEdit"
								@update:value="getRecipientCount" /></n-form-item
						><n-form-item :label="t('market.task.edit.tags')"
							><remote-select
								v-model="form.tag_ids"
								type="tags"
								multiple
								:group-ids="form.group_ids"
								:disabled="isEdit"
								@update:model-value="getRecipientCount"
						/></n-form-item>
					</n-card>
					<n-card :title="t('campaignComposer.delivery')">
						<div class="delivery-grid">
							<n-form-item :label="t('market.task.edit.ipWarmup')"
								><n-switch
									v-model:value="form.warmup"
									:checked-value="1"
									:unchecked-value="0" /></n-form-item
							><n-form-item :label="t('market.task.edit.unsubscribeLink')"
								><n-switch
									v-model:value="form.unsubscribe"
									:checked-value="1"
									:unchecked-value="0" /></n-form-item
							><n-form-item :label="t('market.task.edit.trackClick')"
								><n-switch
									v-model:value="form.track_click"
									:checked-value="1"
									:unchecked-value="0" /></n-form-item
							><n-form-item :label="t('market.task.edit.trackOpen')"
								><n-switch v-model:value="form.track_open" :checked-value="1" :unchecked-value="0"
							/></n-form-item>
						</div>
						<p class="composer-hint">{{ t('market.task.edit.ipWarmupTip') }}</p>
						<n-form-item :label="t('market.task.edit.threads')"
							><n-space
								><n-radio-group v-model:value="threadsType" @update:value="handleUpdateThread"
									><n-radio :value="0">{{ t('market.task.edit.threadsAuto') }}</n-radio
									><n-radio :value="1">{{
										t('market.task.edit.threadsCustom')
									}}</n-radio></n-radio-group
								><n-input-number
									v-if="threadsType === 1"
									v-model:value="form.threads"
									:min="1"
									:max="100"
									style="width: 110px" /></n-space
						></n-form-item>
						<n-form-item :label="t('market.task.edit.sendTime')" path="start_time"
							><n-space vertical
								><n-radio-group v-model:value="sendTimeType" @update:value="handleUpdateSend"
									><n-radio :value="0">{{ t('market.task.edit.sendTimeNow') }}</n-radio
									><n-radio :value="1">{{
										t('campaignComposer.scheduled')
									}}</n-radio></n-radio-group
								><n-date-picker
									v-if="sendTimeType === 1"
									v-model:value="form.start_time"
									type="datetime" /></n-space
						></n-form-item>
						<n-form-item :label="t('market.task.edit.remark')"
							><n-input v-model:value="form.remark"
						/></n-form-item>
					</n-card>
				</div>
				<div class="task-preview">
					<n-card :title="t('campaignComposer.mail_workspace')">
						<n-form-item :label="t('market.task.edit.displayName')" path="full_name"
							><div class="header-editor">
								<n-input
									v-model:value="form.full_name"
									:placeholder="t('market.task.edit.displayNamePlaceholder')"
									maxlength="120" /><n-button
									:loading="generating === 'full_name'"
									:disabled="!aiEnabled || !!generating"
									@click="generate('full_name')"
									>{{ t('campaignComposer.generate_name') }}</n-button
								>
							</div></n-form-item
						>
						<n-form-item :label="t('market.task.edit.subject')" path="subject"
							><div class="header-editor">
								<n-input
									v-model:value="form.subject"
									:placeholder="t('market.task.edit.subjectPlaceholder')"
									maxlength="200" /><n-button
									:loading="generating === 'subject'"
									:disabled="!aiEnabled || !!generating"
									@click="generate('subject')"
									>{{ t('campaignComposer.generate_subject') }}</n-button
								>
							</div></n-form-item
						>
						<div class="mail-envelope">
							<span
								>{{ t('market.task.edit.from') }}: {{ form.full_name }} &lt;{{
									form.senders.join(', ') || '—'
								}}&gt;</span
							><span
								>{{ t('market.task.edit.to') }}: {{ recipientsCount }}
								{{ t('campaignComposer.recipients') }}</span
							>
						</div>
						<div class="body-toolbar">
							<n-radio-group v-model:value="bodyTab" size="small"
								><n-radio-button value="preview">{{ t('common.actions.preview') }}</n-radio-button
								><n-radio-button v-if="contentMode === 'ai'" value="rich">{{
									t('campaignComposer.rich_edit')
								}}</n-radio-button
								><n-radio-button value="html">HTML</n-radio-button></n-radio-group
							><n-radio-group
								v-if="bodyTab === 'preview'"
								v-model:value="previewDevice"
								size="small"
								><n-radio-button value="desktop">{{ t('campaignComposer.desktop') }}</n-radio-button
								><n-radio-button value="mobile">{{
									t('campaignComposer.mobile')
								}}</n-radio-button></n-radio-group
							>
						</div>
						<div v-if="bodyTab === 'preview'" class="preview-canvas">
							<div class="preview-paper" :class="{ 'mobile-paper': previewDevice === 'mobile' }">
								<bt-preview
									v-if="templateContent.trim()"
									:value="templateContent"
									height="580" /><n-empty
									v-else
									:description="t('campaignComposer.empty')"
									class="empty-preview" />
							</div>
						</div>
						<task-body-editor v-else-if="bodyTab === 'rich'" v-model:value="templateContent" />
						<n-input
							v-else
							class="html-input"
							v-model:value="templateContent"
							type="textarea"
							:autosize="{ minRows: 22, maxRows: 32 }"
							:placeholder="t('campaignComposer.html_placeholder')" />
						<p class="composer-hint">{{ t('campaignComposer.preview_hint') }}</p>
						<n-form-item v-if="can('campaign:publish')" :label="t('market.task.edit.testEmail')" :show-feedback="false"
							><div class="header-editor">
								<n-input
									v-model:value="testEmail"
									:placeholder="t('market.task.edit.testEmailPlaceholder')" /><n-button
									:disabled="!!generating"
									@click="handleSendTest"
									>{{ t('market.task.edit.sendTest') }}</n-button
								>
							</div></n-form-item
						>
					</n-card>
				</div>
			</bt-form>
		</div>
		<div class="action-buttons">
			<span class="composer-hint">{{ t('campaignComposer.review_hint') }}</span
			><n-space
				><n-button secondary @click="handleGoBack">{{ t('common.actions.back') }}</n-button
				><n-button
					v-if="can(isEdit ? 'campaign:update' : 'campaign:create')"
					type="primary"
					:loading="submitting"
					:disabled="!!generating || generationDialog"
					@click="handleSubmit"
					>{{ t('common.actions.confirm') }}</n-button
				></n-space
			>
		</div>
		<n-modal
			:show="workbenchActive && generationDialog"
			@update:show="generationDialog = $event"
			preset="card"
			:title="t('campaignComposer.generated_review')"
			style="width: 760px; max-width: 94vw"
			><template v-if="generationPending"
				><n-alert type="info" class="mb-16px">{{ t('campaignComposer.replace_hint') }}</n-alert>
				<p v-if="generationPending.target === 'all' || generationPending.target === 'full_name'">
					<b>{{ t('market.task.edit.displayName') }}:</b> {{ generationPending.full_name }}
				</p>
				<p v-if="generationPending.target === 'all' || generationPending.target === 'subject'">
					<b>{{ t('market.task.edit.subject') }}:</b> {{ generationPending.subject }}
				</p>
				<bt-preview
					v-if="generationPending.html_content"
					:value="generationPending.html_content"
					height="390" /><n-space justify="end"
					><n-button @click="generationDialog = false">{{
						t('campaignComposer.keep_current')
					}}</n-button
					><n-button type="primary" @click="applyGeneration(generationPending)">{{
						t('campaignComposer.apply')
					}}</n-button></n-space
				></template
			></n-modal
		>
	</div>
</template>
<script lang="ts" setup>
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchDirty, useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
const { markClean, markDirty } = useWorkbenchDirty()
import RemoteSelect from '@/features/workspace/RemoteSelect.vue'
import { getDefaultSender } from '@/features/workspace/defaultSender'
import { workspace, can } from '@/features/workspace/api'
import { FormRules } from 'naive-ui'
import { useGlobalStore } from '@/store'
import { object } from '@/features/growth/api'
import { growth } from '@/features/growth/api'
import marketData from '@/features/growth/market-data.json'
import { marketChoices, retainChoice } from '@/features/growth/options'
import { normalizeEmailHTML } from '@/utils/email-content'
import { confirm, isObject, Message } from '@/utils'

import { addTask, getTaskDetails, sendTestEmail, updateTask } from '@/api/modules/market/task'
import { Task } from './interface'
import { Template } from '../template/interface'

import TemplateSelect from './components/TemplateSelect.vue'

const { t, locale } = useI18n()
const TaskBodyEditor = defineAsyncComponent(() => import('./components/TaskBodyEditor.vue'))

const route = useRoute()
const router = useRouter()

const globalStore = useGlobalStore()

const formRef = useTemplateRef('formRef')

const isEdit = ref(false)

const taskId = ref(0)

// 表单数据
const form = reactive({
	addresser: null as null | string,
	senders: [] as string[],
	group_ids: [] as number[],
	cc_email: '',
	full_name: '',
	subject: '',
	group_id: null as null | number,
	template_id: null as null | number,
	is_record: 1,
	unsubscribe: 1,
	warmup: 0,
	threads: 0,
	start_time: null as number | null,
	remark: '',
	tag_ids: [] as number[],
	tag_logic: 'OR',
	track_click: 1,
	track_open: 1,
})

const logicOptions = [
	{
		label: 'OR',
		value: 'OR',
	},
	{
		label: 'AND',
		value: 'AND',
	},
	{
		label: 'NOT',
		value: 'NOT',
	},
]

const rules: FormRules = {
	full_name: {
		trigger: ['blur', 'input'],
		validator: () => {
			if (form.full_name === '') {
				return new Error(t('market.task.edit.validation.displayNameRequired'))
			}
			return true
		},
	},
	subject: {
		trigger: ['blur', 'input'],
		validator: () => {
			if (form.subject === '') {
				return new Error(t('market.task.edit.validation.subjectRequired'))
			}
			return true
		},
	},
	group_ids: {
		trigger: 'change',
		validator: () => {
			if (!form.group_ids.length) {
				return new Error(t('market.task.edit.validation.recipientsRequired'))
			}
			return true
		},
	},
	template_id: {
		trigger: 'change',
		validator: () => {
			if (!templateContent.value.trim()) {
				return new Error(t('market.task.edit.validation.templateRequired'))
			}
			return true
		},
	},
	start_time: {
		validator: () => {
			if (sendTimeType.value === 1 && form.start_time === null) {
				return new Error(t('market.task.edit.validation.sendTimeRequired'))
			}
			return true
		},
	},
}

// 分组名称

// 联系人数量
const recipientsCount = ref(0)

// 线程类型
const threadsType = ref(0)

// 发送时间类型
const sendTimeType = ref(0)

// 测试邮件
const testEmail = ref('')

// 模板内容
const templateContent = ref('')
const ready = ref(false),
	submitting = ref(false),
	contentMode = ref<'template' | 'ai'>('template'),
	bodyTab = ref('preview'),
	previewDevice = ref('desktop')
const ai = reactive({ requirement: '', country: 'US', language: 'en', model: '' })
const modelOptions = ref<{ value: string; label: string }[]>([]),
	modelsLoading = ref(false),
	generating = ref(''),
	aiEnabled = ref(false),
	modelWarning = ref('')
const choices = computed(() => marketChoices(marketData, locale.value))
const availableModels = computed(() => retainChoice(modelOptions.value, ai.model))
const generationPending = ref<any>(null),
	generationDialog = ref(false)
let alive = true,
	generationSerial = 0
function changeCountry(value: string) {
	ai.country = value
	const item = marketData.countries.find(item => item.code === value)
	if (item) ai.language = item.language
}
async function loadAuthorModels() {
	modelsLoading.value = true
	modelWarning.value = ''
	try {
		const result = await growth('campaign_options')
		if (!alive) return
		aiEnabled.value = !!result.enabled
		modelOptions.value = result.models || []
		if (!ai.model) ai.model = result.model || ''
		modelWarning.value = result.warning ? t('campaignComposer.model_warning') : ''
	} catch {
		aiEnabled.value = false
	} finally {
		modelsLoading.value = false
	}
}
function selectTemplateContent(value: string) {
	if (!ready.value || contentMode.value !== 'template') return
	templateContent.value = value
	bodyTab.value = 'preview'
}
function templatesReady(list: Template[]) {
	findTemplateId(list)
	if (!templateContent.value && form.template_id) {
		templateContent.value = list.find(item => item.id === form.template_id)?.html_content || ''
	}
}
function changeContentMode(value: 'template' | 'ai') {
	contentMode.value = value
	bodyTab.value = 'preview'
	if (value === 'ai' && !ai.model) void loadAuthorModels()
}
function applyGeneration(result: any) {
 markDirty()
	if (result.target === 'all' || result.target === 'full_name') form.full_name = result.full_name
	if (result.target === 'all' || result.target === 'subject') form.subject = result.subject
	if (result.target === 'all' || result.target === 'body') {
		templateContent.value = result.html_content
		contentMode.value = 'ai'
		bodyTab.value = 'preview'
	}
	generationDialog.value = false
	generationPending.value = null
	Message.success(t('campaignComposer.applied'))
}
async function generate(target: 'all' | 'body' | 'subject' | 'full_name') {
	if (generating.value) return
	if (!ai.requirement.trim()) {
		Message.warning(t('campaignComposer.requirement_required'))
		return
	}
	if (!ai.country || !ai.language || !ai.model) {
		Message.warning(t('campaignComposer.selection_required'))
		return
	}
	const call = ++generationSerial
	const before = JSON.stringify({
		full_name: form.full_name,
		subject: form.subject,
		content: templateContent.value,
	})
	generating.value = target
	try {
		const result = await growth('campaign_compose', {
			data: {
				...ai,
				target,
				full_name: form.full_name,
				subject: form.subject,
				content: templateContent.value,
			},
		})
		if (!alive || call !== generationSerial) return
		const after = JSON.stringify({
			full_name: form.full_name,
			subject: form.subject,
			content: templateContent.value,
		})
		const overwrites =
			(target === 'all' && (form.full_name || form.subject || templateContent.value)) ||
			(target === 'body' && templateContent.value) ||
			(target === 'subject' && form.subject) ||
			(target === 'full_name' && form.full_name)
		if (before !== after || overwrites) {
			generationPending.value = result
			generationDialog.value = true
		} else applyGeneration(result)
	} catch {
		/* Shared API displays provider and quota errors; existing content is preserved. */
	} finally {
		if (alive) generating.value = ''
	}
}
onBeforeUnmount(() => {
	alive = false
	generationSerial++
})
onMounted(() => {
	void loadAuthorModels()
})

// 跳转模板页面
const handleGoTemplate = () => {
	router.push('/template')
}

// 更新线程类型
const handleUpdateThread = (val: number) => {
	if (val === 0) {
		form.threads = 0
	} else {
		form.threads = 5
	}
}

// 更新发送时间类型
const handleUpdateSend = () => {
	if (sendTimeType.value === 0) {
		form.start_time = null
	}
}

// 发送测试邮件
const handleSendTest = async () => {
	if (!testEmail.value) {
		Message.error(t('market.task.edit.validation.testEmailRequired'))
		return
	}
	if (!form.subject) {
		Message.error(t('market.task.edit.validation.subjectRequired'))
		return
	}
	if (!templateContent.value.trim()) {
		Message.error(t('market.task.edit.validation.templateRequired'))
		return
	}

	await sendTestEmail({
		addresser: form.senders[0] || form.addresser || '',
		senders: form.senders,
		group_ids: form.group_ids,
		cc_email: form.cc_email,
		subject: form.subject,
		full_name: form.full_name,
		recipient: testEmail.value,
		template_id: contentMode.value === 'ai' ? 0 : form.template_id || 0,
		html_content: normalizeEmailHTML(templateContent.value),
		unsubscribe: form.unsubscribe,
		track_open: form.track_open,
		track_click: form.track_click,
		warmup: form.warmup,
	})
}

const handleGoBack = () => {
	if (form.subject || form.group_id) {
		confirm({
			title: t('market.task.edit.discard.title'),
			content: t('market.task.edit.discard.content'),
			onConfirm: () => {
				router.go(-1)
			},
		})
	} else {
		router.go(-1)
	}
}

let audienceSerial = 0
const getRecipientCount = async () => {
	const serial = ++audienceSerial
	if (!form.group_ids.length) {
		recipientsCount.value = 0
		return
	}
	try {
		const result = await workspace('preview_audience', {
			data: { group_ids: form.group_ids, tag_ids: form.tag_ids, tag_logic: form.tag_logic },
		})
		if (serial === audienceSerial) recipientsCount.value = Number(result.total) || 0
	} catch {
		/* API reports errors. */
	}
}
watch(
	() => form.senders,
	values => {
		form.addresser = values[0] || null
	},
	{ deep: true }
)
watch(
	() => form.group_ids,
	values => {
		form.group_id = values[0] || null
		if (!isEdit.value) form.tag_ids = []
		void getRecipientCount()
	},
	{ deep: true }
)

const getParams = () => {
	let startTime = form.start_time
	if (sendTimeType.value === 0 || startTime === null) {
		startTime = Date.now()
	}
	return {
		track_open: form.track_open,
		track_click: form.track_click,
		authoring_config: { mode: contentMode.value, ...ai },
		addresser: form.senders[0] || form.addresser || '',
		senders: form.senders,
		group_ids: form.group_ids,
		cc_email: form.cc_email,
		full_name: form.full_name,
		subject: form.subject,
		group_id: form.group_id || 0,
		template_id: contentMode.value === 'ai' ? 0 : form.template_id || 0,
		html_content: normalizeEmailHTML(templateContent.value),
		is_record: form.is_record,
		unsubscribe: form.unsubscribe,
		warmup: form.warmup,
		threads: form.threads,
		start_time: Math.floor(startTime / 1000),
		remark: form.remark,
		tag_ids: form.tag_ids,
		tag_logic: form.tag_logic,
	}
}

const handleSubmit = async () => {
	if (!can(isEdit.value ? 'campaign:update' : 'campaign:create')) return
	if (submitting.value) return
	await formRef.value?.validate()
	if (!form.senders.length) {
		Message.error(t('campaignComposer.sender_required'))
		return
	}
	submitting.value = true
	try {
		if (!isEdit.value) {
			await addTask(getParams())
		} else {
			await updateTask({
				task_id: taskId.value,
				...getParams(),
			})
		}
		markClean()
		router.push('/market/task')
	} finally {
		submitting.value = false
	}
}

/**
 * @description Find templateId from template list
 */
function findTemplateId(list: Template[]) {
	if (route.query.chat_id) {
		const findRes = list.find(item => item.chat_id == route.query.chat_id)
		if (findRes) {
			form.template_id = findRes.id
			form.subject = globalStore.temp_subject
			templateContent.value = findRes.html_content
		}
	}
}

const initForm = async () => {
	const { task_id: id, type } = route.query
	if (type === 'edit') {
		taskId.value = Number(id)
		isEdit.value = true
	} else {
		taskId.value = 0
		isEdit.value = false
	}

	if (!id) {
		let sender = await getDefaultSender()
		try {
			const identity = await growth('delivery_identity')
			if (identity.from) {
				const allowed = await workspace('list', {
					type: 'senders',
					page: 1,
					page_size: 1,
					data: { ids: [identity.from] },
				})
				if (allowed.list?.some((item: { username: string }) => item.username === identity.from))
					sender = identity.from
			}
		} catch {
			/* Existing scoped default remains available if identity lookup fails. */
		}
		if (sender && !form.senders.length) form.senders = [sender]
		return
	}
	const res = await getTaskDetails({ id: Number(id) })
	if (isObject<Task>(res)) {
		form.addresser = res.addresser
		form.senders = Array.isArray(res.senders) && res.senders.length ? res.senders : [res.addresser]
		form.group_ids =
			Array.isArray(res.group_ids) && res.group_ids.length ? res.group_ids : [res.group_id]
		form.cc_email = res.cc_email || ''
		form.full_name = res.full_name
		form.subject = res.subject
		form.group_id = res.group_id
		// form.group_id = res.etypes.split(',').map(item => Number(item))
		form.template_id = res.template_id
		templateContent.value = res.snapshot_content || ''
		if (!res.template_id) contentMode.value = 'ai'
		const author = object(res.authoring_config)
		if (author.mode === 'ai' || author.mode === 'template') contentMode.value = author.mode
		for (const key of ['country', 'language', 'model', 'requirement'] as const) {
			if (typeof author[key] === 'string') ai[key] = author[key]
		}
		form.is_record = res.is_record
		form.warmup = Number(res.warmup) === 1 ? 1 : 0
		if (Number(res.start_time) * 1000 > Date.now()) {
			form.start_time = Number(res.start_time) * 1000
			sendTimeType.value = 1
		}
		form.unsubscribe = res.unsubscribe
		form.threads = res.threads
		threadsType.value = res.threads === 0 ? 0 : 1
		form.remark = res.remark
		form.tag_logic = res.tag_logic
		form.track_open = res.track_open
		form.track_click = res.track_click
		nextTick(() => {
			form.tag_ids = res.tag_ids
		})
	}
}

initForm().finally(() => {
	ready.value = true
})
</script>

<style scoped lang="scss">
.task-edit-container {
	height: 100%;
	display: flex;
	flex-direction: column;
	overflow: hidden;
}
.composer-scroll {
	flex: 1;
	overflow: auto;
	padding: 24px;
}
.composer-heading {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 24px;
	gap: 16px;
}
h1 {
	font-size: 24px;
	line-height: 1.3;
	margin: 0 0 8px;
	font-weight: 600;
}
.composer-heading p {
	margin: 0;
	color: var(--color-text-3);
	line-height: 1.6;
}
.task-edit-box {
	display: grid;
	grid-template-columns: minmax(330px, 400px) minmax(0, 1fr);
	gap: 24px;
	align-items: start;
}
.composer-settings {
	display: flex;
	flex-direction: column;
	gap: 20px;
	min-width: 0;
}
.task-preview {
	position: sticky;
	top: 0;
	min-width: 0;
}
.composer-hint {
	font-size: 12px;
	color: var(--color-text-3);
	line-height: 1.65;
	margin: 8px 0 16px;
}
.assistant-title {
	font-weight: 600;
	font-size: 16px;
	margin-bottom: 16px;
}
.ai-market,
.delivery-grid {
	display: grid;
	grid-template-columns: repeat(2, minmax(0, 1fr));
	gap: 0 16px;
}
.model-picker,
.header-editor,
.template-picker {
	display: flex;
	align-items: center;
	gap: 10px;
	width: 100%;
}
.model-picker :deep(.n-select),
.header-editor :deep(.n-input),
.template-picker :deep(.n-select) {
	flex: 1;
	min-width: 0;
}
.full-width {
	width: 100%;
}
.generate-actions {
	display: flex;
	gap: 10px;
	flex-wrap: wrap;
}
.mail-envelope {
	display: flex;
	flex-direction: column;
	gap: 6px;
	font-size: 12px;
	color: var(--color-text-3);
	padding: 12px 16px;
	border-radius: 8px;
	background: var(--color-bg-2);
	overflow-wrap: anywhere;
}
.body-toolbar {
	display: flex;
	align-items: center;
	justify-content: space-between;
	flex-wrap: wrap;
	gap: 12px;
	margin: 20px 0 12px;
}
.preview-canvas {
	padding: 16px;
	background: var(--color-bg-2);
	border: 1px solid var(--color-border-1);
	border-radius: 10px;
	overflow: auto;
}
.preview-paper {
	background: #fff;
	color: #243247;
	max-width: 760px;
	min-height: 580px;
	margin: 0 auto;
	border-radius: 6px;
	box-shadow: 0 2px 10px #0000000a;
}
.mobile-paper {
	max-width: 390px;
}
.preview-paper :deep(iframe) {
	display: block;
}
.empty-preview {
	padding-top: 200px;
}
.action-buttons {
	padding: 16px 24px;
	background: var(--color-bg-1);
	border-top: 1px solid var(--color-border-1);
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 16px;
}
.action-buttons .composer-hint {
	margin: 0;
}
@media (max-width: 1150px) {
	.task-edit-box {
		grid-template-columns: 1fr;
	}
	.task-preview {
		position: static;
		grid-row: 1;
	}
	.composer-settings {
		grid-row: 2;
	}
	.preview-paper {
		max-width: 100%;
	}
	.mobile-paper {
		max-width: 390px;
	}
}
@media (max-width: 600px) {
	.composer-scroll {
		padding: 16px;
	}
	.composer-heading {
		align-items: flex-start;
	}
	.header-editor {
		flex-wrap: wrap;
	}
	.header-editor :deep(.n-input) {
		flex-basis: 100%;
	}
	.action-buttons {
		padding: 12px 16px;
	}
	.action-buttons > .composer-hint {
		display: none;
	}
	.ai-market {
		grid-template-columns: 1fr;
	}
	.preview-canvas {
		padding: 8px;
	}
	.task-edit-box {
		gap: 16px;
	}
}
.requirement-input {
	min-height: 170px;
}
.requirement-input :deep(.n-input__textarea-el),
.requirement-input :deep(.n-input__textarea-mirror) {
	min-height: 140px;
	line-height: 1.6;
}
.html-input {
	min-height: 500px;
}
.html-input :deep(.n-input__textarea-el),
.html-input :deep(.n-input__textarea-mirror) {
	min-height: 460px;
	line-height: 1.6;
	font-family: monospace;
}
</style>
