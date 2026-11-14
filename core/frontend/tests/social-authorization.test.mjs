import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import vm from 'node:vm'
import * as Vue from 'vue'
import { parse, compileScript, compileTemplate } from 'vue/compiler-sfc'
import ts from 'typescript'
import * as authorization from '../src/features/social/authorization.ts'
import * as display from '../src/features/social/display.ts'
import {
	storedAuthorizationIds,
	canConfirmAuthorization,
	facebookManualURL,
} from '../src/features/social/authorization.ts'

test('social page and task dialogs compile with their template event handlers', () => {
	for (const file of [
		'../src/views/social/index.vue',
		...fs
			.readdirSync(new URL('../src/features/social/', import.meta.url))
			.filter(file => file.endsWith('.vue'))
			.map(file => '../src/features/social/' + file),
	]) {
		const parsed = parse(fs.readFileSync(new URL(file, import.meta.url), 'utf8'))
		assert.deepEqual(parsed.errors, [], file)
		const result = compileTemplate({
			source: parsed.descriptor.template.content,
			filename: file,
			id: 'social',
			compilerOptions: { expressionPlugins: ['typescript'] },
		})
		assert.deepEqual(result.errors, [], file)
	}
})

function setupComponent(file, props, api, storage = new Map()) {
	const descriptor = parse(
		fs.readFileSync(new URL(`../src/features/social/${file}`, import.meta.url), 'utf8')
	).descriptor
	const script = compileScript(descriptor, { id: 'social-test' })
	const code = ts.transpileModule(script.content, {
		compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
	}).outputText
	const mounted = [],
		copied = [],
		emitted = [],
		fileCalls = [],
		downloads = [],
		exposed = {}
	const module = { exports: {} }
	const mocks = {
		vue: Vue,
		'./api': {
			social: api,
			uploadPostVideo: (file, variantID, aiGenerated) => api('upload_video', { file, variantID, aiGenerated }),
			socialFile: async (id, source) => {
				fileCalls.push({ id, source })
				return new Blob(['image'], { type: 'image/png' })
			},
		},
		'./authorization': authorization,
		'./display': display,
		'@/features/workbench/context': { useWorkbenchActivity: () => Vue.ref(true) },
		'@/features/workspace/api': { can: () => true },
		'@/utils': { Message: { success() {} } },
		'@/hooks/useCopy': { useCopy: () => ({ copyText: async value => copied.push(value) }) },
	}
	vm.runInNewContext(code, {
		module,
		exports: module.exports,
		require: name => {
			if (!(name in mocks)) throw Error(`Unexpected import ${name}`)
			return mocks[name]
		},
		ref: Vue.ref,
		computed: Vue.computed,
		watch: Vue.watch,
		useI18n: () => ({ t: key => key, locale: Vue.ref('en') }),
		onMounted: fn => mounted.push(fn),
		onBeforeUnmount() {},
		setInterval: () => 1,
		clearInterval() {},
		setTimeout: callback => callback(),
		URL: { createObjectURL: () => 'blob:social-image', revokeObjectURL() {} },
		document: {
			createElement: () => {
				const link = {
					href: '',
					download: '',
					click: () => downloads.push({ href: link.href, filename: link.download }),
				}
				return link
			},
		},
		sessionStorage: {
			getItem: key => storage.get(key) || null,
			setItem: (key, value) => storage.set(key, value),
		},
	})
	const scope = Vue.effectScope()
	const state = scope.run(() =>
		module.exports.default.setup(props, {
			expose: value => Object.assign(exposed, value),
			emit: (...args) => emitted.push(args),
		})
	)
	const template = compileTemplate({
		source: descriptor.template.content,
		filename: file,
		id: 'social-test',
		compilerOptions: { expressionPlugins: ['typescript'] },
	})
	assert.deepEqual(template.errors, [])
	const templateModule = { exports: {} }
	vm.runInNewContext(
		ts.transpileModule(template.code, {
			compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
		}).outputText,
		{
			module: templateModule,
			exports: templateModule.exports,
			require: () => ({
				...Vue,
				resolveComponent: name => ({ name }),
				withDirectives: node => node,
			}),
		}
	)
	return {
		state,
		exposed,
		storage,
		copied,
		emitted,
		fileCalls,
		downloads,
		render: () => templateModule.exports.render(Vue.proxyRefs({ ...state, ...props }), []),
		mount: async () => {
			for (const fn of mounted) await fn()
		},
		stop: () => scope.stop(),
	}
}

