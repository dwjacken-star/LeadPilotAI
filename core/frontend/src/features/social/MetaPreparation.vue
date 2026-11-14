<template>
	<n-alert type="warning">{{ t('social.meta_notice') }}</n-alert>
	<n-form label-placement="top" class="mt-16px">
		<n-form-item :label="t('social.meta_media_url')">
			<n-input
				v-model:value="settings.media_fetch_url"
				placeholder="https://example.com/api/social/media" />
		</n-form-item>
		<n-checkbox v-model:checked="settings.media_fetch_confirmed">
			{{ t('social.meta_media_confirmed') }}
		</n-checkbox>
		<n-form-item v-for="platform in ['facebook', 'instagram', 'threads']" :key="platform" :label="platform">
			<n-space vertical>
				<n-checkbox v-model:checked="settings[platform].video_publishing">
					{{ t('social.video_enable') }}
				</n-checkbox>
				<n-checkbox v-if="platform === 'facebook'" v-model:checked="settings.facebook.test_access_confirmed">
					{{ t('social.meta_test_confirmation') }}
				</n-checkbox>
			</n-space>
		</n-form-item>
	</n-form>
	<n-alert type="info" class="mt-16px">{{ t('social.meta_video_blocked') }}</n-alert>
	<n-button class="mt-16px" :loading="loading" @click="load">{{
		t('social.meta_preconditions')
	}}</n-button>
	<template v-if="readiness">
		<p>{{ t('social.meta_blockers') }}</p>
		<pre class="meta-readiness">{{ JSON.stringify(readiness, null, 2) }}</pre>
	</template>
</template>
<script setup lang="ts">
import { social } from './api'
defineProps<{ settings: Record<string, any> }>()
const { t } = useI18n()
const loading = ref(false)
const readiness = ref<Record<string, any> | null>(null)
async function load() {
	loading.value = true
	try {
		readiness.value = await social('meta_readiness')
	} catch {
		/* Shared API displays errors. */
	} finally {
		loading.value = false
	}
}
onMounted(load)
</script>
<style scoped>
.meta-readiness {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	font-size: 12px;
}
</style>
