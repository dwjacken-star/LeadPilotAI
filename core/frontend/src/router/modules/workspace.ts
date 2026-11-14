import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'
const route: RouteRecordRaw = {
	path: '/workspace', component: Layout, meta: { key: 'workspace', title: uiText('ui.p82cac0d4b61c'), titleKey: 'ui.p82cac0d4b61c' },
	children: [{ path: '/workspace', name: 'DepartmentWorkspace', component: () => import('@/views/workspace/index.vue') }],
}
export default route
