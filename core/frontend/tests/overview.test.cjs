const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const { parse, compileScript } = require('vue/compiler-sfc')
const Vue = require('vue')
const calculations = {}
new Function('exports', ts.transpileModule(fs.readFileSync(path.join(__dirname, '../src/views/overview/metrics.ts'), 'utf8'), {
	compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText)(calculations)

const plain = {
	setup: (_, { slots }) => () => Vue.h('div', {}, slots.default?.()),
}
const metric = {
	props: ['title', 'value'],
	setup: props => () => Vue.h('button', {}, `${props.title}:${props.value}`),
}
function component(file, dependencies, globals = {}) {
	const source = fs.readFileSync(path.join(__dirname, '../src/views/overview', file), 'utf8')
	const { descriptor } = parse(source)
	const script = compileScript(descriptor, { id: 'overview-test', inlineTemplate: true }).content
	const code = ts.transpileModule(script, {
		compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
	}).outputText
	const exports = {}
	const scope = {
		...Vue,
		useI18n: () => ({ t: key => key }),
		...globals,
	}
	new Function('exports', 'require', ...Object.keys(scope), code)(
		exports,
		name => {
			if (name in dependencies) return dependencies[name]
			if (name === './metrics') return calculations
			if (name.endsWith('.vue')) return { default: plain }
			return require(name)
		},
		...Object.values(scope),
	)
	return exports.default
}
function host(Component) {
	const node = (type, text = '') => ({ type, text, props: {}, children: [], parent: null })
	const remove = el => {
		if (el.parent) el.parent.children = el.parent.children.filter(item => item !== el)
	}
	const renderer = Vue.createRenderer({
		createElement: node, createText: text => node('#text', text), createComment: () => node('#comment'),
		insert(el, parent) { remove(el); el.parent = parent; parent.children.push(el) },
		remove, setText(el, text) { el.text = text },
		setElementText(el, text) { el.text = text; el.children = [] },
		parentNode: el => el.parent, nextSibling: () => null,
		patchProp(el, key, _, value) { el.props[key] = value },
	})
	const root = node('root')
	const app = renderer.createApp(Component)
	for (const name of ['n-alert', 'n-card', 'n-radio-group', 'n-radio-button', 'n-date-picker'])
		app.component(name, plain)
	app.component('n-select', {
		setup: (_, { attrs }) => () => Vue.h('select', {}, attrs.options.map(option => Vue.h('option', {}, option.label))),
	})
	app.config.globalProperties.$t = key => key
	app.mount(root)
	return { root, app }
}
const text = node => node.text + node.children.map(text).join('')
const find = (node, predicate) => predicate(node) ? node : node.children.map(child => find(child, predicate)).find(Boolean)
const settle = async () => { await new Promise(resolve => setImmediate(resolve)); await Vue.nextTick() }

test('Overview hides global queues for ordinary users and retains administrator drilldown', async () => {
	for (const queueAccess of [false, true]) {
		const navigations = []
		const requests = []
		const Overview = component('index.vue', {
			'@/features/workbench/context': { useWorkbenchActivity: () => Vue.ref(true) },
			'@vueuse/core': { useDebounceFn: fn => fn },
			'naive-ui': { useThemeVars: () => ({ warningColor: 'orange' }) },
			'@/utils': {
				getDayTimeRange: () => [100000, 200000],
				isArray: Array.isArray, isObject: value => !!value && typeof value === 'object',
			},
			'@/hooks/modal/useModal': { useModal: () => [plain, { setState() {}, open() {} }] },
			'@/api/modules/overview': {
				getOverviewInfo: async params => {
					requests.push(params)
					return {
						queue_access: queueAccess,
						dashboard: { sends: 2, open_tracked: 1, click_tracked: 1, delivery_rate: 50, click_rate: 25, bounce_rate: 10, delayed_queue: 987 },
						mail_providers: [], send_mail_chart: { dashboard: {}, data: [] },
						bounce_rate_chart: { data: [] }, open_rate_chart: { data: [] }, click_rate_chart: { data: [] },
					}
				},
			},
			'./components/MetricCard.vue': { default: metric },
		}, { useRouter: () => ({ push: route => navigations.push(route) }) })
		const { root, app } = host(Overview)
		try {
			await settle()
			assert.deepEqual(requests, [{ domain: '', start_time: 100, end_time: 200 }])
			assert.ok(text(root).includes('overview.delivered:50'))
			assert.ok(text(root).includes('overview.clicked:25'), 'nonzero click rates must reach the metric card')
			assert.ok(text(root).includes('overview.bounced:10'), 'nonzero bounce rates must reach the metric card')
			const queue = find(root, node => node.type === 'button' && node.text.startsWith('overview.delayedQueue'))
			if (queueAccess) {
				assert.ok(queue, 'administrator queue card must remain available')
				queue.props.onClick()
				assert.deepEqual(navigations, ['/settings/send-queue'])
			} else {
				assert.equal(queue, undefined, 'ordinary users must never see global queues')
				assert.deepEqual(navigations, [])
				assert.ok(!text(root).includes('987'))
			}
		} finally {
			app.unmount()
		}
	}
})

test('Overview domain selector uses only the governed domain API', async () => {
	let reads = 0
	const Filter = component('components/FilterBar.vue', {
		'@/utils': { getDayTimeRange: () => [100000, 200000], isArray: Array.isArray },
		'@/api/modules/overview': {
			getOverviewDomains: async () => { reads++; return ['own.example', 'shared.example'] },
		},
	})
	const { root, app } = host(Filter)
	try {
		await settle()
		assert.equal(reads, 1)
		const select = find(root, node => node.type === 'select')
		assert.equal(text(select), 'common.all.domainown.exampleshared.example')
	} finally {
		app.unmount()
	}
})
