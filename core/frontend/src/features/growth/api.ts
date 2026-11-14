import { instance } from '@/api'
export const growth = (action: string, params: Record<string, any> = {}): Promise<any> => instance.post('/growth/action', { action, ...params })
export function object(value: any): Record<string, any> { if (typeof value === 'string') { try { return JSON.parse(value) || {} } catch { return {} } }; return value && typeof value === 'object' ? value : {} }
export const list = (value: any): any[] => { if(typeof value==='string'){try{value=JSON.parse(value)}catch{return []}};return Array.isArray(value) ? value : [] }
export const workbook = (kind: string, company_id?: number, asset_id?: string) => instance.post('/growth/export', { kind, company_id, asset_id }, { responseType: 'blob' })
export const downloadMaterial = (asset_id: string) => instance.post('/growth/file', { asset_id }, { responseType: 'blob' })
export const downloadUploadTemplate = (kind: string, company_id: number, language: string) => instance.post('/growth/template', { kind, company_id, language }, { responseType: 'blob' })
export async function uploadMaterial(file: File, company_id: number, purpose: string, ai_allowed: boolean, model = '') {
 const data = new FormData(); data.append('model',model); data.append('file', file); data.append('company_id', String(company_id)); data.append('purpose', purpose); data.append('ai_allowed', String(ai_allowed))
 return instance.post('/growth/upload', data, { headers: { 'Content-Type': 'multipart/form-data' }, timeout: 600000 })
}
export function cleanFields(entries: { key: string; value: string }[]) { return Object.fromEntries(entries.filter(entry => entry.key.trim()).map(entry => [entry.key.trim(), entry.value])) }
export const fieldEntries = (value: any) => Object.entries(object(value)).map(([key, value]) => ({ key, value: typeof value === 'object' ? JSON.stringify(value) : String(value ?? '') }))
