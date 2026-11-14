<template>
	<n-layout class="leadpilot-shell w-full h-full" has-sider>
		<sidebar />
		<section class="workspace-shell"><app-header :top="0" /><workspace-tabs /><app-main /></section>
	</n-layout>
</template>
<script setup lang="ts">
import { Sidebar, AppHeader, AppMain } from './components'
import WorkspaceTabs from './components/WorkspaceTabs.vue'
</script>
