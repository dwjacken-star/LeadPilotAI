<template>
	<section ref="pane" class="workspace-pane" @input.capture="changed" @change.capture="changed">
		<router-view :route="localRoute" />
	</section>
</template>
<script setup lang="ts">
import { routeLocationKey, useRouter } from 'vue-router'
import {
	onBeforeUnmount,
	onActivated,
	onDeactivated,
	provide,
	ref,
	shallowReactive,
	watch,
} from 'vue'
import { disposeEmailEditor } from '@/features/EmailEditor/store'
import useWorkbenchStore from '@/store/modules/workbench'
import { provideWorkbenchActivity, dirtyKey, paneKey } from '@/features/workbench/context'
const props = defineProps<{ tabKey: string; fullPath: string }>()
const router = useRouter(),
	store = useWorkbenchStore(),
	pane = ref<HTMLElement>()
const localRoute = shallowReactive({ ...router.resolve(props.fullPath) })
watch(
	() => props.fullPath,
	path => Object.assign(localRoute, router.resolve(path))
)
provide(routeLocationKey, localRoute as any)
provideWorkbenchActivity()
const editorScope = store.identity + ':' + props.tabKey
provide(paneKey, editorScope)
provide(dirtyKey, (dirty: boolean) => {
	store.dirty[props.tabKey] = dirty
})
onBeforeUnmount(() => {
	document.removeEventListener('input', outsideChange, true)
	document.removeEventListener('change', outsideChange, true)
	disposeEmailEditor(editorScope)
})
let scrollTop = 0
function changed(event: Event) {
	const target = event.target as HTMLElement
	if (target.closest('form,.n-form,[contenteditable="true"]')) store.dirty[props.tabKey] = true
}
function outsideChange(event: Event) {
	const target = event.target as HTMLElement
	if (target.closest('form,.n-form,[contenteditable="true"]')) store.dirty[props.tabKey] = true
}
onDeactivated(() => {
	document.removeEventListener('input', outsideChange, true)
	document.removeEventListener('change', outsideChange, true)
	scrollTop = pane.value?.scrollTop || 0
})
onActivated(() => {
	document.addEventListener('input', outsideChange, true)
	document.addEventListener('change', outsideChange, true)
	if (pane.value) pane.value.scrollTop = scrollTop
})
</script>
