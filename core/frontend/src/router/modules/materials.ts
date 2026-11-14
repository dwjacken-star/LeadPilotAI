import {uiText} from '@/i18n/labels'
import type { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'
const route: RouteRecordRaw = { path: '/materials', component: Layout, meta: { key: 'materials', title: uiText('ui.p2402ea363265'), titleKey: 'growth.materials' }, children: [{ path: '/materials', name: 'Materials', component: () => import('@/views/growth/materials.vue') }] }
export default route
