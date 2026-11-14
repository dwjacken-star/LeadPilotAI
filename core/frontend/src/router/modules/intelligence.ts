import {uiText} from '@/i18n/labels'
import type {RouteRecordRaw} from 'vue-router'
import Layout from '@/layout/index.vue'
const route:RouteRecordRaw={path:'/intelligence',component:Layout,meta:{key:'intelligence',title:uiText('ui.p665ed6ae563a'),titleKey:'intel.title'},children:[{path:'/intelligence',name:'GrowthMaster',component:()=>import('@/views/master/index.vue')}]}
export default route
