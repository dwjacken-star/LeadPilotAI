<template>
	<modal :title="$t('market.template.preview.title')" width="min(1000px, calc(100vw - 32px))" :footer="false">
		<n-radio-group v-model:value="device" size="small" class="preview-toolbar">
			<n-radio-button value="desktop">{{ $t('growth.desktop') }}</n-radio-button>
			<n-radio-button value="mobile">{{ $t('growth.mobile') }}</n-radio-button>
		</n-radio-group>
		<div class="preview-canvas">
			<bt-preview :width="device === 'mobile' ? 375 : '100%'" height="min(640px, calc(100vh - 220px))" :value="content" />
		</div>
	</modal>
</template>

<script lang="ts" setup>
import { useModal } from '@/hooks/modal/useModal'

const content = ref('')
const device = ref<'desktop' | 'mobile'>('desktop')

const [Modal, modalApi] = useModal({
	onChangeState: isOpen => {
		if (isOpen) {
			device.value = 'desktop'
			const state = modalApi.getState<{ html: string }>()
			content.value = state.html || ''
		}
	},
})
</script>

<style scoped>
.preview-toolbar { margin-bottom: 12px; }
.preview-canvas { display: flex; justify-content: center; padding: 12px; background: #f2f5f8; border-radius: 6px; overflow: auto; }
</style>
