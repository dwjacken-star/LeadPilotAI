<template>
	<Container :label="$t('ui.p4f9be057f0ea')">
		<template #options>
			<div>
				<n-input v-model:value="value"></n-input>
			</div>
		</template>
	</Container>
</template>

<script lang="ts" setup>
import Container from './BaseContainer.vue'

const value = defineModel<string>('value')
</script>
