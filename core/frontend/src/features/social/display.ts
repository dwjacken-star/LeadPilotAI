export function socialPlatformName(value: string): string {
	const names: Record<string, string> = {
		facebook: 'Facebook',
		linkedin: 'LinkedIn',
		instagram: 'Instagram',
		threads: 'Threads',
	}
	return names[value] || value
}

export function valueObject(value: unknown): Record<string, any> {
	if (typeof value === 'string') {
		try {
			return JSON.parse(value) || {}
		} catch {
			return {}
		}
	}
	return value && typeof value === 'object' ? (value as Record<string, any>) : {}
}
export function values(value: unknown): any[] {
	if (typeof value === 'string') {
		try {
			value = JSON.parse(value)
		} catch {
			return []
		}
	}
	return Array.isArray(value) ? value : []
}
export function mergeOptions(
	old: { value: number | string; label: string }[],
	rows: { value: number | string; label: string }[]
) {
	return [...new Map([...old, ...rows].map(row => [row.value, row])).values()]
}
// PostgreSQL responses without an offset are serialized in server UTC.
export function socialDate(value: unknown) {
	if (value instanceof Date) return new Date(value.getTime())
	if (typeof value === 'number') return new Date(value)
	const text = String(value).replace(' ', 'T')
	return new Date(text + (/[Zz]$|[+-]\d\d:?\d\d$/.test(text) ? '' : 'Z'))
}
export function metricValue(row: Record<string, any>, key: string): number | null {
	if (row.publish_mode === 'manual') return null
	const counts = valueObject(row.counts),
		availability = valueObject(row.availability)
	const value = Number(counts[key])
	return availability[key] === 'available' &&
		counts[key] !== null &&
		counts[key] !== undefined &&
		Number.isFinite(value)
		? value
		: null
}
export function metricSummary(rows: Record<string, any>[], key: string) {
	const available = rows.map(row => metricValue(row, key)).filter((v): v is number => v !== null)
	return {
		value: available.length ? available.reduce((a, b) => a + b, 0) : null,
		available: available.length,
		total: rows.length,
	}
}
export function socialStateClass(state: string) {
	return [
		'published',
		'manual_confirmed',
		'confirmed',
		'completed',
		'connected',
		'approved',
		'ready',
		'handled',
		'synced',
	].includes(state)
		? 'success'
		: [
					'failed',
					'expired',
					'manual_due',
					'unknown',
					'reconnect',
					'configuration_required',
					'unavailable',
			  ].includes(state)
			? 'warning'
			: 'info'
}
export function syncNotice(
	value: unknown
): { key: string; values: Record<string, number | string> } | null {
	const text = String(value || '')
	const account = /^POST_SYNC_UNAVAILABLE:(\d+)$/.exec(text)
	if (account) return { key: 'social.sync_partial', values: { count: Number(account[1]) } }
	const post = /^FACEBOOK_POST_UNAVAILABLE:(\d+)\/(\d+)$/.exec(text)
	if (post)
		return { key: 'social.post_unavailable_hint', values: { code: post[1] + '/' + post[2] } }
	return null
}

export function reportMetricKeys(platform: string | null, rows: Record<string, any>[]): string[] {
	const actual =
		platform ||
		(rows.length && rows.every(row => row.platform === rows[0].platform) ? rows[0].platform : '')
	return [
		'reactions',
		'comments',
		'shares',
		...(actual === 'facebook'
			? ['views']
			: actual === 'linkedin'
				? ['impressions']
				: ['views', 'impressions']),
		'clicks',
	]
}
