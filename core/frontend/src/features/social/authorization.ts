export type TargetType = 'page' | 'person' | 'professional' | 'profile'
export type AuthorizationStatus =
	| 'pending'
	| 'ready'
	| 'confirmed'
	| 'cancelled'
	| 'failed'
	| 'expired'
export interface AuthorizationTask {
	id: number
	status: AuthorizationStatus
	platform: string
	target_type: TargetType
	expires_at?: string
	error?: string
	expected_account_id?: number
	url?: string
	candidates: { external_id: string; name: string; provider_name: string; provider_user: string }[]
}
export const authorizationStorageKey = 'social.authorization.tasks'
export function storedAuthorizationIds(text: string | null): number[] {
	try {
		const ids = JSON.parse(text || '[]')
		return Array.isArray(ids)
			? [...new Set(ids.filter(id => Number.isSafeInteger(id) && id > 0))]
			: []
	} catch {
		return []
	}
}
export function canConfirmAuthorization(task: AuthorizationTask, selected: string[]) {
	return (
		task.status === 'ready' &&
		selected.length > 0 &&
		(!task.expected_account_id || selected.length === 1) &&
		selected.every(id => task.candidates.some(candidate => candidate.external_id === id))
	)
}
export function facebookManualURL(value: unknown): string | null {
	try {
		const url = new URL(String(value))
		return url.protocol === 'https:' &&
			!url.username &&
			!url.password &&
			(url.hostname === 'facebook.com' || url.hostname.endsWith('.facebook.com')) &&
			(!url.port || url.port === '443')
			? url.href
			: null
	} catch {
		return null
	}
}
