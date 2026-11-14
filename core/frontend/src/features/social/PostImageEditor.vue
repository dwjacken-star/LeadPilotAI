<template>
	<section class="post-image-editor">
		<n-select
			:value="images"
			multiple
			filterable
			:disabled="!editable || uploading"
			:options="allOptions"
			@update:value="selectImages" />
		<n-space v-if="editable" class="image-tools">
			<n-button
				:loading="uploading"
				:disabled="images.length >= MAX_POST_IMAGES"
				@click="input?.click()"
				>{{ t('social.upload_post_images') }}</n-button
			>
			<input
				ref="input"
				type="file"
				multiple
				hidden
				accept="image/jpeg,image/png,.jpg,.jpeg,.png"
				@change="upload" />
			<span class="hint">{{ t('social.post_images_hint') }}</span>
		</n-space>
		<div class="selected-images">
			<article v-for="(image, index) in images" :key="image">
				<n-image
					v-if="urls[image]"
					:src="urls[image]"
					:alt="label(image)"
					object-fit="contain"
					:height="110" />
				<span>{{ index + 1 }}. {{ label(image) }}</span>
				<n-space v-if="editable" size="small">
					<n-button
						size="tiny"
						:disabled="index === 0"
						@click="images = movePostImage(images, index, -1)"
						>{{ t('social.image_up') }}</n-button
					>
					<n-button
						size="tiny"
						:disabled="index === images.length - 1"
						@click="images = movePostImage(images, index, 1)"
						>{{ t('social.image_down') }}</n-button
					>
					<n-button
						size="tiny"
						type="error"
						@click="images = images.filter(value => value !== image)"
						>{{ t('common.actions.delete') }}</n-button
					>
				</n-space>
			</article>
		</div>
		<n-collapse v-if="editable" class="ai-image-settings" @item-header-click="loadModels">
			<n-collapse-item :title="t('social.ai_image_title')" name="ai">
				<p class="hint">{{ t('social.ai_image_hint') }}</p>
				<AIModelSelect
					ref="modelSelect"
					kind="image"
					v-model:value="model"
					:options="models"
					:loading="modelsLoading"
					:placeholder="t('social.image_model')" />
				<n-select v-model:value="size" class="image-tools" :options="sizeOptions" />
				<n-input
					v-model:value="prompt"
					type="textarea"
					:rows="3"
					maxlength="3000"
					show-count
					:placeholder="t('social.image_prompt')" />
				<n-space class="image-tools"
					><n-button
						type="primary"
						:loading="processing"
						:disabled="!model || !canProcessPostImages(images, prompt) || uploading || hasActive"
						@click="processImages"
						>{{
							t(images.length ? 'social.process_post_images' : 'social.generate_post_images')
						}}</n-button
					><n-button :loading="modelsLoading" @click="loadModels">{{
						t('social.refresh_models')
					}}</n-button></n-space
				>
				<n-alert v-if="modelWarning" type="info">{{ modelWarning }}</n-alert>
			</n-collapse-item>
		</n-collapse>
		<n-space v-if="jobs.length" class="image-tools"
			><n-tag
				v-for="job in jobs.slice(0, 10)"
				:key="job.id"
				:type="job.status === 'failed' ? 'error' : job.status === 'completed' ? 'success' : 'info'"
				>#{{ job.id }} {{ stateText(job.status)
				}}<span v-if="job.error"> · {{ job.error }}</span></n-tag
			></n-space
		>
		<template v-if="candidates.length">
			<h4>{{ t('social.image_candidates') }}</h4>
			<p class="hint">{{ t('social.image_candidates_hint') }}</p>
			<div class="selected-images">
				<article v-for="candidate in candidates.slice(0, candidateLimit)" :key="candidate.id">
					<n-image
						v-if="urls['task:' + candidate.id]"
						:src="urls['task:' + candidate.id]"
						:alt="candidate.filename"
						object-fit="contain"
						:height="130" />
					<span>{{ candidate.filename }}</span>
					<span v-if="valueObject(candidate.extracted).source_ref" class="hint"
						>{{ t('social.image_original') }}:
						{{ label(valueObject(candidate.extracted).source_ref) }}</span
					>
					<n-button
						v-if="editable"
						size="small"
						:disabled="
							images.includes('task:' + candidate.id) ||
							(images.length >= MAX_POST_IMAGES &&
								!images.includes(valueObject(candidate.extracted).source_ref))
						"
						@click="useCandidate(candidate)"
						>{{ t('social.use_image_result') }}</n-button
					>
				</article>
			</div>
			<n-button v-if="candidates.length > candidateLimit" size="small" @click="moreCandidates">{{
				t('social.load_more')
			}}</n-button>
		</template>
	</section>
</template>
<script setup lang="ts">
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { social, socialFile, uploadPostImage } from './api'
import {
	MAX_POST_IMAGES,
	movePostImage,
	selectPostCandidate,
	canProcessPostImages,
} from './composer'
import { valueObject } from './display'
import { Message } from '@/utils'
const props = defineProps<{
	variantId: number
	options: { value: string; label: string }[]
	editable: boolean
}>()
const images = defineModel<string[]>('images', { default: () => [] })
const emit = defineEmits<{ uploaded: [] }>()
const { t, te } = useI18n(),
	input = ref<HTMLInputElement>(),
	uploading = ref(false),
	processing = ref(false),
	modelsLoading = ref(false)
const models = ref<{ value: string; label: string }[]>([]),
	model = ref<string | null>(null),
	modelWarning = ref(''),
	size = ref('1024x1024'),
	prompt = ref('')
const candidates = ref<any[]>([]),
	jobs = ref<any[]>([]),
	urls = ref<Record<string, string>>({}),
	uploadedLabels = ref<Record<string, string>>({})
