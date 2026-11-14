<template>
	<n-alert
		v-if="total"
		type="warning"
		class="mb-16px"
		:title="t('social.manual_due_banner', { count: total })">
		<p>{{ t('social.manual_due_hint') }}</p>
		<n-space>
			<n-button v-for="row in rows" :key="row.id" size="small" @click="emit('open', row)">
				{{ row.account_name }} · {{ row.plan_name }} ·
				{{ socialDate(row.due_at).toLocaleString(locale) }}
			</n-button>
			<n-button size="small" @click="emit('view')">{{ t('social.manual_view_records') }}</n-button>
		</n-space>
	</n-alert>
</template>
<script setup lang="ts">
import { social } from './api'
import { socialDate } from './display'
import { useWorkbenchActivity } from '@/features/workbench/context'
import { can } from '@/features/workspace/api'
const emit = defineEmits<{ open: [row: any]; view: [] }>()
const { t, locale } = useI18n()
const active = useWorkbenchActivity()
const total = ref(0),
	rows = ref<any[]>([])
let alive = true,
	loading = false
async function refresh() {
	if (loading || !can('social:publish')) return
	loading = true
	try {
		const result = await social('manual_reminders')
		if (alive) {
			total.value = Number(result.total) || 0
			rows.value = result.rows || []
		}
	} catch {
		/* Shared API displays errors; retain durable reminders on transient failure. */
	} finally {
		loading = false
	}
}
defineExpose({ refresh })
const timer = setInterval(() => {
	if (active.value) void refresh()
}, 15000)
onMounted(() => void refresh())
onBeforeUnmount(() => {
	alive = false
	clearInterval(timer)
})
</script>
