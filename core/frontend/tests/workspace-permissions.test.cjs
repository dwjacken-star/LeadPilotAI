const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const { parse, compileScript } = require('vue/compiler-sfc')
const Vue = require('vue')

function compile(source, dependencies, globals = {}) {
	globals = { h: Vue.h, ...globals }
	const code = ts.transpileModule(source, {
		fileName: 'component.tsx',
		compilerOptions: {
			module: ts.ModuleKind.CommonJS,
			target: ts.ScriptTarget.ES2022,
			jsx: ts.JsxEmit.React,
			jsxFactory: 'h',
		},
	}).outputText
	const exports = {}
	new Function('exports', 'require', ...Object.keys(globals), code)(
		exports,
		name => dependencies[name] || require(name),
		...Object.values(globals)
	)
	return exports
}
const source = fs.readFileSync(path.join(__dirname, '../src/views/workspace/index.vue'), 'utf8')
const { descriptor } = parse(source)
const script = compileScript(descriptor, { id: 'permission-test', inlineTemplate: true }).content
const normalize = compile(
	fs.readFileSync(path.join(__dirname, '../src/features/workspace/normalize.ts'), 'utf8'),
	{}
)
const sections = compile(
	fs.readFileSync(path.join(__dirname, '../src/features/workspace/sections.ts'), 'utf8'),
	{}
)
const principal = Vue.ref(null)
let pause = 1
const api = {
	principal,
	catalog: Vue.ref([]),
	permissionDependencies: Vue.ref({}),
	can: code =>
		principal.value?.super || principal.value?.roles.some(role => role.permissions.includes(code)),
	loadPrincipal: async () => principal.value,
	workspace: async (action, params) => {
		assert.equal(action, 'list', 'rendering must not write or publish anything')
		return {
			list: [{ id: 1, task_process: 0, pause, temp_name: 'Template', email: 'test@example.com' }],
			total: 1,
		}
	},
}
const plain = {
	inheritAttrs: false,
	setup(_, { attrs, slots }) {
		return () => (attrs.show === false ? null : Vue.h('div', {}, slots.default?.()))
	},
}
const button = {
	setup:
		(_, { slots }) =>
		() =>
			Vue.h('button', {}, slots.default?.()),
}
const confirm = {
	emits: ['positiveClick'],
	setup:
		(_, { slots }) =>
		() =>
			slots.trigger?.(),
}
const table = {
	setup:
		(_, { attrs }) =>
		() =>
			Vue.h(
				'table',
				{},
				(attrs.data || []).flatMap(row =>
					attrs.columns.map(column => Vue.h('td', {}, column.render?.(row)))
				)
			),
}
const Workspace = compile(
	script,
	{
		vue: Vue,
		'naive-ui': { NButton: button, NSpace: plain, NPopconfirm: confirm },
		'@/utils': { Message: { success() {} } },
		'@/features/workspace/api': api,
		'@/features/workbench/context': { useWorkbenchActivity: () => Vue.ref(true) },
		'@/features/contacts/ContactImportPanel.vue': { default: plain },
		'@/features/contacts/import-state': { emptyContactImport: () => ({}) },
		'@/features/growth/market-data.json': { default: { timezones: [] } },
		'@/features/growth/options': { retainChoice: values => values },
		'@/features/workspace/RemoteSelect.vue': { default: plain },
		'@/features/workspace/PolicyFields.vue': { default: plain },
		'@/features/workspace/defaultSender': { getDefaultSender: async () => '' },
		'@/features/workspace/normalize': normalize,
		'@/features/workspace/sections': sections,
		'@/features/workspace/display': {
			formatWorkspaceCell: () => '',
			workspacePermissionLabel: code => code,
		},
	},
	{
		useI18n: () => ({ t: key => key }),
		useTemplateRef: Vue.useTemplateRef,
	}
).default

