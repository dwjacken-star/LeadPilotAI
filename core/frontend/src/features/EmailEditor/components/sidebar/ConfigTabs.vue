<template>
	<n-tabs v-model:value="activeTab" type="line">
		<n-tab-pane v-for="tab in tabs" :key="tab.name" :name="tab.name" :tab="tab.label">
			<component :is="tab.component" />
		</n-tab-pane>
	</n-tabs>
</template>

<script lang="ts" setup>
import {uiText} from '@/i18n/labels'
import Content from './Content.vue'
import Page from './Page.vue'

const activeTab = ref('config')

const tabs = [
	{ name: 'config', label: uiText('ui.p4f9be057f0ea'), component: Content },
	{ name: 'page', label: uiText('ui.pfb06270f7c21'), component: Page },
]
</script>

<style lang="scss" scoped>
.n-tabs {
	--n-tab-padding: 16px 12px 14px;
	--n-tab-gap: 0;
	--n-pane-padding-top: 0;
	--n-tab-font-size: 14px;
	height: 100%;
	:deep(> .n-tabs-nav .n-tabs-wrapper) {
		width: 100%;
		.n-tabs-tab-wrapper {
			flex: 1;
		}
		.n-tabs-tab {
			width: 100%;
			justify-content: center;
		}
	}
}

.n-tab-pane {
	flex: 1;
	overflow: auto;
}
</style>
