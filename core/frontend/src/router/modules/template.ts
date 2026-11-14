import {uiText} from '@/i18n/labels'
import type { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'
import { scopedBusinessView } from '@/features/workspace/routes'

export default {
	path: '/template',
	component: Layout,
	meta: {
		sort: 3,
		title: uiText('ui.p3226ecbe6502'),
		key: 'template',
		titleKey: 'layout.menu.template',
	},
	children: [
		{
			path: '/template',
			name: 'template',
			component: scopedBusinessView('templates', () => import('@/views/template/index.vue')),
		},
		{
			path: 'ai-template/:chatId',
			name: 'ai-template',
			component: () => import('@/views/template/pages/AITemplate/index.vue'),
		},
	],
} as RouteRecordRaw
