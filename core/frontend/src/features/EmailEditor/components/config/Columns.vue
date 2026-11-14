<template>
	<Collapse :default-expanded-names="['1', '2', '3']">
		<n-collapse-item :title="$t('ui.pcf723c59b62a')" name="1">
			<ColumnsPanel />
		</n-collapse-item>
		<n-collapse-item :title="$t('ui.p502f6a46d48e')" name="2">
			<ColumnPropertiesPanel />
		</n-collapse-item>
		<n-collapse-item :title="$t('ui.p56de5b98b536')" name="3">
			<RowPropertiesPanel />
		</n-collapse-item>
	</Collapse>
</template>

<script lang="ts" setup>
import Collapse from '../shared/Collapse.vue'
import ColumnsPanel from './columns/ColumnsPanel.vue'
import ColumnPropertiesPanel from './columns/ColumnPropertiesPanel.vue'
import RowPropertiesPanel from './columns/RowPropertiesPanel.vue'
</script>
