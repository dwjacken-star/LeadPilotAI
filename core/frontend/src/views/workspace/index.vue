<template>
	<div class="workspace">
		<header>
			<div>
				<h1>{{ sectionTitle }}</h1>
				<p>
					{{ principal?.username }} ·
					{{ principal?.dept_name || t('workspaceDisplay.unavailable') }} ·
					{{ principal?.super ? t('workspaceCopy.w403') : t('workspaceCopy.w63') }}
				</p>
			</div>
			<n-button :loading="loading" @click="refresh">{{ t('workspaceCopy.w47') }}</n-button>
		</header>
		<n-alert type="info" class="mb-4">{{ t('workspaceCopy.w48') }}</n-alert>
		<n-tabs v-model:value="tab" type="line" @update:value="changeTab">
			<n-tab v-for="item in tabs" :key="item.key" :name="item.key">{{ item.label }}</n-tab>
		</n-tabs>
		<div class="toolbar">
			<n-input
				v-model:value="keyword"
				clearable
				:placeholder="t('workspaceCopy.w0')"
				class="search"
				@update:value="search" />
			<n-button v-if="mayCreate" type="primary" @click="openEditor()">{{
				tab === 'tasks' ? t('workspaceCopy.w64') : t('workspaceCopy.w376')
			}}</n-button>
			<n-button v-if="tab === 'contacts' && can('contact:import')" @click="showImport = true">{{
				t('workspaceCopy.w49')
			}}</n-button>
			<n-button v-if="tab === 'contacts' && can('contact:export')" @click="exportCSV">{{
				t('workspaceCopy.w50')
			}}</n-button>
		</div>
		<n-data-table
			:columns="columns"
			:data="rows"
			:loading="loading"
			:row-key="row => row.id"
			:scroll-x="1100" />
		<div class="pager">
			<n-pagination
				v-model:page="page"
				:page-size="20"
				:item-count="total"
				@update:page="refresh" />
		</div>
		<n-modal
			:show="workbenchActive && editing"
			@update:show="editing = $event"
			preset="card"
			:title="editorTitle"
			class="workspace-editor">
			<n-form label-placement="top">
				<template v-if="tab === 'departments'">
					<n-form-item :label="t('workspaceCopy.w400')"
						><n-input v-model:value="draft.name"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w1')"
						><remote-select v-model="draft.parent_id" type="departments"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w2')"
						><n-switch v-model:value="draft.active"
					/></n-form-item>
				</template>
				<template v-else-if="tab === 'users'">
					<n-form-item :label="t('workspaceCopy.w3')"
						><n-input v-model:value="draft.username"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w4')"
						><n-input v-model:value="draft.email"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w5')"
						><n-input v-model:value="draft.password" type="password" show-password-on="click"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w6')"
						><remote-select v-model="draft.dept_id" type="departments"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w391')"
						><remote-select v-model="draft.role_ids" type="roles" multiple
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w7')"
						><n-switch v-model:value="draft.status" :checked-value="1" :unchecked-value="0"
					/></n-form-item>
				</template>
				<template v-else-if="tab === 'roles'">
					<n-form-item :label="t('workspaceCopy.w392')"
						><n-input v-model:value="draft.role_name"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w8')"
						><n-select v-model:value="draft.data_scope" :options="scopeOptions"
					/></n-form-item>
					<n-form-item v-if="draft.data_scope === 'custom'" :label="t('workspaceCopy.w9')"
						><remote-select v-model="draft.custom_depts" type="departments" multiple
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w10')"
						><n-checkbox-group :value="draft.permissions" @update:value="selectPermissions"
							><n-space
								><n-checkbox v-for="code in catalog" :key="code" :value="code" :title="code">{{
									permissionLabel(code)
								}} <small>{{ code }}</small></n-checkbox></n-space
							></n-checkbox-group
						></n-form-item
					>
					<n-alert type="info" class="mb-4">{{ t('workspaceDisplay.rolePermissionHint') }}</n-alert>
					<n-form-item :label="t('workspaceCopy.w11')"
						><n-switch v-model:value="draft.status" :checked-value="1" :unchecked-value="0"
					/></n-form-item>
				</template>
				<template v-else-if="tab === 'grants'">
					<n-form-item :label="t('workspaceCopy.w12')"
						><remote-select v-model="draft.dept_id" type="departments"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w13')"
						><remote-select v-model="draft.sender" type="senders"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w14')"
						><n-switch v-model:value="draft.active"
					/></n-form-item>
				</template>
				<template v-else-if="tab === 'policies'">
					<n-form-item :label="t('workspaceCopy.w15')"
						><n-select v-model:value="draft.scope" :options="policyScopes"
					/></n-form-item>
					<n-form-item v-if="draft.scope === 'department'" :label="t('workspaceCopy.w347')"
						><remote-select v-model="draft.scope_key" type="departments"
					/></n-form-item>
					<n-form-item v-else-if="draft.scope === 'sender'" :label="t('workspaceCopy.w16')"
						><remote-select v-model="draft.scope_key" type="senders"
					/></n-form-item>
					<n-form-item v-else-if="draft.scope !== 'global'" :label="t('workspaceCopy.w17')"
						><n-input v-model:value="draft.scope_key"
					/></n-form-item>
					<policy-fields v-model="draft.policy" />
				</template>
				<template v-else-if="tab === 'groups'">
					<n-form-item :label="t('workspaceCopy.w18')"
						><n-input v-model:value="draft.name"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w396')"
						><n-input v-model:value="draft.description" type="textarea"
					/></n-form-item>
					<contact-import-panel
						v-if="!editId"
						:key="groupImportKey"
						ref="groupImportPanel"
						v-model="groupImportDraft"
						:disabled="!can('contact:import')" />
				</template>
				<template v-else-if="tab === 'contacts'">
					<n-form-item :label="t('workspaceCopy.w19')"
						><n-input v-model:value="draft.email"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w20')"
						><remote-select v-model="draft.group_id" type="groups"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w365')"
						><n-input v-model:value="draft.attribs.name"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w394')"
						><n-switch v-model:value="draft.active" :checked-value="1" :unchecked-value="0"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w369')"
						><n-switch v-model:value="draft.status" :checked-value="1" :unchecked-value="0"
					/></n-form-item>
				</template>
				<template v-else-if="tab === 'tags'">
					<n-form-item :label="t('workspaceCopy.w21')"
						><n-input v-model:value="draft.name"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w22')"
						><remote-select v-model="draft.group_id" type="groups"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w401')"
						><n-color-picker v-model:value="draft.color"
					/></n-form-item>
				</template>
				<template v-else-if="tab === 'templates'">
					<n-form-item :label="t('workspaceCopy.w23')"
						><n-input v-model:value="draft.temp_name"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w24')"
						><n-input
							v-model:value="draft.content"
							type="textarea"
							:autosize="{ minRows: 12, maxRows: 25 }"
					/></n-form-item>
					<n-alert type="info"
						>{{ t('workspaceCopy.w51') }}<code v-pre>{{ $t('ui.pbf29e9d89b83') }}</code
						>。</n-alert
					>
					<bt-preview :value="draft.content || ''" :height="360" class="preview" />
				</template>
				<template v-else-if="tab === 'tasks' || tab === 'schedules' || tab === 'automations'">
					<n-form-item :label="t('workspaceCopy.w25')"
						><n-input v-model:value="draft.name"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w26')"
						><remote-select
							v-model="draft.campaign.group_ids"
							type="groups"
							multiple
							@update:model-value="draft.campaign.tag_ids = []"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w27')"
						><remote-select v-model="draft.campaign.senders" type="senders" multiple
					/></n-form-item>
					<n-alert type="info" class="mb-3">{{ $t('repair.rotation_hint') }}</n-alert>
					<n-form-item :label="$t('repair.cc_email')"
						><n-input v-model:value="draft.campaign.cc_email" :placeholder="$t('repair.cc_hint')"
					/></n-form-item>
					<n-form-item :label="t('workspaceCopy.w28')"
						><n-input v-model:value="draft.campaign.full_name"
					/></n-form-item>
					<template v-if="tab !== 'automations'">
						<n-form-item :label="t('workspaceCopy.w29')"
							><remote-select v-model="draft.campaign.template_id" type="templates"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w30')"
							><n-input v-model:value="draft.campaign.subject"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w31')"
							><remote-select
								v-model="draft.campaign.tag_ids"
								type="tags"
								multiple
								:group-ids="draft.campaign.group_ids"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w379')"
							><n-select
								v-model:value="draft.campaign.tag_logic"
								:options="['AND', 'OR', 'NOT'].map(value => ({ value, label: value }))"
						/></n-form-item>
					</template>
					<n-space
						><n-checkbox v-model:checked="draft.campaign.track_open">{{
							t('workspaceCopy.w387')
						}}</n-checkbox
						><n-checkbox v-model:checked="draft.campaign.track_click">{{
							t('workspaceCopy.w388')
						}}</n-checkbox></n-space
					>
					<n-divider>{{ t('workspaceCopy.w353') }}</n-divider
					><policy-fields v-model="draft.campaign.policy" />
					<n-form-item v-if="tab === 'tasks'" :label="t('workspaceCopy.w373')"
						><n-date-picker v-model:value="startAt" type="datetime" clearable
					/></n-form-item>
					<template v-if="tab === 'schedules'">
						<n-divider>{{ t('workspaceCopy.w364') }}</n-divider>
						<n-form-item :label="t('workspaceCopy.w371')"
							><n-select v-model:value="draft.cadence" :options="cadenceOptions"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w32')"
							><n-select
								v-model:value="draft.timezone"
								filterable
								:options="retainChoice(timezoneOptions, draft.timezone)"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w33')"
							><n-date-picker v-model:value="startAt" type="datetime"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w356')"
							><n-time-picker
								v-model:formatted-value="draft.local_time"
								value-format="HH:mm"
								format="HH:mm"
								:clearable="false"
						/></n-form-item>
						<n-form-item v-if="draft.cadence === 'weekly'" :label="t('workspaceCopy.w377')"
							><n-select v-model:value="draft.weekdays" :options="dayOptions" multiple
						/></n-form-item>
						<n-form-item v-if="draft.cadence === 'monthly'" :label="t('workspaceCopy.w34')"
							><n-input-number v-model:value="draft.month_day" :min="1" :max="31"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w378')"
							><n-input-number v-model:value="draft.max_runs" :min="0"
						/></n-form-item>
						<n-form-item :label="t('workspaceCopy.w35')"
							><n-date-picker v-model:value="endsAt" type="datetime" clearable
						/></n-form-item>
					</template>
					<template v-if="tab === 'automations'">
						<n-divider>{{ t('workspaceCopy.w395') }}</n-divider>
						<n-alert type="info">{{ t('workspaceCopy.w402') }}</n-alert>
						<section v-for="(step, index) in draft.steps" :key="index" class="step">
							<n-space justify="space-between"
								><strong>{{ t('workspaceCopy.step', { index: index + 1 }) }}</strong
								><n-button size="small" @click="draft.steps.splice(index, 1)">{{
									t('workspaceCopy.w52')
								}}</n-button></n-space
							>
							<n-form-item :label="t('workspaceCopy.w36')"
								><n-input-number v-model:value="step.delay_seconds" :min="0"
							/></n-form-item>
							<n-form-item :label="t('workspaceCopy.w372')"
								><n-select v-model:value="step.condition" :options="conditionOptions"
							/></n-form-item>
							<n-form-item :label="t('workspaceCopy.w37')"
								><remote-select v-model="step.template_id" type="templates"
							/></n-form-item>
							<n-form-item :label="t('workspaceCopy.w38')"
								><n-input v-model:value="step.subject"
							/></n-form-item>
						</section>
						<n-button
							@click="
								draft.steps.push({
									delay_seconds: 86400,
									condition: 'always',
									template_id: null,
									subject: '',
								})
							"
							>{{ t('workspaceCopy.w53') }}</n-button
						>
					</template>
					<n-form-item v-if="tab !== 'tasks'" :label="t('workspaceCopy.w39')"
						><n-switch v-model:value="draft.active"
					/></n-form-item>
				</template>
			</n-form>
			<n-space justify="end"
				><n-button @click="editing = false">{{ t('workspaceCopy.w54') }}</n-button
				><n-button type="primary" :loading="saving" @click="save">{{
					tab === 'tasks' ? t('workspaceCopy.w351') : t('workspaceCopy.w65')
				}}</n-button></n-space
			>
		</n-modal>
		<n-modal
			:show="workbenchActive && showImport"
			@update:show="showImport = $event"
			preset="card"
			:title="t('workspaceCopy.w40')"
			class="workspace-editor">
			<n-form-item :label="t('workspaceCopy.w41')"
				><remote-select v-model="importGroup" type="groups"
			/></n-form-item>
			<contact-import-panel
				:key="existingImportKey"
				ref="existingImportPanel"
				v-model="existingImportDraft"
				:allow-empty="false" />
			<n-checkbox v-model:checked="confirmed">{{ t('workspaceCopy.w55') }}</n-checkbox>
			<n-space justify="end" class="mt-4"
				><n-button type="primary" :loading="saving" @click="confirmExistingImport">{{
					t('importPreview.confirm_import')
				}}</n-button></n-space
			>
		</n-modal>
		<n-modal
			:show="workbenchActive && showDetail"
			@update:show="showDetail = $event"
			preset="card"
			:title="t('workspaceCopy.w42')"
			class="workspace-details">
			<n-data-table :columns="detailColumns" :data="detailRows" :scroll-x="1000" />
			<n-pagination
				v-model:page="detailPage"
				:page-size="20"
				:item-count="detailTotal"
				@update:page="loadDetails" />
		</n-modal>
		<n-modal
			:show="workbenchActive && showTags"
			@update:show="showTags = $event"
			preset="card"
			:title="t('workspaceCopy.w43')"
			class="workspace-editor">
			<remote-select v-model="tagIds" type="tags" :group-id="tagContact?.group_id" multiple />
			<n-button type="primary" class="mt-4" @click="saveTags">{{
				t('workspaceCopy.w59')
			}}</n-button>
		</n-modal>
		<n-modal
			:show="workbenchActive && showTaskPolicy"
			@update:show="showTaskPolicy = $event"
			preset="card"
			:title="t('workspaceCopy.w44')"
			class="workspace-editor">
			<policy-fields v-model="taskPolicy" />
			<n-button type="primary" class="mt-4" :loading="saving" @click="saveTaskPolicy">{{
				t('workspaceCopy.w60')
			}}</n-button>
		</n-modal>
		<n-modal
			:show="workbenchActive && showAdopt"
			@update:show="showAdopt = $event"
			preset="card"
			:title="t('workspaceCopy.w45')"
			class="workspace-editor">
			<p>{{ t('workspaceCopy.w61') }}</p>
			<remote-select v-model="adoptOwner" type="users" />
			<n-button type="primary" class="mt-4" @click="adopt">{{ t('workspaceCopy.w62') }}</n-button>
		</n-modal>
	</div>
