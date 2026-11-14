import { instance } from '@/api'
export const social = (action: string, params: Record<string, unknown> = {}): Promise<any> =>
	instance.post('/social/action', { action, ...params })
export async function uploadSocial(file: File, planID: number, aiAllowed: boolean) {
	const data = new FormData()
	data.append('file', file)
	data.append('plan_id', String(planID))
	data.append('ai_allowed', String(aiAllowed))
	return instance.post('/social/upload', data, {
		headers: { 'Content-Type': 'multipart/form-data' },
		timeout: 600000,
	})
}
export const socialFile = (id: string, source = 'task'): Promise<Blob> =>
	instance.post(
		'/social/file',
		{ id, source },
		{ responseType: 'blob', fetchOptions: { prefix: '/api', download: false } }
	)
export async function uploadPostImage(file: File, variantID: number): Promise<any> {
 const data = new FormData()
 data.append('file', file); data.append('purpose', 'post_image'); data.append('variant_id', String(variantID))
 return instance.post('/social/upload', data, {headers: {'Content-Type': 'multipart/form-data'}, timeout: 120000})
}
export async function uploadPostVideo(file: File, variantID: number, aiGenerated: boolean): Promise<any> {
	const data = new FormData()
	data.append('file', file)
	data.append('purpose', 'post_video')
	data.append('variant_id', String(variantID))
	data.append('ai_generated', String(aiGenerated))
	return instance.post('/social/upload', data, {
		headers: { 'Content-Type': 'multipart/form-data' },
		timeout: 180000,
	})
}
export const exportSocial = (planID?: number,filters:Record<string,unknown>={}) =>
	instance.post('/social/export', { plan_id: planID || 0,...filters }, { responseType: 'blob' })
