<template>
	<div class="variant-navigation">
		<n-button :disabled="!left" :aria-label="t('social.scroll_left')" @click="scroll(-1)"
			>‹</n-button
		>
		<div ref="strip" class="variant-strip" role="tablist" @scroll="measure">
			<button
				v-for="item in options"
				:key="item.value"
				:ref="element => remember(item.value, element)"
				type="button"
				role="tab"
				:aria-selected="model === item.value"
				:class="{ selected: model === item.value }"
				@click="model = item.value">
				{{ item.label }}
			</button>
		</div>
		<n-button :disabled="!right" :aria-label="t('social.scroll_right')" @click="scroll(1)"
			>›</n-button
		>
	</div>
</template>
<script setup lang="ts">
const props = defineProps<{ options: { value: number; label: string }[] }>()
const model = defineModel<number | null>()
const { t } = useI18n(),
	strip = ref<HTMLElement>(),
	left = ref(false),
	right = ref(false)
const buttons = new Map<number, HTMLElement>()
let observer: ResizeObserver | undefined
function remember(id: number, element: any) {
	if (element) buttons.set(id, element)
	else buttons.delete(id)
}
function measure() {
	const el = strip.value
	if (el) {
		left.value = el.scrollLeft > 1
		right.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 1
	}
}
function scroll(direction: number) {
	const el = strip.value
	el?.scrollBy({ left: direction * Math.max(160, el.clientWidth * 0.7), behavior: 'smooth' })
}
watch(
	model,
	async () => {
		await nextTick()
		buttons.get(Number(model.value))?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
		measure()
	}
)
watch(() => props.options, async () => { await nextTick(); measure() }, { deep: true })
onMounted(() => {
	observer = new ResizeObserver(measure)
	if (strip.value) observer.observe(strip.value)
	measure()
})
onBeforeUnmount(() => observer?.disconnect())
</script>
<style scoped>
.variant-navigation {
	display: flex;
	align-items: center;
	gap: 10px;
	min-width: 0;
	margin: 16px 0;
}
.variant-strip {
	display: flex;
	gap: 8px;
	flex: 1;
	min-width: 0;
	overflow-x: auto;
	scrollbar-width: none;
}
.variant-strip::-webkit-scrollbar {
	display: none;
}
.variant-strip button {
	flex-shrink: 0;
	border: 1px solid #8884;
	padding: 10px 16px;
	border-radius: 6px;
	background: transparent;
	color: inherit;
	cursor: pointer;
	white-space: nowrap;
}
.variant-strip button.selected {
	border-color: var(--primary-color, #187151);
	color: var(--primary-color, #187151);
	background: #18815112;
}
</style>