</template>

<script setup lang="ts">
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import ContactImportPanel from '@/features/contacts/ContactImportPanel.vue'
import { emptyContactImport } from '@/features/contacts/import-state'
import marketData from '@/features/growth/market-data.json'
import { retainChoice } from '@/features/growth/options'
const timezoneOptions = marketData.timezones.map(value => ({
	value,
	label: value.replaceAll('_', ' '),
}))
const groupImportDraft = ref(emptyContactImport()),
	groupImportPanel = useTemplateRef('groupImportPanel'),
	groupImportKey = ref(0)
const existingImportDraft = ref(emptyContactImport()),
	existingImportPanel = useTemplateRef('existingImportPanel'),
	existingImportKey = ref(0)
const { t } = useI18n()
import { ref, computed, h, onMounted, onBeforeUnmount, watch } from 'vue'
import { NButton, NSpace, NPopconfirm } from 'naive-ui'
import { Message } from '@/utils'

import { workspace, principal, catalog, permissionDependencies, can, loadPrincipal } from '@/features/workspace/api'
import RemoteSelect from '@/features/workspace/RemoteSelect.vue'
import { getDefaultSender } from '@/features/workspace/defaultSender'
import PolicyFields from '@/features/workspace/PolicyFields.vue'
import { asArray, normalizePolicy, normalizeCampaign, completePermissionSelection } from '@/features/workspace/normalize'
import { formatWorkspaceCell, workspacePermissionLabel } from '@/features/workspace/display'
import { sectionTabs } from '@/features/workspace/sections'
import type { WorkspaceSection } from '@/features/workspace/sections'

