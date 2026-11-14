// Database JSON columns and empty Go slices may arrive as null on older installs.
export function asArray<T = any>(value: unknown): T[] {
	return Array.isArray(value) ? value : []
}

export function completePermissionSelection(codes: string[], dependencies: Record<string, string[]>) {
	const result = new Set(codes)
	for (const code of result)
		for (const dependency of dependencies[code] || []) result.add(dependency)
	return [...result]
}

export function normalizePolicy(value: any, defaults: Record<string, any> = {}) {
	const result = { ...defaults, ...(value && typeof value === 'object' && !Array.isArray(value) ? value : {}) }
	result.weekdays = asArray(result.weekdays)
	result.timezone = result.timezone || defaults.timezone || 'Asia/Shanghai'
	result.window_start = result.window_start || ''
	result.window_end = result.window_end || ''
	return result
}

export function normalizeCampaign(value: any, defaults: Record<string, any> = {}) {
	const result = { ...defaults, ...(value && typeof value === 'object' && !Array.isArray(value) ? value : {}) }
	result.tag_ids = asArray(result.tag_ids)
 result.senders=asArray(result.senders);if(!result.senders.length&&result.sender)result.senders=[result.sender]
 result.group_ids=asArray(result.group_ids);if(!result.group_ids.length&&result.group_id)result.group_ids=[result.group_id]
 result.cc_email=result.cc_email||''
	result.policy = normalizePolicy(result.policy, defaults.policy)
	return result
}
