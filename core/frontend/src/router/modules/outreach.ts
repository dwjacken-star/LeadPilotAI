import {uiText} from '@/i18n/labels'
import type { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'
const route: RouteRecordRaw = { path: '/outreach', component: Layout, meta: { key: 'outreach', title: uiText('ui.pca1a75cf018c'), titleKey: 'growth.outreach' }, children: [{ path: '/outreach', name: 'Outreach', component: () => import('@/views/growth/outreach.vue') }] }
export default route
