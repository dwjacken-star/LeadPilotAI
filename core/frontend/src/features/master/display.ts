export const channels = ['email', 'facebook', 'linkedin', 'whatsapp'] as const
export const flows = ['WF01', 'WF02', 'WF03', 'WF04', 'WF05', 'WF06'] as const
export function object(value: unknown): Record<string, any> {
	if (typeof value === 'string') {
		try {
			return object(JSON.parse(value))
		} catch {
			return {}
		}
	}
	return value && typeof value === 'object' && !Array.isArray(value)
		? (value as Record<string, any>)
		: {}
}
export function array(value: unknown): any[] {
	if (typeof value === 'string') {
		try {
			return array(JSON.parse(value))
		} catch {
			return []
		}
	}
	return Array.isArray(value) ? value : []
}
export function canReview(status: string) {
	return ['draft', 'approved', 'blocked', 'rejected'].includes(status)
}
export function canResume(status: string) {
	return [
		'paused',
		'failed',
		'configuration_required',
		'budget_exhausted',
		'waiting_approval',
		'waiting_external',
	].includes(status)
}
export function replyNoticeKey(value: unknown) {
	const data = object(value)
	return data.automatic_reply_status === 'sent'
		? 'intel.auto_reply_sent'
		: data.automatic_reply_id
			? 'intel.auto_reply_check'
			: 'intel.reply_review_hint'
}
export function mergeOptions(old: any[], next: any[]) {
	return [...new Map([...old, ...next].map(option => [option.value, option])).values()]
}
export function offeringFields(pack: any, values: Record<string, any>) {
	return array(object(pack?.config).fields).map(field => ({
		...field,
		value:
			values[field.key] ??
			(field.type === 'list'
				? []
				: field.type === 'boolean'
					? false
					: field.type === 'number'
						? null
						: ''),
	}))
}
export function previewDocument(html: string) {
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src https: data:"><style>body{font:16px/1.7 Arial,sans-serif;margin:24px;color:#25352f;background:#fff;overflow-wrap:anywhere}p{margin:0 0 16px}img{max-width:100%;height:auto}table{max-width:100%}</style></head><body>${html}</body></html>`
}