const props = defineProps<{ section?: WorkspaceSection; initialTab?: string }>()
const sectionTitle = computed(() => t(props.section ? {
	marketing: 'layout.menu.market',
	templates: 'layout.menu.template',
	contacts: 'layout.menu.contacts',
}[props.section] : 'workspaceCopy.w46'))
const message = Message
const tab = ref(props.initialTab || 'tasks'),
	keyword = ref(''),
	page = ref(1),
	total = ref(0),
	rows = ref<any[]>([]),
	loading = ref(false)
const editing = ref(false),
	saving = ref(false),
	draft = ref<any>({}),
	editId = ref<number | null>(null),
	startAt = ref<number | null>(null),
	endsAt = ref<number | null>(null)
let requestSerial = 0,
	timer: ReturnType<typeof setTimeout> | undefined
const definitions = computed(() => [
	{ key: 'tasks', label: t('workspaceCopy.w66'), read: 'campaign:list', write: 'campaign:create' },
	{
		key: 'schedules',
		label: t('workspaceCopy.w67'),
		read: 'schedule:list',
		write: 'schedule:manage',
	},
	{
		key: 'automations',
		label: t('workspaceCopy.w68'),
		read: 'automation:list',
		write: 'automation:manage',
	},
	{ key: 'groups', label: t('workspaceCopy.w69'), read: 'contact:list', write: 'contact:create' },
	{ key: 'contacts', label: t('workspaceCopy.w70'), read: 'contact:list', write: 'contact:create' },
	{ key: 'tags', label: t('workspaceCopy.w71'), read: 'contact:list', write: 'contact:create' },
	{
		key: 'templates',
		label: t('workspaceCopy.w72'),
		read: 'template:list',
		write: 'template:create',
	},
	{ key: 'reports', label: t('workspaceCopy.w73'), read: 'report:list', write: '' },
	...[
		t('workspaceCopy.w404'),
		t('workspaceCopy.w405'),
		t('workspaceCopy.w74'),
		t('workspaceCopy.w75'),
		t('workspaceCopy.w76'),
		t('workspaceCopy.w77'),
	].map(value => {
		const [key, label] = value.split(':')
		return { key, label, read: '', write: '', admin: true }
	}),
])
const tabs = computed(() =>
	definitions.value.filter(item =>
		(!props.section || sectionTabs[props.section].includes(item.key)) &&
		('admin' in item ? principal.value?.super : can(item.read))
	)
)
const mayCreate = computed(
	() =>
		!['audit', 'reports'].includes(tab.value) &&
		(principal.value?.super ||
			can(definitions.value.find(item => item.key === tab.value)?.write || ''))
)
const editorTitle = computed(
	() =>
		`${editId.value ? t('workspaceCopy.w78') : t('workspaceCopy.w376')}${definitions.value.find(item => item.key === tab.value)?.label || ''}`
)
const scopeOptions = computed(() => [
	{ value: 'self', label: t('workspaceCopy.w79') },
	{ value: 'department', label: t('workspaceCopy.w80') },
	{ value: 'descendants', label: t('workspaceCopy.w81') },
	{ value: 'custom', label: t('workspaceCopy.w82') },
	{ value: 'all', label: t('workspaceCopy.w83') },
])
const policyScopes = computed(() => [
	{ value: 'global', label: t('workspaceCopy.w84') },
	{ value: 'department', label: t('workspaceCopy.w347') },
	{ value: 'sender', label: t('workspaceCopy.w85') },
	{ value: 'ip', label: t('workspaceCopy.w86') },
	{ value: 'provider', label: t('workspaceCopy.w87') },
])
const cadenceOptions = computed(() => [
	{ value: 'once', label: t('workspaceCopy.w88') },
	{ value: 'daily', label: t('workspaceCopy.w89') },
	{ value: 'weekly', label: t('workspaceCopy.w90') },
	{ value: 'monthly', label: t('workspaceCopy.w91') },
])
const dayOptions = computed(() =>
	[
		t('workspaceCopy.w357'),
		t('workspaceCopy.w358'),
		t('workspaceCopy.w359'),
		t('workspaceCopy.w360'),
		t('workspaceCopy.w361'),
		t('workspaceCopy.w362'),
		t('workspaceCopy.w363'),
	].map((label, value) => ({ label, value }))
)
const conditionOptions = computed(() => [
	{ value: 'always', label: t('workspaceCopy.w92') },
	{ value: 'opened', label: t('workspaceCopy.w93') },
	{ value: 'clicked', label: t('workspaceCopy.w94') },
	{ value: 'not_opened', label: t('workspaceCopy.w95') },
])
const stateLabels = computed<Record<string, string>>(() => ({
	pending: t('workspaceCopy.w96'),
	retry: t('workspaceCopy.w97'),
	claimed: t('workspaceCopy.w98'),
	sending: t('workspaceCopy.w99'),
	accepted: t('workspaceCopy.w100'),
	failed: t('workspaceCopy.w101'),
	skipped: t('workspaceCopy.w102'),
	unknown: t('workspaceCopy.w103'),
	waiting: t('workspaceCopy.w385'),
	completed: t('workspaceCopy.w104'),
	stopped: t('workspaceCopy.w105'),
}))
function permissionLabel(code: string) {
	return workspacePermissionLabel(code, t)
}
function selectPermissions(codes: string[]) {
	draft.value.permissions = completePermissionSelection(codes, permissionDependencies.value)
}
function json(value: any, fallback: any = {}) {
	if (value == null) return fallback
	if (typeof value === 'string') {
		try {
			return JSON.parse(value)
		} catch {
			return fallback
		}
	}
	return value
}
const policy = () => ({
	per_minute: 30,
	per_hour: 500,
	per_day: 2000,
	min_interval_ms: 2000,
	concurrency: 1,
	recipient_cooldown_hours: 24,
	batch_size: 0,
	batch_rest_seconds: 0,
	timezone: 'Asia/Shanghai',
	window_start: '',
	window_end: '',
	weekdays: [],
})
const campaign = () => ({
	group_ids: [],
	senders: [],
	cc_email: '',
	group_id: null,
	template_id: null,
	sender: null,
	subject: '',
	full_name: '',
	tag_ids: [],
	tag_logic: 'AND',
	policy: policy(),
	track_open: true,
	track_click: true,
})
function openEditor(row?: any) {
	groupImportDraft.value = emptyContactImport()
	groupImportKey.value++
	editId.value = row?.id ? Number(row.id) : null
	draft.value = {
		name: '',
		username: '',
		email: '',
		role_name: '',
		description: '',
		status: 1,
		active: true,
		dept_id: principal.value?.dept_id,
		parent_id: null,
		role_ids: [],
		permissions: [],
		data_scope: 'department',
		custom_depts: [],
		attribs: {},
		temp_name: '',
		content: '',
		group_id: null,
		color: '#18a058',
		scope: 'global',
		scope_key: '*',
		policy: policy(),
		campaign: campaign(),
		cadence: 'daily',
		timezone: 'Asia/Shanghai',
		local_time: '09:00',
		weekdays: [1, 2, 3, 4, 5],
		month_day: 1,
		max_runs: 0,
		steps: [{ delay_seconds: 0, condition: 'always', template_id: null, subject: '' }],
		...JSON.parse(JSON.stringify(row || {})),
	}
	for (const key of ['permissions', 'role_ids', 'custom_depts', 'weekdays'])
		draft.value[key] = asArray(json(draft.value[key], []))
	draft.value.policy = normalizePolicy(json(draft.value.policy), policy())
	draft.value.attribs = json(draft.value.attribs) || {}
	if (tab.value === 'schedules') draft.value.campaign = json(row?.config, campaign())
	if (tab.value === 'automations') {
		const config = json(row?.config, { campaign: campaign(), steps: draft.value.steps })
		draft.value.campaign = config?.campaign
		draft.value.steps = asArray(config?.steps)
	}
	draft.value.campaign = normalizeCampaign(draft.value.campaign, campaign())
	startAt.value = row?.next_run_at ? new Date(row.next_run_at).getTime() : null
	endsAt.value = row?.ends_at ? new Date(row.ends_at).getTime() : null
	editing.value = true
	if (!row && ['tasks', 'schedules', 'automations'].includes(tab.value)) {
		const target = draft.value
		void getDefaultSender().then(sender => {
			if (sender && editing.value && draft.value === target && !target.campaign.senders.length)
				target.campaign.senders = [sender]
		})
	}
}
async function refresh(silent = false) {
	const serial = ++requestSerial
	if (!silent) loading.value = true
	try {
		const result = await workspace('list', {
			type: tab.value,
			page: page.value,
			page_size: 20,
			keyword: keyword.value,
		})
		if (serial === requestSerial) {
			rows.value = asArray(result.list)
			total.value = Number(result.total) || 0
		}
	} finally {
		if (serial === requestSerial) loading.value = false
	}
}
let liveTimer: ReturnType<typeof setTimeout> | undefined,
	liveMounted = true
