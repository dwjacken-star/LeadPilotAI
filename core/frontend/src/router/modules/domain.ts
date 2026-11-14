import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/domain',
	component: Layout,
	meta: { sort: 5, key: 'domain', title: uiText('ui.p5a710c24f533'), titleKey: 'layout.menu.domain' },
	children: [
		{
			path: '/domain',
			name: 'Domain',
			component: () => import('@/views/domain/index.vue'),
		},
		{
			path: 'edit-domain/:domain',
			name: 'EditDomain',
			component: () => import('@/views/domain/pages/editDomain/index.vue'),
		},
	],
}
  
export default route
