import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/send',
	component: Layout,
	meta: { sort: 3, key: 'api', title: uiText('ui.pda8cd86d7c62'), titleKey: 'layout.menu.sendApi' },
	children: [
		{
			path: '/send',
			name: 'Api',
			component: () => import('@/views/api/index.vue'),
		},
	],
}

export default route
