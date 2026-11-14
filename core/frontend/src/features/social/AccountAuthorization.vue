<template>
	<n-space>
		<n-button
			v-for="platform in ['facebook', 'linkedin', 'instagram', 'threads']"
			v-show="showConnect"
			:key="platform"
			@click="open(platform)"
			>{{ t('social.connect', { platform: platformLabel(platform) }) }}</n-button
		>
		<n-button v-if="tasks.length" @click="show = true"
			>{{ t('social.auth_tasks') }} ({{ tasks.length }})</n-button
		>
	</n-space>
	<n-modal
		:show="active && show"
		preset="card"
		:title="t('social.auth_tasks')"
		style="width: 780px; max-width: 94vw"
		@update:show="show = $event">
		<n-alert type="info">{{ t('social.auth_profiles_hint') }}</n-alert>
		<n-form v-if="!reconnect" label-placement="top" class="mt-16px">
			<n-form-item :label="t('social.platforms')">
				<n-select v-model:value="platform" :options="platformOptions" />
			</n-form-item>
			<n-form-item :label="t('social.target_type')">
				<n-select v-model:value="targetType" :options="targetOptions" />
			</n-form-item>
			<template v-if="manual">
				<n-alert type="info">{{ t('social.manual_account_hint') }}</n-alert>
				<n-form-item :label="t('social.manual_account_label')">
					<n-input v-model:value="manualName" maxlength="200" />
				</n-form-item>
			</template>
			<n-button
				type="primary"
				:loading="busy"
				:disabled="manual ? !manualName.trim() : !availability[platform]?.enabled"
				@click="perform(create)">
				{{ t(manual ? 'social.create_manual_account' : 'social.auth_create') }}
			</n-button>
		</n-form>
		<n-alert v-else type="warning" class="mt-16px">
			{{ t('social.auth_reconnect_hint', { name: reconnect.name, id: reconnect.id }) }}
		</n-alert>
		<n-card v-for="task in tasks" :key="task.id" size="small" class="mt-16px">
			<n-space align="center">
				<b>#{{ task.id }} · {{ platformLabel(task.platform) }}</b>
				<n-tag>{{ t('social.target_' + task.target_type) }}</n-tag>
				<n-tag :type="socialStateClass(task.status)">{{ t('social.states.' + task.status) }}</n-tag>
				<n-button size="small" :loading="refreshing" @click="perform(() => refreshTask(task.id))">{{
					t('growth.refresh')
				}}</n-button>
			</n-space>
			<p v-if="task.expires_at">
				{{ t('social.auth_expires') }}: {{ socialDate(task.expires_at).toLocaleString(locale) }}
			</p>
			<p v-if="task.error" class="hint">{{ task.error }}</p>
			<n-tag v-if="pollingErrors[task.id]" type="warning"
				>{{ t('social.states.unavailable') }} · {{ t('growth.refresh') }}</n-tag
			>
			<template v-if="task.url && ['pending', 'ready'].includes(task.status)">
				<n-input :value="task.url" readonly :aria-label="t('social.auth_link')" />
				<n-space class="mt-8px">
					<n-button size="small" @click="perform(() => copyLink(task.url!))">{{
						t('social.auth_copy_link')
					}}</n-button>
					<n-button
						size="small"
						tag="a"
						:href="task.url"
						target="_blank"
						rel="noopener noreferrer"
						>{{ t('social.auth_open_link') }}</n-button
					>
				</n-space>
			</template>
			<template v-if="task.status === 'ready'">
				<p>{{ t('social.auth_select_hint') }}</p>
				<n-checkbox-group
					:value="selections[task.id] || []"
					@update:value="value => (selections[task.id] = value.map(String))">
					<div v-for="candidate in task.candidates" :key="candidate.external_id" class="candidate">
						<n-checkbox :value="candidate.external_id">
							<b>{{ candidate.name }}</b> · {{ candidate.external_id }}
							<div>
								{{ t('social.auth_identity') }}: {{ candidate.provider_name || '—' }} ·
								{{ candidate.provider_user || '—' }}
							</div>
						</n-checkbox>
					</div>
				</n-checkbox-group>
				<n-checkbox
					:checked="providerChanges[task.id] || false"
					@update:checked="value => (providerChanges[task.id] = value)">
					{{ t('social.auth_provider_change') }}
				</n-checkbox>
				<n-button
					class="mt-8px"
					type="primary"
					:loading="busy"
					:disabled="!canConfirmAuthorization(task, selections[task.id] || [])"
					@click="perform(() => confirmTask(task))">
					{{ t('social.auth_confirm') }}
				</n-button>
			</template>
			<n-space class="mt-8px">
				<n-button
					v-if="['pending', 'ready'].includes(task.status)"
					size="small"
					:loading="busy"
					@click="perform(() => cancelTask(task.id))"
					>{{ t('social.auth_cancel') }}</n-button
				>
				<n-button size="small" @click="forget(task.id)">{{ t('social.auth_forget') }}</n-button>
			</n-space>
		</n-card>
	</n-modal>
