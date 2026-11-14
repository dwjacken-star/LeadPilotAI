import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/market',
	redirect: '/market/task',
	name: 'MarketLayout',
	component: Layout,
	meta: {
		key: 'market',
		title: uiText('ui.p96ddece42801'),
		titleKey: 'layout.menu.market',
	},
	children: [
		{
			path: '/market',
			redirect: '/market/task',
			name: 'Market',
			component: () => import('@/views/market/index.vue'),
			children: [
				{
					path: 'task',
					name: 'MarketTask',
					meta: { title: uiText('ui.p7bb0ddf9221c'), titleKey: 'layout.menu.task' },
					component: () => import('@/views/market/task/index.vue'),
				},
				{
					path: 'template',
					name: 'MarketTemplate',
					meta: { title: uiText('ui.p3ec1ae061c27'), hidden: true, titleKey: 'layout.menu.template' },
					component: () => import('@/views/market/template/index.vue'),
				},
			],
		},
		{
			path: 'task/edit',
			name: 'MarketTaskEdit',
			meta: { title: uiText('ui.p7bb0ddf9221c'), titleKey: 'ui.p7bb0ddf9221c', hidden: true },
			component: () => import('@/views/market/task/edit.vue'),
		},
		{
			path: 'task/analytics/:id',
			name: 'MarketTaskAnalytics',
			meta: { title: uiText('ui.p25bc96295797'), titleKey: 'ui.p25bc96295797', hidden: true },
			component: () => import('@/views/market/task/analytics.vue'),
		},
	],
}

export default route
