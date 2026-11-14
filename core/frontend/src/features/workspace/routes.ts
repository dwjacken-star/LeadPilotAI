import { defineAsyncComponent, defineComponent, h } from 'vue'
import type { Component } from 'vue'
import { useRoute } from 'vue-router'
import { principal } from './api'
import { sectionEntry } from './sections'
import type { WorkspaceSection } from './sections'

export function scopedBusinessView(
	section: WorkspaceSection,
	legacy: () => Promise<{ default: Component }>
) {
	const legacyView = defineAsyncComponent(legacy)
	const scopedView = defineAsyncComponent(() => import('@/views/workspace/index.vue'))
	return defineComponent({
		name: 'ScopedBusinessView',
		setup() {
			const route = useRoute()
			return () =>
				principal.value?.super
					? h(legacyView)
					: h(scopedView, {
							key: route.path,
							section,
							initialTab: sectionEntry(section, route.path),
						})
		},
	})
}
