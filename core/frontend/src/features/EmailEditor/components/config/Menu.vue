<template>
	<Collapse :default-expanded-names="['1', '2', '3']">
		<n-collapse-item :title="$t('ui.p5522729536fd')" name="1">
			<MenuList></MenuList>
		</n-collapse-item>
		<n-collapse-item :title="$t('ui.p99a0efc6cfd8')" name="2">
			<div class="py-8px">
				<StyleForm v-model:value="blockStyle" />
			</div>
		</n-collapse-item>
		<n-collapse-item :title="$t('ui.pe6443af99d2f')" name="3">
			<div class="py-8px">
				<ContainerForm v-model:value="containerStyle" />
			</div>
		</n-collapse-item>
	</Collapse>
</template>

<script lang="tsx" setup>
import {uiText} from '@/i18n/labels'
import { useConfig } from '../../hooks/useConfig'
import { useSetData } from '../../hooks/useSetData'
import { useNormalForm } from '../style/useNormalForm'

import Collapse from '../shared/Collapse.vue'
import MenuList from './menu/List.vue'

const { selectedBlockKey, blockConfigMap } = useConfig()
const { autoSaveFn } = useSetData()

const blockStyle = computed({
	get() {
		return blockConfigMap.value[selectedBlockKey.value].style
	},
	set(newVal) {
		blockConfigMap.value[selectedBlockKey.value].style = newVal
	},
})

const containerStyle = computed({
	get() {
		return blockConfigMap.value[selectedBlockKey.value].containerStyle
	},
	set(newVal) {
		blockConfigMap.value[selectedBlockKey.value].containerStyle = newVal
	},
})

watch(
	() => [blockStyle.value, containerStyle.value],
	() => {
		autoSaveFn()
	},
	{
		deep: true,
	}
)

const [StyleForm] = useNormalForm([
	{
		attrKey: 'fontWeight',
	},
	{
		attrKey: 'fontSize',
	},
	{
		attrKey: 'letterSpacing',
	},
	{
		attrKey: 'color',
		label: uiText('ui.p3a53d53d14ee'),
	},
	{
		attrKey: 'layout',
		key: 'display',
	},
	{
		attrKey: 'padding',
	},
])

const [ContainerForm] = useNormalForm([
	{
		attrKey: 'textAlign',
	},
	{
		attrKey: 'padding',
	},
])
</script>
