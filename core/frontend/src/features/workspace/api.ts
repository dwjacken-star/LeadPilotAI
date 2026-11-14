import { ref } from 'vue'
import { instance } from '@/api'
import { asArray } from './normalize'

export interface WorkspacePrincipal {
	id: number
	username: string
	dept_id: number
	dept_name?: string
	super: boolean
	roles: { role_id: number; role_name: string; permissions: string[]; data_scope: string }[]
}
export const principal = ref<WorkspacePrincipal | null>(null)
export const catalog = ref<string[]>([])
export const permissionDependencies = ref<Record<string, string[]>>({})
export async function workspace(action: string, params: Record<string, any> = {}): Promise<any> {
	return instance.post('/enterprise/action', { action, ...params })
}
export async function loadPrincipal() {
	const result = await workspace('me')
	principal.value = { ...result.principal, dept_name: result.dept_name || '', roles: asArray(result.principal?.roles).map((role: any) => ({ ...role, permissions: asArray(role.permissions) })) }
	catalog.value = asArray(result.catalog)
	permissionDependencies.value = result.permission_dependencies || {}
	return result.principal as WorkspacePrincipal
}
export function can(permission: string) {
	return !!principal.value && (principal.value.super || principal.value.roles.some(role => role.permissions.includes(permission) || role.permissions.includes('*')))
}
