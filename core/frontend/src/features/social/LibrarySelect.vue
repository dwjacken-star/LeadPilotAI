<template>
	<div class="library-select">
		<n-select
			:value="model"
			multiple
			remote
			filterable
			clearable
			:options="options"
			:loading="loading"
			:placeholder="t('social.select_' + kind)"
			@search="search"
			@update:value="v => (model = v)" /><n-button
			v-if="more"
			size="small"
			:loading="loading"
			@click="next"
			>{{ t('social.load_more') }}</n-button
		>
	</div>
</template>
<script setup lang="ts">
import { social } from './api'
import { mergeOptions } from './display'
const props = defineProps<{ companyId: number | null; kind: 'products' | 'assets' }>()
const model = defineModel<(number | string)[]>({ default: () => [] })
const { t } = useI18n()
const options = ref<{ value: any; label: string }[]>([]),
	loading = ref(false),
	more = ref(false)
let page = 1,
	keyword = '',
	serial = 0,
	timer: ReturnType<typeof setTimeout> | undefined,
	alive = true
async function load() {
	if (!props.companyId) {
		options.value = []
		return
	}
	const call = ++serial
	loading.value = true
	try {
		const result = await social('library_options', {
			kind: props.kind,
			page,
			keyword,
			data: { company_id: props.companyId },
		})
		if (!alive || call !== serial) return
		const rows = (result.list || []).map((r: any) => ({
			value: r.id,
			label: props.kind === 'products' ? `${r.sku} · ${r.name}` : r.filename,
		}))
		options.value = mergeOptions(
			page === 1 ? options.value.filter(o => model.value.includes(o.value)) : options.value,
			rows
		)
		more.value = page * 20 < result.total
	} finally {
		if (alive && call === serial) loading.value = false
	}
}
async function hydrate() {
	if (!props.companyId || !model.value.length) return
	const company = props.companyId
	const result = await social('library_options', {
		kind: props.kind,
		page: 1,
		data: { company_id: company, ids: model.value },
	})
	if (alive && company === props.companyId) {
		options.value = mergeOptions(
			options.value,
			(result.list || []).map((r: any) => ({
				value: r.id,
				label: props.kind === 'products' ? `${r.sku} · ${r.name}` : r.filename,
			}))
		)
	}
}
watch(
	model,
	() => {
		void hydrate()
	},
	{ immediate: true }
)
function search(text: string) {
	keyword = text
	page = 1
	clearTimeout(timer)
	timer = setTimeout(() => {
		void load()
	}, 250)
}
function next() {
	page++
	void load()
}
watch(
	() => props.companyId,
	() => {
		page = 1
		keyword = ''
		options.value = []
		void load()
		void hydrate()
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
.library-select {
	width: 100%;
	display: flex;
	gap: 8px;
	align-items: center;
}
.library-select :deep(.n-select) {
	flex: 1;
	min-width: 0;
}
</style>