</template>
<script setup lang="ts">
import { social } from './api'
import { socialDate, socialStateClass } from './display'
import {
	authorizationStorageKey,
	storedAuthorizationIds,
	canConfirmAuthorization,
} from './authorization'
import type { AuthorizationTask, TargetType } from './authorization'
import { useWorkbenchActivity } from '@/features/workbench/context'
import { Message } from '@/utils'
import { useCopy } from '@/hooks/useCopy'
defineProps<{ availability: Record<string, any>; showConnect: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const { t, locale } = useI18n()
const { copyText } = useCopy()
const active = useWorkbenchActivity()
const show = ref(false),
	busy = ref(false),
	refreshing = ref(false)
const platform = ref('facebook'),
	targetType = ref<TargetType>('page'),
	manualName = ref('')
const reconnect = ref<any>(null),
	tasks = ref<AuthorizationTask[]>([])
const selections = ref<Record<number, string[]>>({}),
	providerChanges = ref<Record<number, boolean>>({}),
	pollingErrors = ref<Record<number, boolean>>({})
const manual = computed(() => platform.value === 'facebook' && targetType.value === 'person')
const platformOptions = [
	{ value: 'facebook', label: 'Facebook' },
	{ value: 'linkedin', label: 'LinkedIn' },
	{ value: 'instagram', label: 'Instagram' },
	{ value: 'threads', label: 'Threads' },
]
const platformLabel = (value: string) =>
	platformOptions.find(option => option.value === value)?.label || value
const targetOptions = computed(() =>
	(platform.value === 'instagram'
		? ['professional']
		: platform.value === 'threads'
			? ['profile']
			: ['page', 'person']
	).map(value => ({ value, label: t('social.target_' + value) }))
)
watch(platform, value => {
	targetType.value =
		value === 'instagram' ? 'professional' : value === 'threads' ? 'profile' : 'page'
})
let alive = true
function persist() {
	try {
		sessionStorage.setItem(
			authorizationStorageKey,
			JSON.stringify(tasks.value.map(task => task.id))
		)
	} catch {
		/* Storage may be disabled. */
	}
}
async function perform(action: () => Promise<void>) {
	if (busy.value) return
	busy.value = true
	try {
		await action()
	} catch {
		/* Shared API displays errors. */
	} finally {
		busy.value = false
	}
}
function open(value: string) {
	platform.value = value
	targetType.value =
		value === 'instagram' ? 'professional' : value === 'threads' ? 'profile' : 'page'
	manualName.value = ''
	reconnect.value = null
	show.value = true
}
async function start(value: string, type: TargetType, account?: any) {
	const result = await social('authorize', {
		data: { platform: value, target_type: type, ...(account ? { account_id: account.id } : {}) },
	})
	if (!alive) return
	tasks.value.push({
		id: result.id,
		url: result.url,
		status: 'pending',
		platform: value,
		target_type: type,
		expected_account_id: account?.id,
		candidates: [],
	})
	persist()
	await refreshTask(result.id)
}
async function create() {
	if (manual.value) {
		await social('create_manual_account', { data: { name: manualName.value.trim() } })
		manualName.value = ''
		emit('changed')
		Message.success(t('social.saved'))
	} else await start(platform.value, targetType.value)
}
async function reconnectAccount(row: any) {
	reconnect.value = row
	show.value = true
	await perform(() => start(row.platform, row.target_type || 'page', row))
}
defineExpose({ reconnectAccount })
async function refreshTask(id: number) {
	try {
		const result = await social('authorization_status', { id })
		if (!alive) return
		const index = tasks.value.findIndex(task => task.id === id)
		if (index < 0) return
		delete pollingErrors.value[id]
		tasks.value[index] = { ...tasks.value[index], ...result, candidates: result.candidates || [] }
		selections.value[id] = (selections.value[id] || []).filter(externalId =>
			tasks.value[index].candidates.some(candidate => candidate.external_id === externalId)
		)
	} catch (error) {
		if (alive && tasks.value.some(task => task.id === id)) pollingErrors.value[id] = true
		throw error
	}
}
async function refreshTasks() {
	if (refreshing.value) return
	refreshing.value = true
	try {
		await Promise.allSettled(
			tasks.value
				.filter(task => ['pending', 'ready'].includes(task.status) && !pollingErrors.value[task.id])
				.map(task => refreshTask(task.id))
		)
	} catch {
		/* Keep tasks available for retry. */
	} finally {
		refreshing.value = false
	}
}
async function confirmTask(task: AuthorizationTask) {
	if (!canConfirmAuthorization(task, selections.value[task.id] || [])) return
	await social('authorization_confirm', {
		id: task.id,
		data: {
			external_ids: selections.value[task.id],
			allow_provider_change: providerChanges.value[task.id] || false,
		},
	})
	await refreshTask(task.id)
	emit('changed')
}
async function cancelTask(id: number) {
	await social('authorization_cancel', { id })
	await refreshTask(id)
}
function forget(id: number) {
	tasks.value = tasks.value.filter(task => task.id !== id)
	delete selections.value[id]
	delete providerChanges.value[id]
	delete pollingErrors.value[id]
	persist()
}
async function copyLink(url: string) {
	await copyText(url)
}
watch(show, value => {
	if (value) void refreshTasks()
})
const timer = setInterval(() => {
	if (show.value && active.value) void refreshTasks()
}, 4000)
onMounted(async () => {
	let ids: number[] = []
	try {
		ids = storedAuthorizationIds(sessionStorage.getItem(authorizationStorageKey))
	} catch {
		/* Storage may be disabled. */
	}
	tasks.value = ids.map(id => ({
		id,
		status: 'pending',
		platform: '',
		target_type: 'page',
		candidates: [],
	}))
	if (ids.length) {
		show.value = true
		await refreshTasks()
	}
})
onBeforeUnmount(() => {
	alive = false
	clearInterval(timer)
})
</script>
<style scoped>
.candidate {
	padding: 10px 0;
	overflow-wrap: anywhere;
}
</style>
