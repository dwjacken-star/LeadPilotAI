<template>
	<div class="option-picker">
		<n-select
			:value="model"
			:multiple="multiple"
			:disabled="disabled"
			:options="options"
			:loading="loading"
			remote
			filterable
			clearable
			:placeholder="placeholder"
			@search="search"
			@update:value="v => (model = v)" /><n-button
			v-if="more"
			size="small"
			:loading="loading"
			@click="next"
			>{{ t('intel.load_more') }}</n-button
		>
	</div>
</template>
<script setup lang="ts">
import { master } from './api'
import { mergeOptions } from './display'
const props = defineProps<{
	kind: 'companies' | 'products' | 'prospects' | 'rules'
	companyId?: number | null
	multiple?: boolean
	placeholder?: string
	disabled?: boolean
}>()
const model = defineModel<any>({ default: null }),
	{ t } = useI18n()
const options = ref<any[]>([]),
	loading = ref(false),
	more = ref(false)
let page = 1,
	keyword = '',
	serial = 0,
	alive = true,
	timer: ReturnType<typeof setTimeout> | undefined
async function load(hydrate = false) {
	if (props.kind !== 'companies' && !props.companyId) {
		options.value = []
		return
	}
	const seq = ++serial
	loading.value = true
	try {
		const ids = hydrate
			? (Array.isArray(model.value) ? model.value : [model.value]).filter(Boolean)
			: []
		const result = await master('options', {
			company_id: props.companyId,
			page,
			keyword,
			data: { resource: props.kind, ids },
		})
		if (!alive || seq !== serial) return
		const selected = Array.isArray(model.value) ? model.value : [model.value]
		options.value = mergeOptions(
			page === 1 && !hydrate
				? options.value.filter(o => selected.includes(o.value))
				: options.value,
			result.list || []
		)
		if (!hydrate) more.value = page * 20 < result.total
	} catch {
		// The shared API layer displays the error; remote option loading stays retryable.
	} finally {
		if (alive && seq === serial) loading.value = false
	}
}
function search(value: string) {
	keyword = value
	page = 1
	clearTimeout(timer)
	timer = setTimeout(() => void load(), 300)
}
function next() {
	page++
	void load()
}
watch(
	() => [props.companyId, props.kind],
	() => {
		page = 1
		keyword = ''
		options.value = []
		void load()
	},
	{ immediate: true }
)
watch(
	model,
	() => {
		const selected = (Array.isArray(model.value) ? model.value : [model.value]).filter(Boolean)
		if (selected.some(v => !options.value.some(o => o.value === v))) void load(true)
	},
	{ immediate: true }
)
onBeforeUnmount(() => {
	alive = false
	serial++
	clearTimeout(timer)
})
</script>
<style scoped>
.option-picker {
	display: flex;
	align-items: center;
	gap: 8px;
	width: 100%;
}
.option-picker .n-select {
	min-width: 0;
	flex: 1;
}
</style>
