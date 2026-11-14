<template>
	<n-select :value="selectValue" :multiple="multiple" :options="options" filterable remote clearable
		:loading="loading" :placeholder="placeholder" @update:value="value => emit('update:modelValue', multiple ? (Array.isArray(value) ? value : []) : value)"
		@search="search" @scroll="scroll" @focus="loadFirst" />
</template>
<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { workspace } from './api'
import { asArray } from './normalize'
const props = withDefaults(defineProps<{ modelValue: any; type: string; multiple?: boolean; groupId?: number; groupIds?:number[]; placeholder?: string }>(), { multiple: false, placeholder: '' })
const emit = defineEmits(['update:modelValue'])
const cache = ref(new Map<any, { label: string; value: any }>())
const visible = ref<any[]>([])
const loading = ref(false)
let page = 1, total = 0, keyword = '', serial = 0
let timer: ReturnType<typeof setTimeout> | undefined
const fields: Record<string, string> = { departments: 'name', users: 'username', roles: 'role_name', groups: 'name', tags: 'name', templates: 'temp_name', senders: 'username' }
function add(rows: any[]) {
	for (const row of asArray(rows)) {
		const value = props.type === 'senders' ? row.username : Number(row.id)
		cache.value.set(value, { value, label: String(row[fields[props.type]] || value) })
	}
}
const selectValue = computed(() => props.multiple ? asArray(props.modelValue) : (props.modelValue ?? null))
const options = computed(() => {
	const selected = Array.isArray(props.modelValue) ? props.modelValue : props.modelValue == null ? [] : [props.modelValue]
	return [...new Set([...selected, ...visible.value])].map(value => cache.value.get(value) || { value, label: `#${value}` })
})
async function load(reset = false) {
	const request = ++serial
	if (reset) { page = 1; visible.value = [] }
	loading.value = true
	try {
		const result = await workspace('list', { type: props.type, page, page_size: 20, keyword, data: { group_id: props.groupId,group_ids:props.groupIds } })
		if (request !== serial) return
		const rows = asArray(result.list)
		add(rows); total = Number(result.total) || 0
		visible.value.push(...rows.map((row: any) => props.type === 'senders' ? row.username : Number(row.id)))
	} catch { /* The API interceptor displays request errors; keep selections available. */ } finally { if (request === serial) loading.value = false }
}
function loadFirst() { if (!visible.value.length && !loading.value) void load(true) }
function search(value: string) { clearTimeout(timer); timer = setTimeout(() => { keyword = value; void load(true) }, 300) }
function scroll(event: Event) {
	const target = event.target as HTMLElement
	if (target.scrollTop + target.clientHeight >= target.scrollHeight - 30 && !loading.value && visible.value.length < total) { page++; void load() }
}
watch(() => [props.modelValue, props.groupId], async () => {
	const selected = (Array.isArray(props.modelValue) ? props.modelValue : [props.modelValue]).filter(value => value != null && !cache.value.has(value))
	if (selected.length) {
		try { const result = await workspace('list', { type: props.type, page_size: 200, data: { ids: selected, group_id: props.groupId } }); add(result.list) } catch { /* Request errors are displayed by the interceptor. */ }
	}
}, { immediate: true })
watch(() => props.groupId, () => { keyword = ''; void load(true) })
watch(() => props.groupIds, () => { keyword='';void load(true) },{deep:true})
onBeforeUnmount(() => { serial++; clearTimeout(timer) })
</script>