async function refreshLive() {
	try {
		if (
			liveMounted &&
			workbenchActive.value &&
			!document.hidden &&
			!loading.value &&
			['tasks', 'reports'].includes(tab.value) &&
			(location.pathname === '/workspace' || !!props.section)
		)
			await refresh(true)
	} catch {
		/* Interceptor displays errors. */
	} finally {
		if (liveMounted) liveTimer = setTimeout(refreshLive, 3000)
	}
}
onMounted(() => {
	liveTimer = setTimeout(refreshLive, 3000)
})
onBeforeUnmount(() => {
	liveMounted = false
	clearTimeout(liveTimer)
})
function changeTab() {
	page.value = 1
	keyword.value = ''
	void refresh()
}
function search() {
	clearTimeout(timer)
	timer = setTimeout(() => {
		page.value = 1
		void refresh()
	}, 300)
}
async function save() {
	saving.value = true
	try {
		let action = 'save',
			data = JSON.parse(JSON.stringify(draft.value))
		if (tab.value === 'groups' && !editId.value) {
			if (!(await groupImportPanel.value?.validate())) return
			if (groupImportDraft.value.file_data) {
				action = 'create_group_import'
				data = { ...data, ...groupImportDraft.value }
			}
		}
		if (tab.value === 'tasks') {
			action = 'create_task'
			data = {
				...data.campaign,
				name: data.name,
				start_at: startAt.value ? Math.floor(startAt.value / 1000) : 0,
			}
		}
		if (tab.value === 'schedules') {
			action = 'save_schedule'
			if (!startAt.value) throw new Error(t('workspaceCopy.w117'))
			data.next_at = new Date(startAt.value).toISOString()
			data.ends_at = endsAt.value ? new Date(endsAt.value).toISOString() : ''
		}
		if (tab.value === 'automations') {
			action = 'save_automation'
			data.config = { campaign: data.campaign, steps: data.steps }
		}
		await workspace(action, { type: tab.value, id: editId.value || 0, data })
		editing.value = false
		message.success(t('workspaceCopy.w350'))
		await refresh()
	} catch (error: any) {
		if (error instanceof Error) message.error(error.message)
	} finally {
		saving.value = false
	}
}
async function control(row: any, operation: string) {
	await workspace('task_control', { id: Number(row.id), data: { operation } })
	await refresh()
}
async function deleteRow(row: any) {
	await workspace('delete', { type: tab.value, id: Number(row.id) })
	await refresh()
}
function button(label: string, handler: () => void) {
	return h(NButton, { size: 'small', onClick: handler }, { default: () => label })
}
const columns = computed<any[]>(() => {
	const fields: Record<string, [string, string][]> = {
		departments: [
			['id', 'ID'],
			['name', t('workspaceCopy.w347')],
			['parent_id', t('workspaceCopy.w349')],
			['active', t('workspaceCopy.w118')],
		],
		users: [
			['id', 'ID'],
			['username', t('workspaceCopy.w119')],
			['dept_id', t('workspaceCopy.w347')],
			['email', t('workspaceCopy.w120')],
			['status', t('workspaceCopy.w121')],
		],
		roles: [
			['id', 'ID'],
			['role_name', t('workspaceCopy.w391')],
			['data_scope', t('workspaceCopy.w390')],
			['permissions', t('workspaceCopy.w122')],
		],
		grants: [
			['id', 'ID'],
			['dept_id', t('workspaceCopy.w347')],
			['sender', t('workspaceCopy.w123')],
			['active', t('workspaceCopy.w124')],
		],
		policies: [
			['scope', t('workspaceCopy.w390')],
			['scope_key', t('workspaceCopy.w380')],
			['policy', t('workspaceCopy.w125')],
		],
		audit: [
			['created_at', t('workspaceCopy.w126')],
			['account_id', t('workspaceCopy.w127')],
			['dept_id', t('workspaceCopy.w347')],
			['action', t('workspaceCopy.w128')],
			['resource_id', t('workspaceCopy.w397')],
		],
		groups: [
			['id', 'ID'],
			['name', t('workspaceCopy.w129')],
			['description', t('workspaceCopy.w396')],
			['dept_id', t('workspaceCopy.w347')],
		],
		contacts: [
			['id', 'ID'],
			['email', t('workspaceCopy.w130')],
			['group_id', t('workspaceCopy.w131')],
			['active', t('workspaceCopy.w394')],
			['status', t('workspaceCopy.w368')],
		],
		reports: [
			['id', t('workspaceCopy.w132')],
			['task_name', t('workspaceCopy.w133')],
			['dept_id', t('workspaceCopy.w347')],
			['recipient_count', t('workspaceCopy.w134')],
			['submitted_count', t('workspaceCopy.w135')],
			['delivered_count', t('workspaceCopy.w367')],
			['bounced_count', t('workspaceCopy.w398')],
			['opened_count', t('workspaceCopy.w136')],
			['clicked_count', t('workspaceCopy.w137')],
		],
		tags: [
			['id', 'ID'],
			['name', t('workspaceCopy.w138')],
			['group_id', t('workspaceCopy.w139')],
		],
		templates: [
			['id', 'ID'],
			['temp_name', t('workspaceCopy.w140')],
			['dept_id', t('workspaceCopy.w347')],
		],
		tasks: [
			['id', 'ID'],
			['task_name', t('workspaceCopy.w141')],
			['addresser', t('workspaceCopy.w142')],
			['recipient_count', t('workspaceCopy.w143')],
			['delivery_counts', t('workspaceCopy.w144')],
			['wait_reason', t('workspaceCopy.w145')],
			['next_attempt_at', t('workspaceCopy.w146')],
		],
		schedules: [
			['id', 'ID'],
			['name', t('workspaceCopy.w147')],
			['cadence', t('workspaceCopy.w148')],
			['next_run_at', t('workspaceCopy.w149')],
			['run_count', t('workspaceCopy.w150')],
			['active', t('workspaceCopy.w151')],
			['last_error', t('workspaceCopy.w152')],
		],
		automations: [
			['id', 'ID'],
			['name', t('workspaceCopy.w153')],
			['group_id', t('workspaceCopy.w154')],
			['active', t('workspaceCopy.w155')],
		],
	}
	const visibleFields = (fields[tab.value] || []).filter(([key]) => key !== 'id')
	if (tab.value === 'users') visibleFields.splice(2, 0, ['role_names', t('workspaceCopy.w391')])
	if (tab.value === 'contacts')
		visibleFields.push(
			['verification_status', t('emailVerification.result')],
			['verification_code', t('importPreview.reason')]
		)
	if (tab.value === 'roles')
		visibleFields.splice(2, 0, ['custom_dept_names', t('workspaceCopy.w9')])
	if (['tasks', 'schedules'].includes(tab.value))
		visibleFields.splice(
			1,
			0,
			['group_names', t('workspaceCopy.w129')],
			['template_name', t('workspaceCopy.w140')]
		)
	const result: any[] = visibleFields.map(([key, title]) => ({
		key,
		title,
		minWidth: 100,
		ellipsis: { tooltip: true },
		render: (row: any) => formatWorkspaceCell(tab.value, key, row, t),
	}))
	if (tab.value === 'tasks')
		result.splice(5, 0, {
			title: t('workspaceCopy.w156'),
			key: 'state',
			width: 90,
			render: (row: any) =>
				Number(row.task_process) === 4
					? t('workspaceCopy.w366')
					: Number(row.task_process) === 2
						? t('workspaceCopy.w370')
						: Number(row.pause) === 1
							? t('workspaceCopy.w157')
							: Number(row.task_process) === 0
								? t('workspaceCopy.w158')
								: t('workspaceCopy.w159'),
		})
	if (!['audit', 'reports'].includes(tab.value))
		result.push({
			title: t('workspaceCopy.w160'),
			key: 'actions',
			fixed: 'right',
			width: tab.value === 'tasks' ? 360 : 230,
			render: (row: any) => {
				const actions = []
				if (principal.value?.super && ['tasks', 'schedules', 'automations'].includes(tab.value))
					actions.push(
						button(t('workspaceCopy.w161'), () => {
							adoptID.value = Number(row.id)
							adoptOwner.value = Number(row.owner_id)
							showAdopt.value = true
						})
					)
				if (tab.value === 'tasks') {
					if (can('campaign:publish'))
						actions.push(button(t('emailVerification.recheck'), () => control(row, 'recheck')))
					if (Number(row.task_process) < 2) {
						if (can('campaign:publish') && Number(row.pause) === 1)
							actions.push(button(t('workspaceCopy.w162'), () => control(row, 'publish')))
						if (can('campaign:pause') && Number(row.pause) === 0)
							actions.push(button(t('workspaceCopy.w163'), () => control(row, 'pause')))
						if (can('campaign:cancel'))
							actions.push(
								h(
									NPopconfirm,
									{ onPositiveClick: () => control(row, 'cancel') },
									{
										trigger: () => button(t('workspaceCopy.w164'), () => {}),
										default: () => t('workspaceCopy.w165'),
									}
								)
							)
					}
					if (Number(row.task_process) < 2 && can('campaign:pause'))
						actions.push(
							button(t('workspaceCopy.w166'), () => {
								policyTaskID.value = Number(row.id)
								taskPolicy.value = normalizePolicy(json(row.send_policy), policy())
								showTaskPolicy.value = true
							})
						)
					actions.push(button(t('workspaceCopy.w167'), () => details('deliveries', row)))
				} else {
					const write =
						tab.value === 'templates'
							? 'template:update'
							: ['groups', 'contacts', 'tags'].includes(tab.value)
								? 'contact:update'
								: definitions.value.find(item => item.key === tab.value)?.write || ''
					if ((principal.value?.super || can(write)) && row.role_name !== 'admin')
						actions.push(button(t('workspaceCopy.w168'), () => openEditor(row)))
					if (tab.value === 'schedules')
						actions.push(button(t('workspaceCopy.w169'), () => details('runs', row)))
					if (tab.value === 'automations')
						actions.push(button(t('workspaceCopy.w170'), () => details('enrollments', row)))
					if (tab.value === 'contacts' && can('contact:update'))
						actions.push(
							button(t('workspaceCopy.w171'), () => {
								tagContact.value = row
								tagIds.value = []
								showTags.value = true
							})
						)
					const del = tab.value === 'templates' ? 'template:delete' : 'contact:delete'
					if (['contacts', 'groups', 'tags', 'templates'].includes(tab.value) && can(del))
						actions.push(
							h(
								NPopconfirm,
								{ onPositiveClick: () => deleteRow(row) },
								{
									trigger: () => button(t('workspaceCopy.w172'), () => {}),
									default: () => t('workspaceCopy.w173'),
								}
							)
						)
				}
				return h(NSpace, { size: 6 }, { default: () => actions })
			},
		})
	return result
})
const showImport = ref(false),
	importGroup = ref<number | null>(null),
	confirmed = ref(false)
