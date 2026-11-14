import { useGlobalStore, useUserStore } from '@/store'
import { setLanguage } from '@/i18n'
import { clearPendingRequests } from '@/api'
import router from '@/router/router'
import useWorkbenchStore from '@/store/modules/workbench'
import loadingBar from '@/config/loadingBar'
import { loadPrincipal, principal } from '@/features/workspace/api'
import { routeAllowed } from '@/features/growth/access'

router.beforeEach(async to => {
	loadingBar.start()

	const store = useUserStore()
	if (!store.isLogin) {
		clearPendingRequests()
		useWorkbenchStore().reset()
		principal.value = null
		return to.path === '/login' ? true : '/login'
	}
	if (to.path === '/login') return '/workspace'
	try {
		const account = await loadPrincipal()
		useWorkbenchStore().initialize(
			account.id,
			path => router.resolve(path),
			path => routeAllowed(account, path)
		)
		if (!routeAllowed(account, to.path)) return '/workspace'
		const globalStore = useGlobalStore()
		try {
			await globalStore.getLang()
			setLanguage(globalStore.lang)
		} catch {
			setLanguage(globalStore.lang)
		}
		return to.matched.length > 0 ? true : '/workspace'
	} catch {
		store.resetLoginInfo()
		principal.value = null
		return '/login'
	}
})
router.afterEach((to, from, failure) => {
	loadingBar.finish()
	if (!failure && to.path !== '/login') useWorkbenchStore().visit(to)
})
export default router
