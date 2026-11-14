export const MAX_POST_IMAGES = 10
export function canProcessPostImages(images: string[], prompt: string): boolean {
	return images.length > 0 || prompt.trim().length > 0
}
export function canRunSocialNow(status: string): boolean {
	return ['draft', 'scheduled', 'paused'].includes(status)
}
export function publishingTime(value = '09:00:00'): string {
	return /^\d{2}:\d{2}$/.test(value) ? value + ':00' : value
}
export function applyScheduleDate(config: { start_date: string }, value: string | null) {
	if (value && /^\d{4}-\d{2}-\d{2}$/.test(value)) config.start_date = value
}
export function singleFlight<T extends (...args: any[]) => Promise<any>>(action: T): T {
	let pending: ReturnType<T> | undefined
	return ((...args: Parameters<T>) => {
		if (!pending)
			pending = action(...args).finally(() => {
				pending = undefined
			}) as ReturnType<T>
		return pending
	}) as T
}
export function movePostImage(images: string[], index: number, direction: number): string[] {
	const result = [...images],
		target = index + direction
	if (index < 0 || index >= images.length || target < 0 || target >= images.length) return result
	;[result[index], result[target]] = [result[target], result[index]]
	return result
}
export function selectPostCandidate(images: string[], source: string, candidate: string): string[] {
	if (images.includes(candidate)) return images
	const index = images.indexOf(source),
		result = [...images]
	if (index >= 0) result[index] = candidate
	else if (result.length < MAX_POST_IMAGES) result.push(candidate)
	return result
}
