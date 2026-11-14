<template>
	<section>
		<n-alert type="info">{{ t('social.manual_delivery_hint') }}</n-alert>
		<div class="post-body">{{ content.body }}</div>
		<p class="manual-link">{{ content.link }}</p>
		<n-space class="mt-16px">
			<n-button @click="perform(copyDraft)">{{ t('social.manual_copy') }}</n-button>
			<n-button
				tag="a"
				href="https://www.facebook.com/"
				target="_blank"
				rel="noopener noreferrer"
				>{{ t('social.manual_open_facebook') }}</n-button
			>
		</n-space>
		<n-space class="mt-16px">
			<n-button
				v-for="(image, index) in images"
				:key="image"
				:loading="downloading === image"
				@click="perform(() => download(image, index))">
				{{ t('social.manual_download_image', { index: index + 1 }) }}
			</n-button>
		</n-space>
		<template v-if="delivery.status === 'manual_due' && can('social:publish')">
			<n-form-item class="mt-16px" :label="t('social.manual_external_url')">
				<n-input v-model:value="externalURL" placeholder="https://www.facebook.com/…" />
			</n-form-item>
			<n-popconfirm @positive-click="perform(markPublished)">
				<template #trigger>
					<n-button
						type="primary"
						:loading="busy"
						:disabled="!!externalURL.trim() && !facebookManualURL(externalURL.trim())"
						>{{ t('social.manual_mark_published') }}</n-button
					>
				</template>
				{{ t('social.manual_confirm_warning') }}
			</n-popconfirm>
		</template>
		<n-popconfirm
			v-if="
				['scheduled', 'manual_due', 'retry', 'blocked', 'failed', 'unknown'].includes(
					delivery.status
				) && can('social:publish')
			"
			@positive-click="perform(cancel)">
			<template #trigger
				><n-button class="mt-16px" type="warning" :loading="busy">{{
					t('social.manual_cancel')
				}}</n-button></template
			>
			{{ t('social.manual_cancel_warning') }}
		</n-popconfirm>
		<n-button
			v-if="facebookManualURL(delivery.remote_url)"
			class="mt-16px"
			tag="a"
			:href="facebookManualURL(delivery.remote_url)!"
			target="_blank"
			rel="noopener noreferrer">
			{{ t('social.open_post') }}
		</n-button>
	</section>
</template>
<script setup lang="ts">
import { social, socialFile } from './api'
import { values, valueObject } from './display'
import { facebookManualURL } from './authorization'
import { can } from '@/features/workspace/api'
import { useCopy } from '@/hooks/useCopy'
const props = defineProps<{ delivery: Record<string, any> }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const { copyText } = useCopy()
const externalURL = ref(''),
	busy = ref(false),
	downloading = ref('')
const content = computed(() => {
	const snapshot = valueObject(props.delivery.snapshot)
	return Object.keys(snapshot).length ? snapshot : valueObject(props.delivery.variant)
})
const images = computed<string[]>(() =>
	values(content.value.images).filter(
		(image: unknown) =>
			typeof image === 'string' &&
			/^(task|library):(?:\d+|[0-9a-fA-F]{8}-(?:[0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12})$/.test(image)
	)
)
watch(
	() => props.delivery.id,
	() => {
		externalURL.value = ''
	}
)
async function perform(action: () => Promise<void>) {
	if (busy.value) return
	busy.value = true
	try {
		await action()
	} catch {
		/* Shared API displays errors. */
	} finally {
		busy.value = false
	}
}
async function copyDraft() {
	await copyText([content.value.body, content.value.link].filter(Boolean).join('\n\n'))
}
async function download(image: string, index: number) {
	downloading.value = image
	try {
		const [source, id] = image.split(':')
		const blob = await socialFile(id, source)
		const url = URL.createObjectURL(blob)
		const link = document.createElement('a')
		link.href = url
		link.download = `post-${props.delivery.id}-${index + 1}.${blob.type === 'image/jpeg' ? 'jpg' : 'png'}`
		link.click()
		setTimeout(() => URL.revokeObjectURL(url), 1000)
	} finally {
		downloading.value = ''
	}
}
async function markPublished() {
	const url = externalURL.value.trim()
	if (props.delivery.status !== 'manual_due' || (url && !facebookManualURL(url))) return
	await social('delivery_control', {
		id: props.delivery.id,
		data: {
			operation: 'manual_confirm',
			manual_confirmed: true,
			...(url ? { external_url: url } : {}),
		},
	})
	emit('changed')
}
async function cancel() {
	if (
		!['scheduled', 'manual_due', 'retry', 'blocked', 'failed', 'unknown'].includes(
			props.delivery.status
		) ||
		!can('social:publish')
	)
		return
	await social('delivery_control', { id: props.delivery.id, data: { operation: 'cancel' } })
	emit('changed')
}
</script>
<style scoped>
.post-body {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	margin-top: 16px;
}
.manual-link {
	overflow-wrap: anywhere;
}
</style>
