<template>
	<div class="filter-picker">
		<n-select
			:value="model"
			clearable
			remote
			filterable
			:options="options"
			:loading="loading"
			:placeholder="t('social.all_accounts')"
			@search="search"
			@update:value="value => (model = value)" /><n-button
			v-if="more"
			:loading="loading"
			@click="load(page + 1)"
			>{{ t('social.load_more') }}</n-button
		>
	</div>
</template>
<script setup lang="ts">
import { social } from './api'
import { mergeOptions, socialPlatformName } from './display'
const props = defineProps<{ platform?: string | null; permission: string }>()
const model = defineModel<number | null>({ default: null }),
	{ t } = useI18n()
const options = ref<any[]>([]),
	loading = ref(false),
	more = ref(false)
let page = 1,
	keyword = '',
	serial = 0,
	alive = true,
	timer: ReturnType<typeof setTimeout> | undefined
async function load(next = 1) {
	const call = ++serial
	loading.value = true
	try {
		const result = await social('filter_accounts', {
			platform: props.platform || '',
			page: next,
			keyword,
			data: { permission: props.permission },
		})
		if (alive && call === serial) {
			options.value = mergeOptions(
				next === 1 ? options.value.filter(o => o.value === model.value) : options.value,
				(result.list || []).map((r: any) => ({
					value: r.id,
					label:
						socialPlatformName(r.platform) +
						' · ' +
						r.name +
						' · ' +
						t('social.target_' + (r.target_type || 'page')) +
						' · ' +
						t('social.mode_' + (r.publish_mode || 'automatic')),
				}))
			)
			page = next
			more.value = page * 20 < result.total
		}
	} catch {
		/* Shared API displays errors. */
	} finally {
		if (alive && call === serial) loading.value = false
	}
}
function search(value: string) {
	keyword = value
	clearTimeout(timer)
	timer = setTimeout(() => void load(), 250)
}
watch(
	() => [props.platform, props.permission],
	() => {
		options.value = []
		keyword = ''
		void load()
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
.filter-picker {
	display: flex;
	gap: 8px;
	width: 300px;
	max-width: 100%;
}
.filter-picker :deep(.n-select) {
	flex: 1;
	min-width: 0;
}
</style>
