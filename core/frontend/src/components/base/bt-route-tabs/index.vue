<template>
	<div class="bt-title mb-8px!">{{ title }}</div>
	<n-tabs :value="activeTab" type="line" class="route-tabs" @update:value="handleUpdateTab">
		<n-tab-pane v-for="tab in tabs" :key="tab.name" :name="tab.name" :tab="tab.titleKey ? t(tab.titleKey) : tab.title"> </n-tab-pane>
	</n-tabs>
	<router-view v-slot="{ Component, route: childRoute }"
		><keep-alive><component :is="Component" :key="childRoute.name" /></keep-alive
	></router-view>
</template>

<script lang="ts" setup>
import { get } from 'lodash-es'
import { RouteLocationNormalized } from 'vue-router'

interface TabItem {
	title: string
 titleKey?:string
	name: string
}

const { t } = useI18n()

const route = useRoute()

const router = useRouter()

const tabs = ref<TabItem[]>([])

const activeTab = computed(() => {
	return String(route.name)
})

const title = computed(() => {
	const record = route.matched.find(r => r.meta?.key)
	return record?.meta?.titleKey
		? t(String(record.meta.titleKey))
		: String(record?.meta?.title || '')
})

const handleUpdateTab = (name: string) => {
	router.push({ name })
}

// Add tab when route changes
const addTab = (route: RouteLocationNormalized) => {
	const existTab = tabs.value.find(tab => tab.name === route.name)
	if (existTab) return

	const section = route.matched.find(
		record => record.name && !record.meta?.key && record.children?.length
	)
	if (section?.children) {
		tabs.value = section.children
			.filter(routeItem => {
				const isHidden = get(routeItem, 'meta.hidden', false)
				return !isHidden
			})
			.map(routeItem => {
				const key = get(routeItem, 'meta.titleKey', '')
				const title = get(routeItem, 'meta.title', '')
				return {
					title: String(title),
 titleKey:String(key),
					name: String(routeItem.name),
				}
			})
	}
}

addTab(route)
</script>

<style lang="scss" scoped>
.route-tabs {
	--n-tab-gap: 32px;
	--n-tab-padding: 12px 4px;
	--n-pane-padding-top: 16px;
	--n-tab-border-color: var(--color-border-1);
}
</style>
