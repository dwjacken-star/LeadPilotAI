<template>
	<container :label="$t('ui.p35c80f964568')">
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