watch(showImport, value => {
	if (value) {
		existingImportDraft.value = emptyContactImport()
		existingImportKey.value++
	}
})
async function confirmExistingImport() {
	if (!importGroup.value) {
		message.error(t('workspaceCopy.w41'))
		return
	}
	if (!(await existingImportPanel.value?.validate())) return
	saving.value = true
	try {
		await workspace('import', {
			data: {
				group_id: importGroup.value,
				csv: existingImportDraft.value.file_data,
				file_type: 'xlsx',
				filter_keywords: existingImportDraft.value.filter_keywords,
				preview_token: existingImportDraft.value.preview_token,
				skip_invalid: existingImportDraft.value.skip_invalid,
				keep_pending: existingImportDraft.value.keep_pending,
				validation_sender: existingImportDraft.value.validation_sender,
				confirmed: confirmed.value,
			},
		})
		message.success(t('workspaceCopy.w350'))
		showImport.value = false
		await refresh()
	} finally {
		saving.value = false
	}
}
function download(name: string, content: string) {
	const url = URL.createObjectURL(
		new Blob(['\ufeff' + content], { type: 'text/csv;charset=utf-8' })
	)
	const link = document.createElement('a')
	link.href = url
	link.download = name
	link.click()
	URL.revokeObjectURL(url)
}
async function exportCSV() {
	const result = await workspace('export')
	download('contacts.csv', result.csv)
}
const showDetail = ref(false),
	detailRows = ref<any[]>([]),
	detailPage = ref(1),
	detailTotal = ref(0),
	detailType = ref('deliveries'),
	detailID = ref(0)