const candidateLimit = ref(10)
const hasActive = computed(() => jobs.value.some(j => ['queued', 'running'].includes(j.status)))
const allOptions = computed(() => [
	...props.options,
	...Object.entries(uploadedLabels.value)
		.filter(([value]) => !props.options.some(o => o.value === value))
		.map(([value, label]) => ({ value, label })),
])
const sizeOptions = computed(() =>
	['1024x1024', '1536x1024', '1024x1536'].map(value => ({
		value,
		label: t('social.image_sizes.' + value),
	}))
)
const stateText = (state: string) =>
	te('social.states.' + state) ? t('social.states.' + state) : state
const label = (image: string) =>
	props.options.find(o => o.value === image)?.label || uploadedLabels.value[image] || image
let alive = true,
	serial = 0,
	loadingResults = false
const previewPending = new Set<string>()
const modelSelect = ref<InstanceType<typeof AIModelSelect> | null>(null)
async function loadModels() {
	if (modelsLoading.value) return
	modelsLoading.value = true
	try {
		const [r] = await Promise.all([social('image_models'), modelSelect.value?.reload()])
		if (!alive) return
		models.value = r.models || []
		modelWarning.value =
			r.warning || (!r.enabled || !models.value.length ? t('social.image_ai_unavailable') : '')
		if (!models.value.some(o => o.value === model.value))
			model.value = models.value.find(o => o.value === r.model)?.value || models.value[0]?.value || null
	} catch {
		/* Shared API shows errors. */
	} finally {
		modelsLoading.value = false
	}
}
async function loadResults() {
	if (loadingResults || !props.variantId) return
	loadingResults = true
	const current = ++serial
	try {
		const r = await social('post_images', { id: props.variantId })
		if (!alive || current !== serial) return
		candidates.value = r.candidates || []
		jobs.value = r.jobs || []
		for (const c of r.images || []) uploadedLabels.value['task:' + c.id] = c.filename
		void loadPreviews()
	} catch {
		/* Shared API shows errors. */
	} finally {
		loadingResults = false
	}
}
async function loadPreviews() {
	for (const image of [
		...images.value,
		...candidates.value.slice(0, candidateLimit.value).map(c => 'task:' + c.id),
	]) {
		if (!alive) return
		if (urls.value[image] || previewPending.has(image)) continue
		previewPending.add(image)
		const [source, id] = image.split(':')
		try {
			const blob = await socialFile(id, source)
			if (alive) urls.value[image] = URL.createObjectURL(blob)
		} catch {
			/* Filename remains visible. */
		} finally {
			previewPending.delete(image)
		}
	}
}
function selectImages(values: string[]) {
	if (values.length > MAX_POST_IMAGES) {
		Message.warning(t('social.image_limit'))
		return
	}
	images.value = values
}
function moreCandidates() {
	candidateLimit.value += 10
	void loadPreviews()
}
async function upload(event: Event) {
	const field = event.target as HTMLInputElement,
		files = [...(field.files || [])]
	field.value = ''
	if (!files.length) return
	if (files.length + images.value.length > MAX_POST_IMAGES) {
		Message.warning(t('social.image_limit'))
		return
	}
	if (files.some(f => f.size > 8 * 1024 * 1024 || !['image/jpeg', 'image/png'].includes(f.type))) {
		Message.warning(t('social.post_images_hint'))
		return
	}
	uploading.value = true
	try {
		for (const file of files) {
			const r = await uploadPostImage(file, props.variantId)
			if (!alive) return
			uploadedLabels.value[r.ref] = r.filename
			if (!images.value.includes(r.ref)) images.value = [...images.value, r.ref]
		}
	} catch {
		/* Successfully uploaded images remain selected. */
	} finally {
		uploading.value = false
		if (alive) {
			emit('uploaded')
			void loadPreviews()
		}
	}
}
async function processImages() {
	if (!model.value || !canProcessPostImages(images.value, prompt.value) || hasActive.value) return
	processing.value = true
	try {
		await social('process_images', {
			id: props.variantId,
			data: { images: images.value, model: model.value, prompt: prompt.value, size: size.value },
		})
		await loadResults()
		Message.success(t('social.image_processing_queued'))
	} catch {
		/* Shared API shows errors. */
	} finally {
		processing.value = false
	}
}
function useCandidate(candidate: any) {
	const info = valueObject(candidate.extracted)
	images.value = selectPostCandidate(images.value, info.source_ref, 'task:' + candidate.id)
	emit('uploaded')
}
watch(
	() => images.value,
	() => void loadPreviews(),
	{ deep: true }
)
onMounted(() => {
	void loadResults()
	void loadPreviews()
})
const poll = setInterval(() => {
	if (!workbenchActive.value) return
	if (alive && hasActive.value) void loadResults()
}, 3000)
onBeforeUnmount(() => {
	alive = false
	serial++
	clearInterval(poll)
	Object.values(urls.value).forEach(url => URL.revokeObjectURL(url))
})
</script>
<style scoped>
.post-image-editor {
	width: 100%;
	min-width: 0;
}
.image-tools {
	margin: 12px 0;
}
.hint {
	font-size: 12px;
	line-height: 1.6;
	color: var(--color-text-3);
}
.selected-images {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
	gap: 12px;
	margin: 12px 0;
}
.selected-images article {
	display: flex;
	flex-direction: column;
	gap: 8px;
	padding: 10px;
	border: 1px solid #83a49240;
	border-radius: 8px;
	min-width: 0;
}
.selected-images article > span {
	overflow-wrap: anywhere;
	font-size: 12px;
}
.ai-image-settings {
	margin: 16px 0;
}
h4 {
	margin: 16px 0 8px;
}
</style>
