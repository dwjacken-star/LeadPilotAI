<template>
	<n-modal
		:show="workbenchActive && show"
		@update:show="show = $event"
		preset="card"
		draggable
		:close-on-esc="false"
		:mask-closable="false"
		:title="$t('ui.pe0f7f215ba95')"
		class="w-100">
		<n-form ref="addModelFormRef" :model="addModelFormData" :rules="rules">
			<n-form-item :label="$t('settings.aiModel.modelId')" path="modelId">
				<n-input v-model:value="addModelFormData.modelId"></n-input>
			</n-form-item>
			<n-form-item :label="$t('settings.aiModel.modelAlias')" path="title">
				<n-input v-model:value="addModelFormData.title"></n-input>
			</n-form-item>
			<n-form-item :label="$t('settings.aiModel.modelFeature')" path="capability">
				<n-select
					v-model:value="addModelFormData.capability"
					:options="modelCapabilitiesExclude"
					multiple>
				</n-select>
			</n-form-item>
			<n-form-item :label="$t('settings.aiModel.contentLength')" path="max_tokens">
				<n-input-number
					v-model:value="addModelFormData.max_tokens"
					:min="1"
					class="w-100%"></n-input-number>
			</n-form-item>
		</n-form>
		<template #footer>
			<div class="flex justify-end gap-5">
				<n-button @click="close">{{ $t('ui.p77dfd2135f4d') }}</n-button>
				<n-button type="primary" @click="confirmAddModel(store)">{{ $t('ui.p04a212215ef9') }}</n-button>
			</div>
		</template>
	</n-modal>
</template>

<script setup lang="ts">
import {uiText} from '@/i18n/labels'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import type { SelectOption } from 'naive-ui'
import { ModelStore } from '../dto'
import { confirmAddModel } from '../controller'
const store = inject<ModelStore>('modelStore')!
const { addModelFormData, addModelFormRef } = store
const show = ref(false)
const modelCapabilities: SelectOption[] = [
	{
		label: uiText('ui.p4cddc093d54b'),
		value: 'llm',
	},
	{
		label: uiText('ui.p625f139d6ccd'),
		value: 'vision',
	},
	{
		label: uiText('ui.p0284c6ac58cb'),
		value: 'tools',
	},
	{
		label: 'text-to-image',
		value: 'text-to-image',
	},
]

const modelCapabilitiesExclude = computed(() => {
	if (
		addModelFormData.value.capability.includes('llm') ||
		addModelFormData.value.capability.includes('vision') ||
		addModelFormData.value.capability.includes('tools')
	) {
		return modelCapabilities.map(item => {
			if (item.value === 'text-to-image') {
				return {
					...item,
					disabled: true,
				}
			} else {
				return item
			}
		})
	}

	if (addModelFormData.value.capability.includes('text-to-image')) {
		return modelCapabilities.map(item => {
			if (item.value === 'text-to-image') {
				return {
					...item,
				}
			} else {
				return {
					...item,
					disabled: true,
				}
			}
		})
	}

	return modelCapabilities
})

const rules = {
	modelId: [{ required: true, message: uiText('ui.p0aab296eb710'), trigger: 'blur' }],
	title: [{ required: true, message: uiText('ui.padfdc0b5f4d2'), trigger: 'blur' }],
	capability: [
		{
			validator(_: any, value: any) {
				if (value.length) {
					return true
				} else {
					return new Error('capability is required')
				}
			},
			trigger: 'blur',
		},
	],
	max_tokens: [
		{
			validator(_: any, value: any) {
				if (value) {
					return true
				} else {
					return new Error('max_tokens is required')
				}
			},
			trigger: 'blur',
		},
	],
}

/***
 * @description Open modal
 */
function open() {
	show.value = true
}

/**
 * @description Close  modal
 */
function close() {
	show.value = false
	addModelFormData.value.capability = ['llm']
	addModelFormData.value.max_tokens = 8192
	addModelFormData.value.modelId = ''
	addModelFormData.value.title = ''
}

defineExpose({
	open,
	close,
})
</script>

<style scoped></style>
