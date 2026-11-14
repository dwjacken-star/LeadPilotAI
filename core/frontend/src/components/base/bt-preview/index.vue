<template>
	<iframe
        :title="$t('common.actions.preview')"
        sandbox="allow-popups allow-popups-to-escape-sandbox"
		width="100%"
		:height="height"
		:srcdoc="normalizeEmailHTML(value)"
		:style="{ border: 'none', borderRadius: '4px' }">
	</iframe>
</template>

<script lang="ts" setup>
import {normalizeEmailHTML} from '@/utils/email-content'
defineProps({
	value: {
		type: String,
		default: '',
	},
	height: {
		type: [String, Number],
		default: 520,
	},
})
</script>

<style lang="scss" scoped></style>
