// Missing/error measurements are not measured zeroes. Keep the backend's -1
// sentinel, including old responses that omitted a field or encoded numbers.
export function overviewRate(value: unknown): number {
	if (value === null || value === undefined || (typeof value === 'string' && value.trim() === '')) return -1
	if (typeof value !== 'number' && typeof value !== 'string') return -1
	const number = Number(value)
	return Number.isFinite(number) && number >= 0 ? number : -1
}

export function deferredQueueCount(value: unknown): number {
	const number = overviewRate(value)
	return Number.isInteger(number) ? number : -1
}
