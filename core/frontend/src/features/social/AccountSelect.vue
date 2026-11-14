<template>
	<div class="account-picker">
		<n-select
			:value="model"
			multiple
			remote
			filterable
			clearable
			:options="options.filter(o => platforms.includes(o.platform))"
			:loading="loading"
			:placeholder="t('social.account_optional')"
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
import { mergeOptions, socialPlatformName } from './display'
const props = defineProps<{ platforms: string[] }>()
const model = defineModel<number[]>({ default: () => [] }),
	{ t } = useI18n()
const options = ref<any[]>([]),
	loading = ref(false),
	more = ref(false)
let page = 1,
	keyword = '',
	serial = 0,
	alive = true,
	timer: ReturnType<typeof setTimeout> | undefined
const choices = (rows: any[]) =>
	rows.map(r => ({
		value: r.id,
		label:
			socialPlatformName(r.platform) +
			' · ' +
			r.name +
			' · ' +
			t('social.target_' + (r.target_type || 'page')) +
			' · ' +
			t('social.mode_' + (r.publish_mode || 'automatic')),
		platform: r.platform,
	}))
async function load() {
	const call = ++serial
	loading.value = true
	try {
		const result = await social('account_options', {
			page,
			keyword,
			data: { platforms: props.platforms },
		})
		if (alive && call === serial) {
			options.value = mergeOptions(
				page === 1 ? options.value.filter(o => model.value.includes(o.value)) : options.value,
				choices(result.list || [])
			)
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
	model,
	async () => {
		if (!model.value.length) return
		let result
		try {
			result = await social('account_options', { data: { ids: model.value } })
		} catch {
			return
		}
		if (alive) options.value = mergeOptions(options.value, choices(result.list || []))
	},
	{ immediate: true }
)
watch(
	() => props.platforms,
	async () => {
		page = 1
		if (model.value.length) {
			try {
				const result = await social('account_options', {
					data: { ids: model.value, platforms: props.platforms },
				})
				if (alive) model.value = (result.list || []).map((r: any) => r.id)
			} catch {
				/* Preserve selections on errors. */
			}
		}
		void load()
	},
	{ deep: true }
)
onMounted(() => void load())
onBeforeUnmount(() => {
	alive = false
	serial++
	clearTimeout(timer)
})
</script>
<style scoped>
.account-picker {
	display: flex;
	width: 100%;
	gap: 8px;
}
.account-picker :deep(.n-select) {
	flex: 1;
	min-width: 0;
}
</style>
