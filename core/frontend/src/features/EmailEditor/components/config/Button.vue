<template>
	<Collapse :default-expanded-names="['1', '2', '3']">
		<n-collapse-item :title="$t('ui.p97c89a4d6630')" name="1">
			<div class="pb-10px">
				<ActionForm v-model:value="attr"></ActionForm>
			</div>
		</n-collapse-item>
		<n-collapse-item :title="$t('ui.pe11c4124ed46')" name="2">
			<div class="py-8px">
				<ButtonOptionsForm v-model:value="blockStyle" />
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
import BtnLink from '../style/Link.vue'

const { selectedBlockKey, blockConfigMap } = useConfig()
const { autoSaveFn } = useSetData()

const attr = computed({
	get() {
		return blockConfigMap.value[selectedBlockKey.value].attr
	},
	set(newVal) {
		blockConfigMap.value[selectedBlockKey.value].attr = newVal
	},
})

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
	() => [attr.value, blockStyle.value, containerStyle.value],
	() => {
		autoSaveFn()
	},
	{
		deep: true,
	}
)

const [ActionForm] = useNormalForm([
	{
		attrKey: 'custom',
		render: () => <BtnLink v-model:value={attr.value!.href} v-model:target={attr.value!.target} />,
	},
])

const [ButtonOptionsForm] = useNormalForm([
	{
		attrKey: 'backgroundColor',
		label: uiText('ui.p874df4e77434'),
	},
	{
		attrKey: 'color',
		label: uiText('ui.p3a53d53d14ee'),
	},
	{
		attrKey: 'widthAuto',
		key: 'width',
		label: uiText('ui.pa58ddf50c5ea'),
	},
	{
		attrKey: 'fontWeight',
	},
	{
		attrKey: 'fontSize',
	},
	{
		attrKey: 'lineHeight',
	},
	{
		attrKey: 'letterSpacing',
	},
	{
		attrKey: 'padding',
	},
	{
		attrKey: 'border',
	},
	{
		attrKey: 'borderRadius',
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
