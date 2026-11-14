import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/automation',
	component: Layout,
	meta: {
		sort: 3,
		key: 'automation',
		title: uiText('ui.pa15fde5173db'), titleKey: 'ui.pa15fde5173db',
		icon: 'i-mdi-refresh-auto',
		hidden: true,
	},
	children: [
		{
			path: '/automation',
			name: 'Automation',
			component: () => import('@/views/automation/index.vue'),
		},
	],
}

export default route
