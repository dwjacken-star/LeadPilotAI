import {uiText} from '@/i18n/labels'
import type {RouteRecordRaw} from 'vue-router'
import {Layout} from '@/router/constant'
const route:RouteRecordRaw={path:'/crm',component:Layout,meta:{key:'crm',title:uiText('ui.p00aed7bc798f'),titleKey:'crm.title'},children:[{path:'/crm',name:'CRM',component:()=>import('@/views/crm/index.vue')}]}
export default route
