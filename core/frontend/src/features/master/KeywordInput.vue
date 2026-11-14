<template>
	<div class="keywords">
		<n-input
			v-model:value="text"
			type="textarea"
			:autosize="{ minRows: 2, maxRows: 6 }"
			:placeholder="t('repair.filter_hint')"
			@update:value="update" /><n-space size="small" class="tags"
			><n-tag v-for="word in model" :key="word" size="small">{{ word }}</n-tag></n-space
		>
	</div>
</template>
<script setup lang="ts">
import { parseImportKeywords } from '@/features/contacts/import-state'
const model = defineModel<string[]>({ default: () => [] }),
	{ t } = useI18n(),
	text = ref('')
function update(value: string) {
	model.value = parseImportKeywords(value)
}
watch(
	model,
	value => {
		if (JSON.stringify(parseImportKeywords(text.value)) !== JSON.stringify(value || []))
			text.value = (value || []).join('\n')
	},
	{ immediate: true, deep: true }
)
</script>
<style scoped>
.keywords {
	width: 100%;
}
.tags {
	margin-top: 8px;
}
</style>
