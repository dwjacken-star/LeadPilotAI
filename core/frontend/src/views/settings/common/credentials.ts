import { useUserStore } from '@/store'
import { principal } from '@/features/workspace/api'
import router from '@/router'
import { Message } from '@/utils'
import i18n from '@/i18n'

export async function credentialChangeComplete() {
	useUserStore().resetLoginInfo()
	principal.value = null
	Message.success(i18n.global.locale.value === 'zh' ? '账号设置已生效，请使用新凭据重新登录。' : 'Account updated. Please sign in with your new credentials.')
	await router.replace('/login')
}
