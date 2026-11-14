<template>
	<iframe
        :title="$t('common.actions.preview')"
        sandbox="allow-popups allow-popups-to-escape-sandbox"
		:width="width"
		:height="height"
		:srcdoc="emailPreviewDocument(value)"
		:style="{ border: 'none', borderRadius: '4px', display: 'block', background: '#fff', width: previewLength(width), maxWidth: '100%', height: previewLength(height) }">
	</iframe>
</template>

<script lang="ts" setup>
import { emailPreviewDocument, previewLength } from '@/utils/email-preview'
defineProps({
	value: {
		type: String,
		default: '',
	},
	width: {
		type: [String, Number],
		default: '100%',
	},
	height: {
		type: [String, Number],
		default: 520,
	},
})
</script>

<style lang="scss" scoped></style>
