<template>
	<container :label="$t('ui.p6bfd0efa57e2')">
		<template #options>
			<n-input v-model:value="value"></n-input>
		</template>
	</container>
</template>

<script lang="ts" setup>
import Container from './BaseContainer.vue'

const value = defineModel<string>('value')
</script>

<style lang="scss" scoped></style>