function host() {
	const node = (type, text = '') => ({ type, text, children: [], parent: null })
	function remove(el) {
		if (el.parent) el.parent.children = el.parent.children.filter(item => item !== el)
	}
	const renderer = Vue.createRenderer({
		createElement: node,
		createText: text => node('#text', text),
		createComment: text => node('#comment', text),
		insert(el, parent) {
			remove(el)
			el.parent = parent
			parent.children.push(el)
		},
		remove,
		setText(el, text) {
			el.text = text
		},
		setElementText(el, text) {
			el.text = text
			el.children = []
		},
		parentNode: el => el.parent,
		nextSibling: () => null,
		patchProp() {},
	})
	return { renderer, root: node('root') }
}
function text(node) {
	return node.text + node.children.map(text).join('')
}
function buttons(node) {
	return (node.type === 'button' ? [text(node)] : []).concat(node.children.flatMap(buttons))
}
async function rendered(section, tab, codes, callback) {
	principal.value = { super: false, roles: [{ permissions: codes }] }
	globalThis.document = { hidden: false }
	globalThis.location = { pathname: '/workspace' }
	const { renderer, root } = host()
	const app = renderer.createApp({ render: () => Vue.h(Workspace, { section, initialTab: tab }) })
	for (const name of new Set([...source.matchAll(/<(n-[\w-]+)/g)].map(match => match[1])))
		app.component(name, name === 'n-button' ? button : name === 'n-data-table' ? table : plain)
	app.component('bt-preview', plain)
	app.mount(root)
	try {
		await new Promise(resolve => setImmediate(resolve))
		await Vue.nextTick()
		callback(buttons(root), text(root))
	} finally {
		app.unmount()
	}
}
test('actual template buttons independently follow create, update and delete grants', async () => {
	for (const operation of ['', 'create', 'update', 'delete']) {
		await rendered(
			'templates',
			'templates',
			['template:list', ...(operation ? ['template:' + operation] : [])],
			labels => {
				assert.equal(labels.includes('workspaceCopy.w376'), operation === 'create')
				assert.equal(labels.includes('workspaceCopy.w168'), operation === 'update')
				assert.equal(labels.includes('workspaceCopy.w172'), operation === 'delete')
			}
		)
	}
})
test('actual contact buttons independently follow CRUD, import, export and tag assignment grants', async () => {
	for (const operation of ['', 'create', 'update', 'delete', 'import', 'export']) {
		await rendered(
			'contacts',
			'contacts',
			['contact:list', ...(operation ? ['contact:' + operation] : [])],
			labels => {
				assert.equal(labels.includes('workspaceCopy.w376'), operation === 'create')
				assert.equal(labels.includes('workspaceCopy.w168'), operation === 'update')
				assert.equal(labels.includes('workspaceCopy.w171'), operation === 'update')
				assert.equal(labels.includes('workspaceCopy.w172'), operation === 'delete')
				assert.equal(labels.includes('workspaceCopy.w49'), operation === 'import')
				assert.equal(labels.includes('workspaceCopy.w50'), operation === 'export')
			}
		)
	}
})
test('actual marketing buttons independently follow create, publish, pause and cancel grants', async () => {
	for (const operation of ['', 'create', 'publish', 'pause', 'cancel']) {
		pause = operation === 'pause' ? 0 : 1
		await rendered(
			'marketing',
			'tasks',
			['campaign:list', ...(operation ? ['campaign:' + operation] : [])],
			labels => {
				assert.equal(labels.includes('workspaceCopy.w64'), operation === 'create')
				assert.equal(labels.includes('workspaceCopy.w162'), operation === 'publish')
				assert.equal(labels.includes('emailVerification.recheck'), operation === 'publish')
				assert.equal(labels.includes('workspaceCopy.w163'), operation === 'pause')
				assert.equal(labels.includes('workspaceCopy.w164'), operation === 'cancel')
			}
		)
	}
})
test('scoped business pages do not expose unrelated workspace tabs or administrative tabs', async () => {
	await rendered(
		'templates',
		'templates',
		['template:list', 'contact:list', 'campaign:list'],
		(_, content) => {
			assert.ok(content.includes('workspaceCopy.w72'))
			for (const key of ['w66', 'w70', 'w74', 'w75', 'w76', 'w77'])
				assert.ok(!content.includes('workspaceCopy.' + key), key)
		}
	)
})
test('business route wrappers load the scoped view for members and preserve the legacy admin view', async () => {
	const { scopedBusinessView } = compile(
		fs.readFileSync(path.join(__dirname, '../src/features/workspace/routes.ts'), 'utf8'),
		{
			vue: Vue,
			'vue-router': { useRoute: () => ({ path: '/contacts/subscribers' }) },
			'./api': api,
			'./sections': sections,
			'@/views/workspace/index.vue': { __esModule: true, default: Workspace },
		}
	)
	let legacyLoads = 0
	const legacy = { render: () => Vue.h('div', 'Legacy admin view') }
	const view = scopedBusinessView('contacts', async () => {
		legacyLoads++
		return { __esModule: true, default: legacy }
	})
	const render = view.setup()
	principal.value = { super: false, roles: [{ permissions: ['contact:list'] }] }
	const memberNode = render()
	assert.equal(await memberNode.type.__asyncLoader(), Workspace)
	assert.equal(memberNode.props.section, 'contacts')
	assert.equal(memberNode.props.initialTab, 'contacts')
	assert.equal(legacyLoads, 0)
	principal.value = { super: true, roles: [] }
	assert.equal(await render().type.__asyncLoader(), legacy)
	assert.equal(legacyLoads, 1)
})

