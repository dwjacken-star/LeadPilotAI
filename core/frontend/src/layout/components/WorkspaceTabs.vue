<template>
	<nav class="workspace-tabs" :aria-label="t('workbench.tabs')">
		<button class="workspace-icon" :aria-label="t('workbench.scroll_left')" @click="scroll(-280)">
			<i class="i-mdi-chevron-left" />
		</button>
		<div ref="strip" class="workspace-tab-strip" role="tablist" @wheel.prevent="wheel">
			<div
				v-for="tab in store.tabs"
				:key="tab.key"
				class="workspace-tab"
				:class="{ active: store.active === tab.key }"
				:data-tab-key="tab.key">
				<button
					role="tab"
					:aria-selected="store.active === tab.key"
					@click="navigate(tab)"
					@keydown.delete.prevent="close(tab)">
					<i v-if="tab.pinned" class="i-mdi-view-dashboard-outline" />{{ title(tab)
					}}<span
						v-if="store.dirty[tab.key]"
						class="workspace-dirty"
						:title="t('workbench.unsaved')"
						>•</span
					>
				</button>
				<button
					v-if="!tab.pinned"
					class="workspace-tab-close"
					:aria-label="t('workbench.close') + ' ' + title(tab)"
					@click="close(tab)">
					×
				</button>
			</div>
		</div>
		<button class="workspace-icon" :aria-label="t('workbench.scroll_right')" @click="scroll(280)">
			<i class="i-mdi-chevron-right" />
		</button>
		<n-dropdown trigger="click" :options="options" @select="action"
			><button class="workspace-icon" :aria-label="t('workbench.manage')">
				<i class="i-mdi-dots-horizontal" /></button
		></n-dropdown>
	</nav>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import useWorkbenchStore from '@/store/modules/workbench'
import type { WorkTab } from '@/features/workbench/model'
const { t, te } = useI18n(),
	router = useRouter(),
	store = useWorkbenchStore(),
	strip = ref<HTMLElement>()
const title = (tab: WorkTab) =>
	(tab.titleKey && te(tab.titleKey) ? t(tab.titleKey) : tab.title) +
	(tab.detail ? ' · ' + tab.detail : '')
function scroll(left: number) {
	strip.value?.scrollBy({ left, behavior: 'smooth' })
}
function wheel(event: WheelEvent) {
	scroll(event.deltaX || event.deltaY)
}
function navigate(tab: WorkTab) {
	void router.push(tab.fullPath)
}
function canDiscard(tab: WorkTab) {
	if (store.busy[tab.key]) {
		window.alert(t('workbench.in_progress'))
		return false
	}
	return !store.dirty[tab.key] || window.confirm(t('workbench.confirm_discard'))
}
async function close(tab: WorkTab) {
	if (tab.pinned || !canDiscard(tab)) return
	if (store.active === tab.key) {
		const index = store.tabs.findIndex(x => x.key === tab.key),
			next = store.tabs[index - 1] || store.tabs[index + 1]
		if (next) {
			const failure = await router.push(next.fullPath)
			if (failure) return
		}
	}
	store.remove(tab.key)
}
const options = computed(() => [
	...store.tabs.map(tab => ({ key: 'open:' + tab.key, label: title(tab) })),
	{ type: 'divider', key: 'divider' },
	...['refresh', 'close_others', 'close_right', 'close_all'].map(key => ({
		key,
		label: t('workbench.' + key),
	})),
])
async function action(key: string) {
	const current = store.tabs.find(tab => tab.key === store.active)
	if (!current) return
	if (key.startsWith('open:')) {
		const tab = store.tabs.find(tab => tab.key === key.slice(5))
		if (tab) navigate(tab)
		return
	}
	if (key === 'refresh') {
		if (canDiscard(current)) store.refresh(current.key)
		return
	}
	const right = new Set(store.tabs.slice(store.tabs.indexOf(current) + 1).map(t => t.key))
	for (const tab of [...store.tabs])
		if (
			!tab.pinned &&
			(key === 'close_all' ||
				(key === 'close_others' && tab.key !== current.key) ||
				(key === 'close_right' && right.has(tab.key)))
		)
			await close(tab)
}
watch(
	() => store.active,
	async () => {
		await nextTick()
		strip.value?.querySelectorAll<HTMLElement>('[data-tab-key]').forEach(node => {
			if (node.dataset.tabKey === store.active)
				node.scrollIntoView({ block: 'nearest', inline: 'nearest' })
		})
	}
)
function beforeUnload(event: BeforeUnloadEvent) {
	if (Object.values(store.dirty).some(Boolean) || Object.values(store.busy).some(Boolean)) {
		event.preventDefault()
		event.returnValue = ''
	}
}
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>
