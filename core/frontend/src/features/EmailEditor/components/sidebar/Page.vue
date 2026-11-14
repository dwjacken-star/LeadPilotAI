<template>
	<div class="p-20px">
		<StyleForm v-model:value="pageStyle" />
	</div>
</template>

<script setup lang="ts">
import {uiText} from '@/i18n/labels'
import { useConfig } from '../../hooks/useConfig'
import { useSetData } from '../../hooks/useSetData'
import { useNormalForm } from '../style/useNormalForm'

const { pageConfig } = useConfig()
const { autoSaveFn } = useSetData()

const pageStyle = computed({
	get() {
		return pageConfig.value.style
	},
	set(newVal) {
		pageConfig.value.style = newVal
	},
})

watch(
	() => [pageStyle.value],
	() => {
		autoSaveFn()
	},
	{
		deep: true,
	}
)

const [StyleForm] = useNormalForm([
	{
		attrKey: 'backgroundColor',
		label: uiText('ui.p874df4e77434'),
	},
	{
		attrKey: 'widthNumber',
		label: uiText('ui.pa58ddf50c5ea'),
		key: 'width',
	},
])
</script>
