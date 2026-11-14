<template>
	<n-button class="expand-trigger" :loading="loading" @click="open">{{
		t('ai_providers.expand_keywords')
	}}</n-button>
	<n-modal
		:show="workbenchActive && show"
		@update:show="show = $event"
		preset="card"
		:title="t('ai_providers.expand_keywords')"
		style="width: min(720px, 94vw)">
		<n-form label-placement="top">
			<n-form-item :label="t('ai_providers.keyword_seed')"
				><n-input v-model:value="seedText" type="textarea" :rows="3"
			/></n-form-item>
			<n-grid :cols="2" :x-gap="16"
				><n-form-item-gi :label="t('growth.country_code')"
					><n-select
						v-model:value="country"
						filterable
						:options="choices.countries" /></n-form-item-gi
				><n-form-item-gi :label="t('growth.language')"
					><n-select
						v-model:value="language"
						filterable
						:options="choices.languages" /></n-form-item-gi
			></n-grid>
			<n-form-item :label="t('growth.model')"
				><AIModelSelect v-model:value="model" filterable :options="models"
			/></n-form-item>
			<n-button type="primary" :loading="loading" @click="generate">{{
				t('ai_providers.generate_suggestions')
			}}</n-button>
		</n-form>
		<n-alert type="info" style="margin: 16px 0">{{ t('ai_providers.keyword_review') }}</n-alert>
		<n-checkbox-group v-model:value="selected"
			><n-space vertical
				><n-checkbox v-for="word in suggestions" :key="word" :value="word">{{
					word
				}}</n-checkbox></n-space
			></n-checkbox-group
		>
		<template #footer
			><n-space justify="end"
				><n-button @click="show = false">{{ t('growth.cancel') }}</n-button
				><n-button type="primary" :disabled="!selected.length" @click="apply">{{
					t('ai_providers.add_selected')
				}}</n-button></n-space
			></template
		>
	</n-modal>
</template>
<script setup lang="ts">
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { growth } from './api'
import marketData from './market-data.json'
import { marketChoices } from './options'
const props = defineProps<{
	seed?: string
	industry?: string
	country?: string
	language?: string
}>()
const value = defineModel<string[]>({ default: () => [] }),
	{ t, locale } = useI18n()
const show = ref(false),
	loading = ref(false),
	seedText = ref(''),
	country = ref('US'),
	language = ref('en'),
	model = ref(''),
	models = ref<any[]>([]),
	suggestions = ref<string[]>([]),
	selected = ref<string[]>([])
const choices = computed(() => marketChoices(marketData, locale.value))
async function open() {
	seedText.value = (value.value || []).join('\n') || props.seed || props.industry || ''
	country.value = props.country || 'US'
	language.value = props.language || 'en'
	suggestions.value = []
	selected.value = []
	show.value = true
	try {
		const result = await growth('text_model_options')
		models.value = result.models || []
		model.value = result.model || ''
	} catch {
		/* Shared API reports errors. */
	}
}
async function generate() {
	if (loading.value) return
	loading.value = true
	try {
		const result = await growth('expand_keywords', {
			data: {
				seed: seedText.value,
				industry: props.industry,
				country: country.value,
				language: language.value,
				model: model.value,
			},
		})
		suggestions.value = result.keywords || []
		selected.value = []
	} catch {
		/* Preserve user input. */
	} finally {
		loading.value = false
	}
}
function apply() {
	const merged = [...(value.value || [])],
		seen = new Set(merged.map(word => word.toLowerCase()))
	for (const word of selected.value) {
		if (!seen.has(word.toLowerCase())) {
			merged.push(word)
			seen.add(word.toLowerCase())
		}
	}
	value.value = merged.slice(0, 50)
	show.value = false
}
</script>
<style scoped>
.expand-trigger {
	margin-top: 10px;
}
</style>
