import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/settings',
	name: 'SettingsLayout',
	redirect: '/settings/common',
	meta: { sort: 9, key: 'settings', title: uiText('ui.pc7f73bb54d92'), titleKey: 'layout.menu.settings' },
	component: Layout,
	children: [
		{
			path: '/settings',
			name: 'Settings',
			redirect: '/settings/common',
			component: () => import('@/views/settings/index.vue'),
			children: [
				{
					path: 'common',
					name: 'SettingsCommon',
					meta: { title: uiText('ui.p7de90a65241a'), titleKey: 'layout.menu.common' },
					component: () => import('@/views/settings/common/index.vue'),
				},
				{
					path: 'service',
					name: 'SettingsService',
					meta: { title: uiText('ui.p329cb8b6ba8c'), titleKey: 'layout.menu.service' },

					component: () => import('@/views/settings/service/index.vue'),
				},
				{
					path: 'bcc',
					name: 'SettingsBcc',
					meta: { title: 'BCC', titleKey: 'layout.menu.bcc' },
					component: () => import('@/views/settings/bcc/index.vue'),
				},
				{
					path: 'forward',
					name: 'SettingsForward',
					meta: { title: uiText('ui.pba4e72261283'), titleKey: 'layout.menu.forward' },
					component: () => import('@/views/settings/forward/index.vue'),
				},
				{
					path: 'ai-model',
					name: 'AiModel',
					meta: { title: uiText('ui.p3d5ac060da0b'), titleKey: 'ui.p3d5ac060da0b' },
					component: () => import('@/views/settings/ai-model/index.vue'),
				},
				{
					path: 'send-queue',
					name: 'SendQueue',
					meta: { title: uiText('ui.p0371e6e56129'), titleKey: 'ui.p0371e6e56129' },
					component: () => import('@/views/settings/send-queue/index.vue'),
				},
			],
		},
		{
			path: 'rspamd',
			name: 'SettingsRspamd',
			meta: { title: uiText('ui.p6a6d8e4003e8'), titleKey: 'ui.p6a6d8e4003e8', hidden: true },
			component: () => import('@/views/settings/rspamd/index.vue'),
		},
	],
}

export default route