async function details(type: string, row: any) {
	detailType.value = type
	detailID.value = Number(row.id)
	detailPage.value = 1
	showDetail.value = true
	await loadDetails()
}
async function loadDetails() {
	const result = await workspace('list', {
		type: detailType.value,
		id: detailID.value,
		page: detailPage.value,
		page_size: 20,
	})
	detailRows.value = asArray(result.list)
	detailTotal.value = Number(result.total) || 0
}
const detailColumns = computed<any[]>(() => {
	const keys =
		detailType.value === 'deliveries'
			? ['recipient', 'delivery_state', 'attempts', 'last_error', 'next_attempt_at', 'message_id']
			: detailType.value === 'runs'
				? ['due_at', 'task_id']
				: ['contact_id', 'step', 'state', 'next_at', 'last_error']
	const cols: any[] = keys.map(key => ({
		key,
		title: (
			{
				recipient: t('workspaceCopy.w174'),
				delivery_state: t('workspaceCopy.w175'),
				attempts: t('workspaceCopy.w176'),
				last_error: t('workspaceCopy.w177'),
				next_attempt_at: t('workspaceCopy.w178'),
				message_id: t('workspaceCopy.w179'),
				due_at: t('workspaceCopy.w180'),
				task_id: t('workspaceCopy.w181'),
				contact_id: t('workspaceCopy.w182'),
				step: t('workspaceCopy.w183'),
				state: t('workspaceCopy.w184'),
				next_at: t('workspaceCopy.w185'),
			} as any
		)[key],
		minWidth: 130,
		render: (row: any) => formatWorkspaceCell(detailType.value, key, row, t),
	}))
	if (detailType.value === 'deliveries' && principal.value?.super)
		cols.push({
			key: 'reconcile',
			title: t('workspaceCopy.w186'),
			minWidth: 210,
			render: (row: any) =>
				row.delivery_state === 'unknown'
					? h(
							NSpace,
							{},
							{
								default: () =>
									['accepted', 'failed'].map(state =>
										h(
											NPopconfirm,
											{
												onPositiveClick: async () => {
													await workspace('reconcile', {
														id: Number(row.id),
														data: { state, reason: t('workspaceCopy.w187') },
													})
													await loadDetails()
												},
											},
											{
												trigger: () =>
													button(
														state === 'accepted'
															? t('workspaceCopy.w188')
															: t('workspaceCopy.w189'),
														() => {}
													),
												default: () => t('workspaceCopy.w190'),
											}
										)
									),
							}
						)
					: '',
		})
	return cols
})
const showTags = ref(false),
	tagContact = ref<any>(null),
	tagIds = ref<number[]>([])
