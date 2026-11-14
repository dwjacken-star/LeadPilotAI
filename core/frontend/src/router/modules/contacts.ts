import {uiText} from '@/i18n/labels'
import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'
import { scopedBusinessView } from '@/features/workspace/routes'

const route: RouteRecordRaw = {
	path: '/contacts',
	redirect: '/contacts/group',
	name: 'ContactsLayout',
	component: Layout,
	meta: {
		sort: 4,
		key: 'contacts',
		title: uiText('ui.pb0dd615cbbdc'),
		titleKey: 'layout.menu.contacts',
	},
	children: [
		{
			path: '/contacts',
			name: 'Contacts',
			redirect: '/contacts/group',
			component: scopedBusinessView('contacts', () => import('@/views/contacts/index.vue')),
			children: [
				{
					path: 'group',
					name: 'ContactsGroup',
					meta: { title: uiText('ui.p171a0606f7c7'), titleKey: 'layout.menu.group' },
					component: () => import('@/views/contacts/group/index.vue'),
				},
				{
					path: 'subscribers',
					name: 'ContactsSubscribers',
					meta: { title: uiText('ui.p6f13df39a03f'), titleKey: 'layout.menu.subscribers' },
					component: () => import('@/views/contacts/subscribers/index.vue'),
				},
				{
					path: 'suspend',
					name: 'ContactsSuspend',
					meta: { title: uiText('ui.pb32c9b19cf0d'), titleKey: 'ui.pb32c9b19cf0d' },
					component: () => import('@/views/contacts/suspend/index.vue'),
				},
				{
					path: 'tags',
					name: 'ContactsTags',
					meta: { title: uiText('ui.p848eed0fbd54'), titleKey: 'ui.p848eed0fbd54' },
					component: () => import('@/views/contacts/tags/index.vue'),
				},
			],
		},
		{
			path: 'settings/:id',
			name: 'ContactsSettings',
			meta: { title: uiText('ui.pc7f73bb54d92'), titleKey: 'ui.pc7f73bb54d92' },
			component: () => import('@/views/contacts/settings/index.vue'),
		},
	],
}

export default route
