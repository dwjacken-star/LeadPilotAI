import useWorkbenchStore from '@/store/modules/workbench'
import axios, { AxiosRequestConfig } from 'axios'
import router from '@/router'
import i18n from '@/i18n'
import { normalizeResponseBody, saveDownload } from '@/utils/response'
import { useUserStore } from '@/store'
import { apiUrlPrefix, isObject, Message } from '@/utils'

interface FetchOptions {
	download?: boolean
	prefix: string
	loading: string
	loadFn: () => void
	cancelResInterceptor: boolean
}

const instance = axios.create({
	baseURL: apiUrlPrefix,
	timeout: 600000,
	headers: {
		'Content-Type': 'application/json',
	},
	fetchOptions: {
		prefix: '/api',
		successMessage: false,
	},
})

const whitePathList = ['/login']

// 存储所有请求的 AbortController
const controllerStore = new Map<string, AbortController>()

let requestSequence = 0
const requestOwners = new Map<string, { key: string; identity: string }>(),
	requestCounts = new Map<string, number>()
const addController = (config: AxiosRequestConfig): void => {
	const key = String(++requestSequence),
		controller = new AbortController()
	;(config as any).__requestId = key
	const store = useWorkbenchStore(),
		owner = store.active
	if (owner) {
		requestOwners.set(key, { key: owner, identity: store.identity })
		requestCounts.set(
			store.identity + ':' + owner,
			(requestCounts.get(store.identity + ':' + owner) || 0) + 1
		)
		store.busy[owner] = true
	}
	if (config.signal) {
		const signal = config.signal
		if (signal.aborted) controller.abort()
		else signal.addEventListener?.('abort', () => controller.abort(), { once: true })
	}
	config.signal = controller.signal
	controllerStore.set(key, controller)
}
const removeController = (config: AxiosRequestConfig): void => {
	const key = (config as any).__requestId,
		owner = requestOwners.get(key)
	controllerStore.delete(key)
	requestOwners.delete(key)
	if (owner) {
		const scope = owner.identity + ':' + owner.key,
			count = Math.max(0, (requestCounts.get(scope) || 1) - 1)
		requestCounts.set(scope, count)
		const store = useWorkbenchStore()
		if (store.identity === owner.identity && store.tabs.some(tab => tab.key === owner.key))
			store.busy[owner.key] = count > 0
	}
}

// 请求拦截器 增加token
instance.interceptors.request.use(config => {
	addController(config)
	const { headers, url } = config
 headers['Accept-Language']=String(i18n.global.locale.value || 'en')
	if (!whitePathList.includes(url || '')) {
		const userStore = useUserStore()
		headers.Authorization = `Bearer ${userStore.login.token}`
	}
	return config
})

// 请求拦截器 处理请求前缀
instance.interceptors.request.use(config => {
	const { fetchOptions } = config
	if (isObject<FetchOptions>(fetchOptions)) {
		config.url = `${fetchOptions.prefix}` + config.url
	}
	return config
})

// 请求拦截器 处理请求前缀
instance.interceptors.request.use(config => {
	const { fetchOptions } = config
	if (isObject<FetchOptions>(fetchOptions) && fetchOptions.loading) {
		const { close } = Message.loading(fetchOptions.loading)
		fetchOptions.loadFn = close
	}
	return config
})

function messageText(raw: string) {
	const key = 'repair.' + raw
	return i18n.global.te(key) ? i18n.global.t(key) : raw
}

instance.interceptors.response.use(
	async response => {
		removeController(response.config)
		const options = response.config.fetchOptions
		options?.loadFn?.()
		const disposition = String(response.headers['content-disposition'] || '')
		let body: any = await normalizeResponseBody(response.data, disposition)
		if (body instanceof Blob) {
			if (options?.download !== false) saveDownload(body, disposition)
			return body
		}
		if (options?.cancelResInterceptor && typeof body === 'string') {
			if (!String(response.headers['content-type'] || '').includes('json')) return body
			body = JSON.parse(body)
		}
		const { code, data, msg, success } = body || {}
		if (code === 0 && success) {
			if (options?.successMessage && msg) Message.success(messageText(msg))
			return data
		}
		const error = Object.assign(new Error(messageText(msg || 'request_failed')), {
			code,
			msg: messageText(msg || 'request_failed'),
		})
		Message.error(error.message, { close: true })
		if (code === 401) {
			useUserStore().resetLoginInfo()
			router.push('/login')
		}
		return Promise.reject(error)
	},
	async error => {
		removeController(error.config || {})
		error.config?.fetchOptions?.loadFn?.()
		if (!axios.isCancel(error)) {
			let text = error.message || 'request_failed'
			try {
				const body: any = await normalizeResponseBody(error.response?.data)
				if (body?.msg) text = body.msg
			} catch {
				/* Use the transport error. */
			}
			Message.error(messageText(text), { close: true })
		}
		return Promise.reject(error)
	}
)

// 取消所有进行中的请求
const clearPendingRequests = (): void => {
	requestOwners.clear()
	requestCounts.clear()
	controllerStore.forEach((controller, key) => {
		controller.abort()
		controllerStore.delete(key)
	})
}

export { instance, clearPendingRequests }
