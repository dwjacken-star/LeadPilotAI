export interface WorkTab {
	key: string
	fullPath: string
	titleKey: string
	title: string
	detail: string
	pinned: boolean
	revision: number
}
const safeQuery = new Set(['task_id', 'type', 'chat_id', 'page', 'tab', 'group_id'])
export function tabFromRoute(route: any): WorkTab | null {
	if (!route?.name || route.path === '/login' || !route.meta?.key) return null
	const query = new URLSearchParams()
	for (const key of Object.keys(route.query || {}).sort()) {
		const value = route.query[key]
		if (key === 'oauth' && (value === 'success' || value === 'failed')) query.set(key,value)
		if (safeQuery.has(key) && typeof value === 'string' && value.length < 180) query.set(key, value)
	}
	const fullPath = route.path + (query.size ? '?' + query.toString() : '')
	const records = route.matched || [],
		leaf = records[records.length - 1] || {}
	const specific =
		Object.keys(route.params || {}).length > 0 || /\/(edit|analytics)(\/|$)/.test(route.path)
	return {
		key: specific ? String(route.name) + ':' + fullPath : String(route.meta.key),
		fullPath,
		titleKey: String(
			(specific ? leaf.meta?.titleKey : '') ||
				records.find((r: any) => r.meta?.key)?.meta?.titleKey ||
				route.meta.titleKey ||
				''
		),
		title: String(leaf.meta?.title || route.meta.title || route.name),
		detail: String(route.query?.task_id || Object.values(route.params || {}).join(' / ') || ''),
		pinned: route.path === '/workspace',
		revision: 0,
	}
}
export function upsertTab(tabs: WorkTab[], incoming: WorkTab): WorkTab[] {
	return tabs.some(t => t.key === incoming.key)
		? tabs.map(t => (t.key === incoming.key ? { ...incoming, revision: t.revision } : t))
		: [...tabs, incoming]
}
export function restoreTabs(
	raw: string | null,
	resolve: (path: string) => any,
	allowed: (path: string) => boolean
): WorkTab[] {
	try {
		const items = JSON.parse(raw || '[]')
		if (!Array.isArray(items)) return []
		return items.slice(0, 24).reduce((tabs: WorkTab[], item: any) => {
			if (
				typeof item?.fullPath !== 'string' ||
				!item.fullPath.startsWith('/') ||
				item.fullPath.startsWith('//') ||
				item.fullPath.length > 1000
			)
				return tabs
			const route = resolve(item.fullPath),
				tab = tabFromRoute(route)
			return tab && route.matched?.length && allowed(route.path) ? upsertTab(tabs, tab) : tabs
		}, [])
	} catch {
		return []
	}
}