test('administrator-style marketing list keeps independent ordinary-role action buttons', async () => {
	const marketingSource = fs.readFileSync(
		path.join(__dirname, '../src/views/market/task/index.vue'),
		'utf8'
	)
	const { descriptor } = parse(marketingSource)
	const marketingScript = compileScript(descriptor, {
		id: 'legacy-marketing-test',
		inlineTemplate: true,
	}).content
	const Marketing = compile(
		marketingScript,
		{
			vue: Vue,
			'naive-ui': { NButton: button, NFlex: plain, NProgress: plain, NTag: plain },
			'@/features/workspace/api': api,
			'@/hooks/modal/useModal': { useModal: () => [plain, { setState() {}, open() {} }] },
			'@/hooks/useTableData': {
				useTableData: () => ({
					loading: Vue.ref(false),
					tableParams: Vue.ref({ page: 1, page_size: 10, keyword: '' }),
					tableList: Vue.ref([{ id: 1, task_process: 0, pause, progress: 0 }]),
					tableTotal: Vue.ref(1),
					getTableData() {},
				}),
			},
			'@/utils': { confirm() {}, formatDurationHighest: () => '', formatTime: () => '' },
			'@/api/modules/market/task': {},
			'./components/TaskDetail.vue': { default: plain },
			'./components/TaskStatus.vue': { default: plain },
			'./components/DeliveryChecks.vue': { default: plain },
		},
		{
			ref: Vue.ref,
			onMounted: Vue.onMounted,
			onBeforeUnmount: Vue.onBeforeUnmount,
			useI18n: () => ({ t: key => key }),
			useRouter: () => ({ push() {} }),
		}
	).default
	for (const operation of ['', 'create', 'update', 'delete', 'publish', 'pause']) {
		pause = operation === 'pause' ? 0 : 1
		principal.value = {
			super: false,
			roles: [{ permissions: ['campaign:list', ...(operation ? ['campaign:' + operation] : [])] }],
		}
		globalThis.document = { hidden: false }
		globalThis.location = { pathname: '/market/task' }
		const { renderer, root } = host()
		const app = renderer.createApp({ render: () => Vue.h(Marketing) })
		app.config.warnHandler = warning => {
			if (!warning.startsWith('Non-function value encountered for default slot'))
				console.warn(warning)
		}
		for (const name of new Set(
			[...marketingSource.matchAll(/<((?:n|bt)-[\w-]+)/g)].map(match => match[1])
		))
			if (name !== 'bt-table-layout')
				app.component(name, name === 'n-button' ? button : name === 'n-data-table' ? table : plain)
		app.component('bt-table-layout', {
			setup:
				(_, { slots }) =>
				() =>
					Vue.h(
						'div',
						{},
						Object.values(slots).flatMap(slot => slot())
					),
		})
		app.mount(root)
		try {
			await Vue.nextTick()
			const labels = buttons(root)
			assert.equal(labels.includes('market.task.actions.add'), operation === 'create')
			assert.equal(labels.includes('common.actions.copy'), operation === 'create')
			assert.equal(labels.includes('common.actions.edit'), operation === 'update')
			assert.equal(labels.includes('common.actions.delete'), operation === 'delete')
			assert.equal(labels.includes('market.task.actions.send'), operation === 'publish')
			assert.equal(labels.includes('market.task.actions.pause'), operation === 'pause')
			assert.ok(labels.includes('market.task.actions.analytics'))
		} finally {
			app.unmount()
		}
	}
})
