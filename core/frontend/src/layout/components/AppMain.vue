<template>
	<main class="workspace-content">
		<keep-alive v-for="tab in store.tabs" :key="store.identity + ':' + tab.key + ':' + tab.revision"
			><workspace-pane v-if="store.active === tab.key" :tab-key="tab.key" :full-path="tab.fullPath"
		/></keep-alive>
	</main>
</template>

<script lang="ts" setup>
import WorkspacePane from './WorkspacePane.vue'
import useWorkbenchStore from '@/store/modules/workbench'
const store = useWorkbenchStore()
</script>
