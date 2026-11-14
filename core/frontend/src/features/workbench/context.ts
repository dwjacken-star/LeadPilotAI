import {
	inject,
	onActivated,
	onDeactivated,
	onBeforeUnmount,
	provide,
	ref,
	watch,
	type InjectionKey,
	type Ref,
} from 'vue'
export const activityKey: InjectionKey<Ref<boolean>> = Symbol('workbench.activity')
export const dirtyKey: InjectionKey<(dirty: boolean) => void> = Symbol('workbench.dirty')
export function useWorkbenchActivity() {
	return inject(activityKey, ref(true))
}
export function useWorkbenchDirty(source?: () => unknown) {
	const set = inject(dirtyKey, () => {})
	let baseline = source ? JSON.stringify(source()) : ''
	function clean() {
		if (source) baseline = JSON.stringify(source())
		set(false)
	}
	if (source) watch(source, () => set(JSON.stringify(source()) !== baseline), { deep: true })
	return { markDirty: () => set(true), markClean: clean }
}
export function provideWorkbenchActivity() {
	const active = ref(true)
	provide(activityKey, active)
	onActivated(() => (active.value = true))
	onDeactivated(() => (active.value = false))
	onBeforeUnmount(() => (active.value = false))
	return active
}

export const paneKey: InjectionKey<string>=Symbol('workbench.pane')
