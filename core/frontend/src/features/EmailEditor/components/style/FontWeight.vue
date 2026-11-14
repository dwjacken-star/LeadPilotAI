<template>
	<container :label="$t('ui.pcafd75692b0b')">
		<template #value>
			<div class="w-100px">
				<n-select v-model:value="value" :options="options"></n-select>
			</div>
		</template>
	</container>
</template>

<script lang="ts" setup>
import {uiText} from '@/i18n/labels'
import Container from './BaseContainer.vue'

interface Props {
	label?: string
}

withDefaults(defineProps<Props>(), {
	label: '',
})

const value = defineModel<string>('value', {
	default: '',
})

const options = [
	{ label: uiText('ui.p45e118d0563e'), value: 'normal' },
	{ label: uiText('ui.p19e07430eed6'), value: 'bold' },
]

if (!value.value) {
	value.value = options[0].value
}
</script>