function renderedNodes(root, parents = []) {
	if (!root || typeof root !== 'object') return []
	if (Array.isArray(root)) return root.flatMap(node => renderedNodes(node, parents))
	const name = root.type?.name
	const nextParents = name ? [...parents, name] : parents
	const children = root.children
	return [
		{ node: root, parents },
		...renderedNodes(Array.isArray(children) ? children : null, nextParents),
		...(children && !Array.isArray(children) && typeof children === 'object'
			? Object.values(children)
					.filter(value => typeof value === 'function')
					.flatMap(slot => renderedNodes(slot(), nextParents))
			: []),
	]
}

test('video inspection is explicit and failed uploads do not replace the reviewed selection', async () => {
	const requests = []
	const editor = setupComponent('PostVideoEditor.vue', { variantId: 42, editable: true, video: 'task:old' }, async (action, data) => {
		requests.push({ action, data })
		return { reference: 'task:inspected', video_manifest: { width: 720, height: 1280 } }
	})
	editor.state.aiGenerated.value = true
	const input = { files: [{ name: 'original.mp4' }], value: 'selected' }
	await editor.state.upload({ target: input })
	assert.equal(requests.length, 1)
	assert.equal(requests[0].action, 'upload_video')
	assert.equal(requests[0].data.variantID, 42)
	assert.equal(requests[0].data.aiGenerated, true)
	assert.deepEqual(editor.emitted.map(event => [...event]), [['update:video', 'task:inspected']])
	assert.equal(input.value, '')
	editor.stop()
	const failed = setupComponent('PostVideoEditor.vue', { variantId: 42, editable: true, video: 'task:old' }, async () => { throw Error('inspection unavailable') })
	await failed.state.upload({ target: { files: [{ name: 'video.mp4' }], value: 'selected' } })
	assert.equal(failed.emitted.length, 0)
	assert.equal(failed.state.loading.value, false)
	failed.stop()
})

test('Meta authorization targets stay independent and readiness never activates publishing', async () => {
	const calls = []
	const component = setupComponent(
		'AccountAuthorization.vue',
		{ availability: {}, showConnect: true },
		async action => {
			calls.push(action)
			return {}
		}
	)
	try {
		component.state.open('instagram')
		assert.equal(component.state.targetType.value, 'professional')
		assert.deepEqual(
			Array.from(component.state.targetOptions.value, item => item.value),
			['professional']
		)
		component.state.open('threads')
		assert.equal(component.state.targetType.value, 'profile')
		assert.deepEqual(
			Array.from(component.state.targetOptions.value, item => item.value),
			['profile']
		)
		assert.equal(calls.length, 0)
		assert.equal(display.socialPlatformName('instagram'), 'Instagram')
		assert.equal(display.socialPlatformName('threads'), 'Threads')
	} finally {
		component.stop()
	}
	const settings = { instagram: { enabled: false }, threads: { enabled: false } }
	const preparation = setupComponent('MetaPreparation.vue', { settings }, async action => {
		calls.push(action)
		return { external_access: 'unverified', video_publishing: false }
	})
	try {
		await preparation.mount()
		assert.deepEqual(calls, ['meta_readiness'])
		assert.equal(settings.instagram.enabled, false)
		assert.equal(settings.threads.enabled, false)
		assert.equal(preparation.state.readiness.value.external_access, 'unverified')
	} finally {
		preparation.stop()
	}
})

