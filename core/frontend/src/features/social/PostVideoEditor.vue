<template>
	<n-alert type="warning">{{ t('social.meta_video_blocked') }}</n-alert>
	<p v-if="video">{{ video }}</p>
	<n-checkbox v-model:checked="aiGenerated" :disabled="loading || !editable">
		{{ t('social.video_ai') }}
	</n-checkbox>
	<input
		v-if="editable"
		type="file"
		accept=".mp4,video/mp4"
		:disabled="loading"
		@change="upload" />
	<n-spin v-if="loading" size="small" />
	<video v-if="preview" :src="preview" controls preload="metadata" class="video-preview" />
	<pre v-if="inspection">{{ JSON.stringify(inspection, null, 2) }}</pre>
</template>
<script setup lang="ts">
import { uploadPostVideo } from './api'
const props = defineProps<{ video?: string; variantId: number; editable: boolean }>()
const emit = defineEmits(['update:video'])
const { t } = useI18n()
const loading = ref(false)
const aiGenerated = ref(false)
const preview = ref('')
const inspection = ref<Record<string, unknown> | null>(null)
async function upload(event: Event) {
	const input = event.target as HTMLInputElement
	const file = input.files?.[0]
	if (!file) return
	loading.value = true
	try {
		const result = await uploadPostVideo(file, props.variantId, aiGenerated.value)
		emit('update:video', result.reference)
		inspection.value = result.video_manifest
		if (preview.value) URL.revokeObjectURL(preview.value)
		preview.value = URL.createObjectURL(file)
	} catch {
		/* Shared API displays inspection/upload errors. */
	} finally {
		loading.value = false
		input.value = ''
	}
}
onBeforeUnmount(() => {
	if (preview.value) URL.revokeObjectURL(preview.value)
})
</script>
<style scoped>
.video-preview {
	max-width: 100%;
	max-height: 320px;
}
pre {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
</style>
