import { defineStore } from 'pinia'
import { ref } from 'vue'
import { tabFromRoute, upsertTab, restoreTabs, type WorkTab } from '@/features/workbench/model'
export default defineStore('Workbench', () => {
	const identity = ref('')
	const tabs = ref<WorkTab[]>([]),
		active = ref(''),
		dirty = ref<Record<string, boolean>>({}),
		busy = ref<Record<string, boolean>>({})
	let storageKey = ''
	function persist() {
		try {
			if (storageKey)
				sessionStorage.setItem(
					storageKey,
					JSON.stringify(tabs.value.map(({ fullPath }) => ({ fullPath })))
				)
		} catch {
			/* Storage is optional. */
		}
	}
	function initialize(
		id: number,
		resolve: (path: string) => any,
		allowed: (path: string) => boolean
	) {
		const next = 'leadpilot.workbench.' + id
		if (storageKey !== next) {
			tabs.value = []
			dirty.value = {}
			busy.value = {}
			storageKey = next
			identity.value = next
			try {
				tabs.value = restoreTabs(sessionStorage.getItem(next), resolve, allowed)
			} catch {
				/* Storage is optional. */
			}
		}
		tabs.value = tabs.value.filter(t => allowed(t.fullPath.split('?')[0]))
		const home = tabFromRoute(resolve('/workspace'))
		if (home) tabs.value = [home, ...tabs.value.filter(tab => tab.key !== home.key)]
		for (const key of Object.keys(dirty.value))
			if (!tabs.value.some(tab => tab.key === key)) {
				delete dirty.value[key]
				delete busy.value[key]
			}
		persist()
	}
	function visit(route: any) {
		const tab = tabFromRoute(route)
		if (!tab) return
		tabs.value = upsertTab(tabs.value, tab)
		active.value = tab.key
		persist()
	}
	function remove(key: string) {
		tabs.value = tabs.value.filter(t => t.key !== key)
		delete dirty.value[key]
		delete busy.value[key]
		persist()
	}
	function reset() {
		try {
			if (storageKey) sessionStorage.removeItem(storageKey)
		} catch {
			/* Storage is optional. */
		}
		tabs.value = []
		active.value = ''
		dirty.value = {}
		busy.value = {}
		storageKey = ''
	}
	function refresh(key: string) {
		const tab = tabs.value.find(t => t.key === key)
		if (tab) tab.revision++
		delete dirty.value[key]
		delete busy.value[key]
	}
	return { identity, tabs, active, dirty, busy, initialize, visit, remove, reset, refresh }
})