test('OAuth tasks keep independent links and do not connect on readiness or page reload', async t => {
	let nextId = 40
	const calls = []
	const api = async (action, params) => {
		calls.push({ action, params: params && JSON.parse(JSON.stringify(params)) })
		if (action === 'authorize')
			return { id: ++nextId, url: `https://provider.example/${nextId}`, expires_in: 600 }
		if (action === 'authorization_status')
			return {
				id: params.id,
				status: 'ready',
				platform: 'linkedin',
				target_type: 'page',
				candidates: [
					{
						external_id: 'page-a',
						name: 'Page A',
						provider_name: 'Real authorizer',
						provider_user: 'public-id',
					},
				],
			}
		return { connected: 1 }
	}
	const component = setupComponent(
		'AccountAuthorization.vue',
		{ availability: { linkedin: { enabled: true } }, showConnect: true },
		api
	)
	try {
		await component.state.start('linkedin', 'page')
		await component.state.start('linkedin', 'person')
		assert.equal(component.state.tasks.value.length, 2)
		assert.equal(component.state.tasks.value[0].url, 'https://provider.example/41')
		assert.equal(component.state.tasks.value[1].url, 'https://provider.example/42')
		await component.state.confirmTask(component.state.tasks.value[0])
		assert.ok(!calls.some(call => call.action === 'authorization_confirm'))
		const restored = setupComponent(
			'AccountAuthorization.vue',
			{ availability: {}, showConnect: true },
			api,
			component.storage
		)
		try {
			await restored.mount()
			assert.equal(restored.state.tasks.value.length, 2)
			assert.equal(restored.state.show.value, true)
			assert.ok(!calls.some(call => call.action === 'authorization_confirm'))
		} finally {
			restored.stop()
		}
		const addedTask = component.state.tasks.value[0]
		component.state.selections.value[addedTask.id] = ['page-a']
		component.state.providerChanges.value[addedTask.id] = true
		await component.state.confirmTask(addedTask)
		assert.equal(
			calls.filter(call => call.action === 'authorization_confirm').at(-1).params.data
				.allow_provider_change,
			true
		)
		await component.exposed.reconnectAccount({
			id: 9,
			platform: 'linkedin',
			target_type: 'person',
			name: 'Original person',
		})

		await t.test(
			'inaccessible restored OAuth tasks pause only their own polling and can be retried or forgotten locally',
			async () => {
				let inaccessible = true
				const calls = []
				const storage = new Map([[authorization.authorizationStorageKey, '[77,78]']])
				const component = setupComponent(
					'AccountAuthorization.vue',
					{ availability: {}, showConnect: true },
					async (action, params) => {
						calls.push({ action, id: params.id })
						if (params.id === 77 && inaccessible) throw Error('Task unavailable')
						return {
							id: params.id,
							status: 'pending',
							platform: 'linkedin',
							target_type: 'page',
							candidates: [],
						}
					},
					storage
				)
				try {
					await component.mount()
					assert.equal(component.state.tasks.value.find(task => task.id === 77).status, 'pending')
					assert.equal(component.state.pollingErrors.value[77], true)
					assert.equal(component.state.pollingErrors.value[78], undefined)
					assert.equal(
						renderedNodes(component.render()).filter(
							({ node }) =>
								node.type?.name === 'n-button' &&
								node.children.default?.().some(child => child.children === 'social.auth_forget')
						).length,
						2
					)
					const failedCalls = () => calls.filter(call => call.id === 77).length
					const originalCalls = failedCalls()
					await component.state.refreshTasks()
					await component.state.refreshTasks()
					assert.equal(failedCalls(), originalCalls)
					assert.ok(calls.filter(call => call.id === 78).length >= 3)
					inaccessible = false
					await component.state.refreshTask(77)
					assert.equal(component.state.pollingErrors.value[77], undefined)
					await component.state.refreshTasks()
					assert.equal(failedCalls(), originalCalls + 2)
					inaccessible = true
					await assert.rejects(component.state.refreshTask(77), /Task unavailable/)
					component.state.forget(77)
					assert.equal(component.state.pollingErrors.value[77], undefined)
					assert.deepEqual(JSON.parse(storage.get(authorization.authorizationStorageKey)), [78])
					assert.ok(!calls.some(call => call.action === 'authorization_cancel'))
				} finally {
					component.stop()
				}
			}
		)
		assert.deepEqual(calls.filter(call => call.action === 'authorize').at(-1).params.data, {
			platform: 'linkedin',
			target_type: 'person',
			account_id: 9,
		})
		const task = component.state.tasks.value.at(-1)
		component.state.selections.value[task.id] = ['page-a']
		await component.state.confirmTask(task)
		assert.deepEqual(
			calls.filter(call => call.action === 'authorization_confirm').at(-1).params.data,
			{ external_ids: ['page-a'], allow_provider_change: false }
		)
	} finally {
		component.stop()
	}
})

