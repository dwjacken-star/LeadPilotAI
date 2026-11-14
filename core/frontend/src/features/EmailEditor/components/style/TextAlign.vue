<template>
	<container :label="label || 'Text Align'">
		<template #value>
			<n-radio-group v-model:value="value">
				<n-radio-button value="left">{{ $t('ui.p8ae1c34bd37f') }}</n-radio-button>
				<n-radio-button value="center">{{ $t('ui.pa2391118c814') }}</n-radio-button>
				<n-radio-button value="right">{{ $t('ui.p954daa8b0033') }}</n-radio-button>
			</n-radio-group>
		</template>
	</container>
</template>

<script lang="ts" setup>
import Container from './BaseContainer.vue'

interface Props {
	label?: string
}

withDefaults(defineProps<Props>(), {
	label: '',
})

const value = defineModel<string>('value')
</script>
