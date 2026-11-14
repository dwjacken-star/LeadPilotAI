import { isNumber } from 'lodash-es'
import { isArray, isObject } from '@/utils'

interface TableParams {
	page: number
	page_size: number
}

interface UseTableDataOptions<K> {
	params: K
	fetchFn?: (params: K) => Promise<unknown>
	loading?: boolean
	immediate?: boolean
}

export const useTableData = <T, K extends TableParams>(options: UseTableDataOptions<K>) => {
	const { fetchFn, params, loading = false, immediate = false } = options

	const loadingRef = ref(loading)

	const tableList = ref<T[]>([])

	const tableTotal = ref(0)

	const tableParams = ref<K>(params)
	let serial=0

	// Get table data
	const getTableData = async (resetPage = false, silent = false) => {
		if (resetPage) {
			params.page = 1
		}

		if (fetchFn) {
			const request=++serial
			const query={...tableParams.value}
			if(!silent) loadingRef.value = true
			try {
				const res = await fetchFn(query)
				if(request!==serial)return
				if (isObject<{ list: T[]; total: number }>(res)) {
					tableList.value = isArray(res.list) ? res.list : []
					tableTotal.value = isNumber(res.total) ? res.total : 0
				}
			} finally {
				if(request===serial) loadingRef.value = false
			}
		}
	}
	onBeforeUnmount(()=>{serial++})

	// If immediate is set, get data immediately
	if (immediate) {
		getTableData()
	}

	return {
		loading: loadingRef,
		tableList,
		tableTotal,
		tableParams,
		getTableData,
	}
}
