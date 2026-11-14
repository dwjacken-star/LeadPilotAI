<template>
	<div class="policy-grid">
		<n-form-item v-for="field in numbers" :key="field.key" :label="field.label">
			<n-input-number v-model:value="value[field.key]" :min="0" :precision="0" :max="field.max" style="width: 100%" />
		</n-form-item>
		<n-form-item :label="t('workspaceCopy.w192')"><n-input v-model:value="value.timezone" :placeholder="$t('ui.pb485a3cf8082')" /></n-form-item>
		<n-form-item :label="t('workspaceCopy.w193')"><n-input v-model:value="value.window_start" :placeholder="t('workspaceCopy.w194')" /></n-form-item>
		<n-form-item :label="t('workspaceCopy.w195')"><n-input v-model:value="value.window_end" placeholder="18:00" /></n-form-item>
		<n-form-item :label="t('workspaceCopy.w196')"><n-select :value="asArray(value.weekdays)" @update:value="value.weekdays = asArray($event)" multiple :options="days" /></n-form-item>
	</div>
	<n-alert type="info" :show-icon="false">{{ t('workspaceCopy.w197') }}</n-alert>
</template>
<script setup lang="ts">
const {t}=useI18n()
import { asArray } from './normalize'
const value = defineModel<Record<string, any>>({ required: true })
const numbers = computed(()=>[
	{ key: 'per_minute', label: t('workspaceCopy.w381'), max: 100000 }, { key: 'per_hour', label: t('workspaceCopy.w382'), max: 1000000 },
	{ key: 'per_day', label: t('workspaceCopy.w383'), max: 10000000 }, { key: 'min_interval_ms', label: t('workspaceCopy.w198'), max: 86400000 },
	{ key: 'concurrency', label: t('workspaceCopy.w355'), max: 100 }, { key: 'recipient_cooldown_hours', label: t('workspaceCopy.w354'), max: 8760 },
	{ key: 'batch_size', label: t('workspaceCopy.w384'), max: 1000000 }, { key: 'batch_rest_seconds', label: t('workspaceCopy.w374'), max: 2592000 },
])
const days = computed(()=>[t('workspaceCopy.w357'), t('workspaceCopy.w358'), t('workspaceCopy.w359'), t('workspaceCopy.w360'), t('workspaceCopy.w361'), t('workspaceCopy.w362'), t('workspaceCopy.w363')].map((label, value) => ({ label, value })))
</script>
<style scoped>.policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 20px } @media(max-width:700px) { .policy-grid { grid-template-columns:1fr } }</style>
