import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { is, isDev } from '@/utils'
import { sortMenuRoutes } from './menu-order'
import Layout from '@/layout/index.vue'

// Routes reflect list
const routesReflectList = [
	'部门工作台',
	'Overview',
	'CRM',
	'Email Marketing',
	'template',
	'Send API',
	'Contacts',
	'MailDomain',
	'MailBoxes',
	'SMTP',
	'Logs',
	'Settings',
	'Automation',
]

// Import routes from modules
const modules = import.meta.webpackContext('./modules', {
	// Whether to search for subdirectories
	recursive: false,
	regExp: /\.ts$/,
})

// Module routes
export let menuList: RouteRecordRaw[] = []

// Iterate through the module list to generate module routes
for (const path of modules.keys()) {
	const mod = modules(path)
	if (is<{ default: RouteRecordRaw }>(mod, 'Module')) {
		menuList.push(mod.default)
	}
}

// Menu ordering uses stable keys; localized or renamed titles cannot create holes.
menuList = sortMenuRoutes(menuList)

const otherArray: RouteRecordRaw[] = []

if (isDev) {
	otherArray.push({
		path: '/test',
		name: 'Test',
		component: () => import('@/views/test/index.vue'),
	})
}

export const routes: RouteRecordRaw[] = [
	{
		path: '/login',
		name: 'Login',
		component: () => import('@/views/login/index.vue'),
	},
	{
		path: '/',
		component: Layout,
		redirect: '/overview',
		children: menuList.map(route => ({ ...route, component: undefined })),
	},
	...otherArray,
]

const router = createRouter({
	history: createWebHistory('/'),
	routes,
	strict: false,
	scrollBehavior: () => ({ left: 0, top: 0 }),
})

export default router
