<template>
	<container :label="$t('ui.p972ad8d8a503')">
		<template #value>
			<div class="w-140px">
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
	{ label: uiText('ui.p4f57a1ce99e6'), value: 'inline-block' },
	{ label: uiText('ui.p4b937cc841d8'), value: 'block' },
]

if (!value.value) {
	value.value = options[0].value
}
</script>
