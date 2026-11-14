import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/logs',
	redirect: '/logs/operation',
	component: Layout,
	meta: { key: 'logs', title: uiText('ui.p126dd3b70a5c'), titleKey: 'layout.menu.logs' },
	children: [
		{
			path: '/logs',
			redirect: '/logs/operation',
			name: 'Logs',
			component: () => import('@/views/logs/index.vue'),
			children: [
				{
					path: 'operation',
					name: 'Operation Logs',
					meta: { title: uiText('ui.p83a8a36e5256'), titleKey: 'ui.p83a8a36e5256' },
					component: () => import('@/views/logs/operate/index.vue'),
				},
				{
					path: 'error',
					name: 'Error Logs',
					meta: { title: uiText('ui.p1a1d9477fdb9'), titleKey: 'ui.p1a1d9477fdb9' },
					component: () => import('@/views/logs/error/index.vue'),
				},
			],
		},
	],
}

export default route
