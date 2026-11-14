<template>
	<n-select
		:value="selectValue" :multiple="multiple" :options="options" filterable remote clearable
		:loading="loading" :placeholder="placeholder" :virtual-scroll="!mailboxInventory"
		@update:value="value => emit('update:modelValue', multiple ? (Array.isArray(value) ? value : []) : value)"
		@search="search" @scroll="scroll" @update:show="handleShow">
		<template #action>
			<div v-if="mailboxInventory && total > 0" class="inventory-footer">
				<span>{{ visible.length }} / {{ total }}</span>
				<n-button v-if="page * 20 < total" size="small" :loading="loading" @mousedown.prevent @click.stop="load()">
					{{ $t('intel.load_more') }}
				</n-button>
			</div>
		</template>
	</n-select>
</template>
<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { workspace } from './api'
import { asArray } from './normalize'
const props = withDefaults(defineProps<{ modelValue: any; type: string; multiple?: boolean; groupId?: number; groupIds?: number[]; placeholder?: string; mailboxInventory?: boolean }>(), { multiple: false, placeholder: '', mailboxInventory: false })
const emit = defineEmits(['update:modelValue'])
const cache = ref(new Map<any, { label: string; value: any }>())
const visible = ref<any[]>([])
const loading = ref(false)
let page = 0, total = 0, keyword = '', serial = 0, alive = true, shown = false
let timer: ReturnType<typeof setTimeout> | undefined
const fields: Record<string, string> = { departments: 'name', users: 'username', roles: 'role_name', groups: 'name', tags: 'name', templates: 'temp_name', senders: 'username' }
const contextKey = () => JSON.stringify([props.type, props.groupId, props.groupIds || [], props.mailboxInventory])
const requestData = () => ({ group_id: props.groupId, group_ids: props.groupIds, ...(props.mailboxInventory ? { purpose: 'grant_editor' } : {}) })
const rowValue = (row: any) => props.type === 'senders' ? row.username : Number(row.id)
function add(rows: any[]) {
	for (const row of asArray(rows)) {
		const value = rowValue(row)
		cache.value.set(value, { value, label: String(row[fields[props.type]] || value) })
	}
}
const selectValue = computed(() => props.multiple ? asArray(props.modelValue) : (props.modelValue ?? null))
const options = computed(() => {
	const selected = Array.isArray(props.modelValue) ? props.modelValue : props.modelValue == null ? [] : [props.modelValue]
	return [...new Set([...selected, ...visible.value])].map(value => cache.value.get(value) || { value, label: props.type === 'senders' ? String(value) : `#${value}` })
})
async function load(reset = false) {
	const request = ++serial
	const requestedPage = reset ? 1 : page + 1
	if (reset) visible.value = []
	loading.value = true
	try {
		const result = await workspace('list', { type: props.type, page: requestedPage, page_size: 20, keyword, data: requestData() })
		if (!alive || request !== serial) return
		const rows = asArray(result.list)
		add(rows)
		total = Number(result.total) || 0
		page = requestedPage // A failed request must not skip a page on retry.
		visible.value = [...new Set([...visible.value, ...rows.map(rowValue)])]
	} catch { /* The API interceptor presents errors; keep selected labels. */ }
	finally { if (alive && request === serial) loading.value = false }
}
function handleShow(value: boolean) {
	shown = value
	if (value) {
		// Refresh on every opening, including reopening an existing grant after
		// another page has created a mailbox. Clear the previous search filter.
		clearTimeout(timer)
		keyword = ''
		void load(true)
	}
}
function search(value: string) {
	clearTimeout(timer)
	keyword = value
	serial++ // Invalidate a response for the previous search immediately.
	timer = setTimeout(() => void load(true), 300)
}
function scroll(event: Event) {
	const target = event.target as HTMLElement
	if (target.scrollTop + target.clientHeight >= target.scrollHeight - 30 && !loading.value && page * 20 < total) void load()
}
watch(() => [props.type, props.groupId, JSON.stringify(props.groupIds || []), props.mailboxInventory], () => {
	serial++
	clearTimeout(timer)
	keyword = ''
	page = 0
	total = 0
	visible.value = []
	cache.value.clear()
	loading.value = false
	if (shown) void load(true)
})
watch(() => [props.modelValue, props.type, props.groupId, JSON.stringify(props.groupIds || []), props.mailboxInventory], async () => {
	const selected = (Array.isArray(props.modelValue) ? props.modelValue : [props.modelValue]).filter(value => value != null && !cache.value.has(value))
	if (!selected.length) return
	const context = contextKey()
	try {
		const result = await workspace('list', { type: props.type, page_size: 200, data: { ...requestData(), ids: selected } })
		if (alive && context === contextKey()) add(result.list)
	} catch { /* The API interceptor presents errors. */ }
}, { immediate: true, deep: true })
onBeforeUnmount(() => { alive = false; serial++; clearTimeout(timer) })
</script>
<style scoped>
.inventory-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: var(--color-text-2); }
</style>
