import {uiText} from '@/i18n/labels'
import type { RouteRecordRaw } from 'vue-router'
import Layout from '@/layout/index.vue'
const route: RouteRecordRaw = {
	path: '/social',
	component: Layout,
	meta: { key: 'social', title: uiText('ui.pf5bb9f231eb0'), titleKey: 'social.title' },
	children: [
		{
			path: '/social',
			name: 'SocialMarketing',
			component: () => import('@/views/social/index.vue'),
		},
	],
}
export default route
