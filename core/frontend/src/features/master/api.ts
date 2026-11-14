import { instance } from '@/api'
import { growth } from '@/features/growth/api'
export const master = (kind: string, params: Record<string, any> = {}): Promise<any> =>
	growth('master', { kind, ...params })
export function uploadIndustry(file: File, companyId: number, packId: number) {
	const body = new FormData()
	body.append('file', file)
	body.append('company_id', String(companyId))
	body.append('pack_id', String(packId))
	body.append('purpose', 'industry_offerings')
	return instance.post('/growth/upload', body, {
		headers: { 'Content-Type': 'multipart/form-data' },
	})
}
