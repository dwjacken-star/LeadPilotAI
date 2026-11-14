<template>
	<iframe
        :title="$t('common.actions.preview')"
        sandbox="allow-popups allow-popups-to-escape-sandbox"
		:width="width"
		:height="height"
		:srcdoc="emailPreviewDocument(value, palette)"
		:style="{ border: 'none', borderRadius: '4px', display: 'block', background: palette.backgroundColor, width: previewLength(width), maxWidth: '100%', height: previewLength(height) }">
	</iframe>
</template>

<script lang="ts" setup>
import { onMounted, ref, watch } from 'vue'
import { useThemeStore } from '@/store'
import { emailPreviewDocument, previewLength, defaultPreviewPalette } from '@/utils/email-preview'

const themeStore = useThemeStore()
const palette = ref(defaultPreviewPalette(themeStore.theme === 'dark'))
function refreshPalette() {
	const defaults = defaultPreviewPalette(themeStore.theme === 'dark')
	const css = typeof document === 'undefined' ? null : getComputedStyle(document.documentElement)
	palette.value = {
		backgroundColor: css?.getPropertyValue('--color-bg-1').trim() || defaults.backgroundColor,
		textColor: css?.getPropertyValue('--color-text-1').trim() || defaults.textColor,
	}
}
watch(() => themeStore.theme, refreshPalette, { immediate: true, flush: 'post' })
onMounted(refreshPalette)
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