const showTaskPolicy = ref(false),
	policyTaskID = ref(0),
	taskPolicy = ref<Record<string, any>>(policy())
const showAdopt = ref(false),
	adoptID = ref(0),
	adoptOwner = ref<number | null>(null)
async function adopt() {
	await workspace('adopt', {
		type: tab.value,
		id: adoptID.value,
		data: { owner_id: adoptOwner.value },
	})
	showAdopt.value = false
	await refresh()
}
async function saveTaskPolicy() {
	saving.value = true
	try {
		await workspace('task_control', {
			id: policyTaskID.value,
			data: { operation: 'policy', policy: taskPolicy.value },
		})
		showTaskPolicy.value = false
		await refresh()
	} finally {
		saving.value = false
	}
}
async function saveTags() {
	await workspace('assign_tags', {
		data: { contact_ids: [Number(tagContact.value.id)], tag_ids: tagIds.value },
	})
	showTags.value = false
	message.success(t('workspaceCopy.w191'))
}
onMounted(async () => {
	await loadPrincipal()
	if (!tabs.value.some(item => item.key === tab.value) && tabs.value.length)
		tab.value = tabs.value[0].key
	if (tabs.value.length) await refresh()
})
onBeforeUnmount(() => {
	clearTimeout(timer)
	requestSerial++
})
</script>
<style scoped>
.workspace {
	padding: 24px;
	max-width: 1600px;
	margin: auto;
}
header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 20px;
}
h1 {
	font-size: 24px;
	font-weight: 600;
	margin: 0 0 6px;
}
header p {
	color: #777;
	margin: 0;
}
.toolbar {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 12px;
	margin: 18px 0;
}
.search {
	max-width: 360px;
}
.pager {
	display: flex;
	justify-content: flex-end;
	margin-top: 20px;
}
:global(.workspace-editor) {
	width: min(860px, 94vw);
	max-height: 90vh;
	overflow: auto;
}
:global(.workspace-details) {
	width: min(1300px, 95vw);
	max-height: 90vh;
	overflow: auto;
}
.preview {
	width: 100%;
	height: 300px;
	margin: 16px 0;
	border: 1px solid #ddd;
	background: white;
}
.step {
	border: 1px solid #ddd;
	border-radius: 8px;
	padding: 16px;
	margin: 16px 0;
}
</style>
