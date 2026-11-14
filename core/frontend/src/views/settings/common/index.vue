<template>
	<div class="max-w-4xl mx-auto px-4 py-8">
		<SecuritySettings />
		<NetworkSettings />
		<OutboundIdentity />
		<UnsubscribeProtection />
		<SystemSettings />
		<NotifySettings />
	</div>
</template>

<script lang="ts" setup>
import { useSettingsStore } from '@/views/settings/common/store'

import SecuritySettings from './components/SecuritySettings.vue'
import OutboundIdentity from './components/OutboundIdentity.vue'
import UnsubscribeProtection from './components/UnsubscribeProtection.vue'
import NetworkSettings from './components/NetworkSettings.vue'
import SystemSettings from './components/SystemSettings.vue'
import NotifySettings from './components/Notify/NotifySettings.vue'

const settingsStore = useSettingsStore()

settingsStore.getCommonConfig()

onBeforeUnmount(() => {
	settingsStore.reset()
})
</script>