test('manual drafts copy existing text and link and only record an explicit due confirmation', async t => {
	const calls = []
	const delivery = {
		id: 12,
		publish_mode: 'manual',
		status: 'scheduled',
		snapshot: JSON.stringify({
			body: 'AI draft',
			link: 'https://shop.example/',
			images: ['task:4', 'library:5'],
		}),
	}
	const component = setupComponent('ManualDelivery.vue', { delivery }, async (action, params) => {
		calls.push({ action, params: JSON.parse(JSON.stringify(params)) })
		return {}
	})

	await t.test(
		'manual draft UUID images render download buttons and request both existing asset sources',
		async () => {
			const taskId = 'c88595e0-8ca1-47ae-8d85-48d59a8861e3'
			const libraryId = '5dd4cef2-2645-4efe-a46a-eebc743756d9'
			const delivery = {
				id: 13,
				publish_mode: 'manual',
				status: 'scheduled',
				snapshot: {
					images: [
						`task:${taskId}`,
						`library:${libraryId}`,
						'task:42',
						'task:',
						'task:not-a-uuid',
						'other:42',
					],
				},
			}
			const component = setupComponent('ManualDelivery.vue', { delivery }, async () => ({}))
			try {
				const buttons = renderedNodes(component.render()).filter(
					({ node }) =>
						node.type?.name === 'n-button' &&
						node.children
							.default?.()
							.some(child => child.children === 'social.manual_download_image')
				)
				assert.equal(buttons.length, 3)
				await buttons[0].node.props.onClick()
				await buttons[1].node.props.onClick()
				assert.deepEqual(component.fileCalls, [
					{ id: taskId, source: 'task' },
					{ id: libraryId, source: 'library' },
				])
				assert.deepEqual(
					component.downloads.map(row => row.filename),
					['post-13-1.png', 'post-13-2.png']
				)
			} finally {
				component.stop()
			}
		}
	)

	await t.test(
		'manual cancellation is standalone for scheduled and due rows and unavailable after confirmation',
		async () => {
			for (const [status, count] of [
				['scheduled', 1],
				['manual_due', 2],
				['manual_confirmed', 0],
			]) {
				const calls = []
				const component = setupComponent(
					'ManualDelivery.vue',
					{
						delivery: { id: 14, publish_mode: 'manual', status, snapshot: {} },
					},
					async (action, params) => {
						calls.push({ action, params: JSON.parse(JSON.stringify(params)) })
						return {}
					}
				)
				try {
					const confirmations = renderedNodes(component.render()).filter(
						({ node }) => node.type?.name === 'n-popconfirm'
					)
					assert.equal(confirmations.length, count, status)
					assert.ok(
						confirmations.every(({ parents }) => !parents.includes('n-popconfirm')),
						status
					)
					if (count) {
						const cancellation = confirmations.find(({ node }) =>
							node.children
								.default()
								.some(child => String(child.children).trim() === 'social.manual_cancel_warning')
						)
						assert.ok(cancellation, status)
						await cancellation.node.props.onPositiveClick()
						assert.deepEqual(
							calls,
							[{ action: 'delivery_control', params: { id: 14, data: { operation: 'cancel' } } }],
							status
						)
					} else {
						await component.state.cancel()
						assert.deepEqual(calls, [], status)
					}
				} finally {
					component.stop()
				}
			}
		}
	)
	try {
		await component.state.copyDraft()
		assert.deepEqual(component.copied, ['AI draft\n\nhttps://shop.example/'])
		assert.deepEqual([...component.state.images.value], ['task:4', 'library:5'])
		await component.state.markPublished()
		assert.equal(calls.length, 0)
		delivery.status = 'manual_due'
		component.state.externalURL.value = 'https://facebook.com.evil.example/post/1'
		await component.state.markPublished()
		assert.equal(calls.length, 0)
		component.state.externalURL.value = 'https://www.facebook.com/posts/1'
		await component.state.markPublished()
		assert.deepEqual(calls[0], {
			action: 'delivery_control',
			params: {
				id: 12,
				data: {
					operation: 'manual_confirm',
					manual_confirmed: true,
					external_url: 'https://www.facebook.com/posts/1',
				},
			},
		})
		assert.equal(component.emitted[0][0], 'changed')
	} finally {
		component.stop()
	}
})

test('independent authorization IDs survive reload without storing provider credentials', () => {
	assert.deepEqual(storedAuthorizationIds('[21,22,21,0,-1,"23",null,1.5]'), [21, 22])
	assert.deepEqual(storedAuthorizationIds('{"id":21}'), [])
	assert.deepEqual(storedAuthorizationIds('invalid JSON'), [])
	assert.deepEqual(storedAuthorizationIds(null), [])
})
test('authorization requires ready status and explicit valid targets; reconnect binds one target', () => {
	const task = {
		id: 21,
		status: 'ready',
		platform: 'linkedin',
		target_type: 'page',
		candidates: [{ external_id: 'a' }, { external_id: 'b' }],
	}
	assert.equal(canConfirmAuthorization(task, []), false)
	assert.equal(canConfirmAuthorization(task, ['a', 'b']), true)
	assert.equal(canConfirmAuthorization(task, ['unknown']), false)
	for (const status of ['pending', 'confirmed', 'cancelled', 'failed', 'expired'])
		assert.equal(canConfirmAuthorization({ ...task, status }, ['a']), false)
	assert.equal(canConfirmAuthorization({ ...task, expected_account_id: 9 }, ['a', 'b']), false)
	assert.equal(canConfirmAuthorization({ ...task, expected_account_id: 9 }, ['a']), true)
})
test('manual receipt links allow only HTTPS Facebook without credentials or lookalike hosts', () => {
	assert.equal(
		facebookManualURL('https://www.facebook.com/user/posts/42'),
		'https://www.facebook.com/user/posts/42'
	)
	assert.equal(
		facebookManualURL('https://m.facebook.com/posts/42'),
		'https://m.facebook.com/posts/42'
	)
	for (const link of [
		'',
		'http://facebook.com/post/42',
		'https://facebook.com.evil.example/post/42',
		'https://evilfacebook.com/post/42',
		'javascript:alert(1)',
		'https://user:secret@facebook.com/post/42',
		'https://facebook.com:444/post/42',
	])
		assert.equal(facebookManualURL(link), null, link)
})
