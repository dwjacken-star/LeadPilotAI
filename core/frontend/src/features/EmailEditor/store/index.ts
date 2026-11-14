import { defineStore, getActivePinia } from 'pinia'
import { inject, getCurrentInstance, ref } from 'vue'
import { paneKey } from '@/features/workbench/context'
import {
	ColumnMap,
	ColumnConfigMap,
	CellMap,
	CellConfigMap,
	BlockMap,
	PageConfig,
	BlockConfigMap,
} from '../types/base'

const editorSetup = () => {
	const version = ref('1.0')

	// 页面配置
	const pageConfig = ref<PageConfig>({
		meta: {
			version: version.value,
			createdAt: '',
			updatedAt: '',
		},
		style: {
			backgroundColor: '#ffffff',
			width: '500px',
			fontFamily: 'PingFang SC, Microsoft YaHei',
		},
	})

	// columnsSource数据表
	const columnsSource = ref<string[]>([])

	// columnsMap数据表
	const columnsMap = ref<ColumnMap>({})

	// columnsConfigMap数据表
	const columnsConfigMap = ref<ColumnConfigMap>({})

	// cellMap数据表
	const cellMap = ref<CellMap>({})

	// cellConfigMap数据表
	const cellConfigMap = ref<CellConfigMap>({})

	// blockMap数据表
	const blockMap = ref<BlockMap>({})

	// blockMap数据表
	const blockConfigMap = ref<BlockConfigMap>({})

	// 当前选中的块
	const selectedBlockKey = ref('')

	// 当前选中的块类型
	const selectedBlockType = ref('')

	// 自动保存方法
	const saveFn = ref<() => unknown>(async () => false)

	return {
		version,
		pageConfig,
		columnsSource,
		columnsMap,
		columnsConfigMap,
		cellMap,
		cellConfigMap,
		blockMap,
		blockConfigMap,
		selectedBlockKey,
		selectedBlockType,
		saveFn,
	}
}
const definitions = new Map<string, ReturnType<typeof makeEditorDefinition>>()
export function useEmailEditorStore() {
	const scope = getCurrentInstance() ? inject(paneKey, 'default') : 'default'
	if (!definitions.has(scope))
		definitions.set(scope, defineStore('email-editor:' + scope, editorSetup))
	return definitions.get(scope)!()
}
function makeEditorDefinition() {
	return defineStore('typed-editor', editorSetup)
}
export function disposeEmailEditor(scope: string) {
	const pinia = getActivePinia(),
		id = 'email-editor:' + scope
	const store = (pinia as any)?._s.get(id)
	store?.$dispose()
	if (pinia) delete pinia.state.value[id]
	definitions.delete(scope)
}
