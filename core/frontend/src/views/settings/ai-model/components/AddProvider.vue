<template>
	<n-modal
		:show="workbenchActive && show"
		@update:show="show = $event"
		preset="card"
		:close-on-esc="false"
		:mask-closable="false"
		draggable
		class="w-150"
		:title="$t('ui.peb3851b63ffe')">
		<n-form ref="addProviderFormDataRef" :model="addProviderFormData" :rules="rules">
			<n-form-item :label="$t('settings.aiModel.supplierTitle')" path="supplierTitle">
				<n-input v-model:value="addProviderFormData.supplierTitle"></n-input>
			</n-form-item>
			<n-form-item :label="$t('settings.aiModel.supplierName')" path="supplierName">
				<n-input v-model:value="addProviderFormData.supplierName"></n-input>
			</n-form-item>
			<n-form-item :label="$t('ui.p862d7e297eaf')" path="baseUrl">
				<n-input v-model:value="addProviderFormData.baseUrl"></n-input>
			</n-form-item>
			<n-form-item :label="$t('ui.p6a091b8cbfb6')" path="apiKey">
				<n-input v-model:value="addProviderFormData.apiKey"></n-input>
			</n-form-item>
		</n-form>
		<template #footer>
			<div class="flex justify-end gap-5">
				<n-button>{{ $t('ui.p77dfd2135f4d') }}</n-button>
				<n-button type="primary" @click="confirmAddProvider(store)">{{ $t('ui.p04a212215ef9') }}</n-button>
			</div>
		</template>
	</n-modal>
</template>

<script setup lang="ts">
import {uiText} from '@/i18n/labels'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { confirmAddProvider } from '../controller'
import { ModelStore } from '../dto'
const store = inject<ModelStore>('modelStore')!
const { addProviderFormData, addProviderFormDataRef } = store
const show = ref(false)
const rules = {
	supplierTitle: [{ required: true, message: uiText('ui.p37d72f432838'), trigger: 'blur' }],
	supplierName: [{ required: true, message: uiText('ui.p3bd864236a18'), trigger: 'blur' }],
	baseUrl: [{ required: true, message: uiText('ui.p207977a81006'), trigger: 'blur' }],
	apiKey: [{ required: true, message: uiText('ui.pd762a33862bd'), trigger: 'blur' }],
}

/**
 * @description Open modal
 */
function open() {
	show.value = true
}

/**
 * @description Close modal
 */
function close() {
	show.value = false
}

defineExpose({
	open,
	close,
})
</script>

<style scoped></style>
