const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const Vue = require('vue')
const { parse, compileScript } = require('vue/compiler-sfc')

const source = file => fs.readFileSync(path.join(__dirname, '../src', file), 'utf8')
function compile(script, dependencies = {}) {
	const code = ts.transpileModule(script, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
	const exports = {}
	new Function('exports', 'require', ...Object.keys(Vue), code)(exports, name => dependencies[name] || require(name), ...Object.values(Vue))
	return exports
}
function component(file, dependencies = {}) {
	const { descriptor } = parse(source(file))
	return compile(compileScript(descriptor, { id: 'ui-regression', inlineTemplate: true }).content, dependencies).default
}
const emailContent = compile(source('utils/email-content.ts'))
const preview = compile(source('utils/email-preview.ts'), { './email-content': emailContent })
const metrics = compile(source('views/overview/metrics.ts'))
const settle = async () => { await new Promise(resolve => setImmediate(resolve)); await Vue.nextTick() }

function mount(Component, props) {
	const node = (type, text = '') => ({ type, text, props: {}, children: [], parent: null })
	const remove = el => { if (el.parent) el.parent.children = el.parent.children.filter(v => v !== el) }
	const renderer = Vue.createRenderer({
		createElement: node, createText: text => node('#text', text), createComment: () => node('#comment'),
		insert(el, parent) { remove(el); el.parent = parent; parent.children.push(el) }, remove,
		setText(el, text) { el.text = text }, setElementText(el, text) { el.text = text; el.children = [] },
		parentNode: el => el.parent, nextSibling: () => null, patchProp(el, key, _, value) { el.props[key] = value },
	})
	const root = node('root')
	const app = renderer.createApp({ setup: () => () => Vue.h(Component, props) })
	app.component('n-select', { setup: (_, { attrs }) => () => Vue.h('select', { ...attrs }, (attrs.options || []).map(o => Vue.h('option', {}, o.label))) })
	app.config.globalProperties.$t = key => key
	app.mount(root)
	const find = (el, type) => el.type === type ? el : el.children.map(c => find(c, type)).find(Boolean)
	return { app, root, get: type => find(root, type) }
}
function remote(workspace) {
	return component('features/workspace/RemoteSelect.vue', {
		'./api': { workspace }, './normalize': { asArray: value => Array.isArray(value) ? value : [] },
	})
}
const mailbox = username => ({ username, domain: 'example.org', active: 1 })

test('overview preserves nonzero rates/counts and distinguishes unavailable from real zero', () => {
	for (const value of [null, undefined, '', '  ', NaN, Infinity, -1, false, {}]) assert.equal(metrics.overviewRate(value), -1)
	assert.equal(metrics.overviewRate('12.75'), 12.75)
	assert.equal(metrics.overviewRate(7.5), 7.5)
	assert.equal(metrics.overviewRate(0), 0)
	assert.equal(metrics.deferredQueueCount('9'), 9)
	assert.equal(metrics.deferredQueueCount('0'), 0)
	assert.equal(metrics.deferredQueueCount(null), -1)
	assert.equal(metrics.deferredQueueCount(1.5), -1)
})

test('sender grant dropdown refreshes every opening and keeps its selected mailbox', async () => {
	let rows = [mailbox('old@example.org')]
	const reads = []
	const Remote = remote(async (action, params) => {
		assert.equal(action, 'list')
		reads.push(params)
		return { list: params.data?.ids ? rows.filter(r => params.data.ids.includes(r.username)) : rows, total: rows.length }
	})
	const view = mount(Remote, Vue.reactive({ type: 'senders', modelValue: 'old@example.org' }))
	try {
		await settle()
		view.get('select').props['onUpdate:show'](true)
		await settle()
		assert.equal(view.get('select').props.options.length, 1)
		view.get('select').props['onUpdate:show'](false)
		rows = [...rows, mailbox('new@example.org')]
		view.get('select').props['onUpdate:show'](true)
		await settle()
		assert.deepEqual(view.get('select').props.options.map(o => o.value), ['old@example.org', 'new@example.org'])
		assert.equal(reads.filter(p => p.page_size === 20).length, 2)
	} finally { view.app.unmount() }
})

test('sender dropdown paginates and retries a failed page without skipping newly added mailboxes', async () => {
	const rows = Array.from({ length: 45 }, (_, i) => mailbox(`mailbox-${i}@example.org`))
	let failSecond = true
	const pages = []
	const Remote = remote(async (_, params) => {
		pages.push(params.page)
		if (params.page === 2 && failSecond) { failSecond = false; throw new Error('temporary failure') }
		return { list: rows.slice((params.page - 1) * 20, params.page * 20), total: rows.length }
	})
	const view = mount(Remote, Vue.reactive({ type: 'senders', modelValue: null }))
	const scroll = () => view.get('select').props.onScroll({ target: { scrollTop: 100, clientHeight: 100, scrollHeight: 200 } })
	try {
		view.get('select').props['onUpdate:show'](true); await settle()
		scroll(); await settle()
		scroll(); await settle()
		scroll(); await settle()
		assert.deepEqual(pages, [1, 2, 2, 3])
		assert.equal(view.get('select').props.options.length, 45)
		assert.ok(view.get('select').props.options.some(o => o.value === 'mailbox-44@example.org'))
	} finally { view.app.unmount() }
})

test('changing dropdown context discards late hydration and resets old options', async () => {
	let finishOld
	const Remote = remote(async (_, params) => {
		if (params.type === 'groups') return new Promise(resolve => { finishOld = resolve })
		return { list: [mailbox('allowed@example.org')], total: 1 }
	})
	const props = Vue.reactive({ type: 'groups', modelValue: 7, groupId: 1 })
	const view = mount(Remote, props)
	try {
		await settle()
		props.type = 'senders'; props.modelValue = 'allowed@example.org'; props.groupId = 2
		await settle()
		finishOld({ list: [{ id: 7, name: 'Old department group' }], total: 1 })
		await settle()
		assert.deepEqual(view.get('select').props.options, [{ value: 'allowed@example.org', label: 'allowed@example.org' }])
	} finally { view.app.unmount() }
})

test('reopening a filtered dropdown clears the previous search', async () => {
	const queries = []
	const Remote = remote(async (_, params) => { queries.push(params.keyword); return { list: [], total: 0 } })
	const view = mount(Remote, Vue.reactive({ type: 'senders', modelValue: null }))
	try {
		view.get('select').props['onUpdate:show'](true); await settle()
		view.get('select').props.onSearch('old-mailbox')
		await new Promise(resolve => setTimeout(resolve, 320)); await settle()
		view.get('select').props['onUpdate:show'](false)
		view.get('select').props['onUpdate:show'](true); await settle()
		assert.deepEqual(queries, ['', 'old-mailbox', ''])
	} finally { view.app.unmount() }
})

test('email preview adds viewport/security policy while preserving authored styles and paragraphs', () => {
	const html = '<!DOCTYPE html><html><head><style>body{background:#123456}</style></head><body><table><tr><td>Business facts</td></tr></table></body></html>'
	const result = preview.emailPreviewDocument(html)
	assert.equal((result.match(/<html\b/gi) || []).length, 1)
	assert.ok(result.indexOf('Content-Security-Policy') < result.indexOf('body{background:#123456}'))
	assert.match(result, /name="viewport"/)
	assert.match(result, /<table><tr><td>Business facts/)
	assert.match(preview.emailPreviewDocument('Line one\nLine two\n\nParagraph two'), /Line one<br>Line two/)
	assert.ok(!preview.emailPreviewDocument('```html\n<table><tr><td>Demo</td></tr></table>\n```').includes('```'))
	assert.equal(preview.previewLength(580), '580px')
	assert.equal(preview.previewLength('calc(100vh - 220px)'), 'calc(100vh - 220px)')
})

test('actual preview iframe uses CSS lengths, responds to width changes and forbids scripts', async () => {
	const Preview = component('components/base/bt-preview/index.vue', { '@/utils/email-preview': preview })
	const props = Vue.reactive({ value: '<p>Demo</p>', width: 375, height: 'calc(100vh - 220px)' })
	const view = mount(Preview, props)
	try {
		const frame = view.get('iframe')
		assert.equal(frame.props.style.width, '375px')
		assert.equal(frame.props.style.height, 'calc(100vh - 220px)')
		assert.equal(frame.props.style.maxWidth, '100%')
		assert.ok(!frame.props.sandbox.includes('allow-scripts'))
		props.width = '100%'; props.value = '<table><tr><td>Updated template</td></tr></table>'
		await settle()
		assert.equal(view.get('iframe').props.style.width, '100%')
		assert.ok(view.get('iframe').props.srcdoc.includes('Updated template'))
	} finally { view.app.unmount() }
})

test('real browser renders preview at desktop and narrow widths without executing template scripts', async t => {
	const playwright = path.resolve(__dirname, '../../../.tools/social-browser-runtime/node_modules/playwright-core')
	const edge = 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
	if (!fs.existsSync(playwright) || !fs.existsSync(edge)) { t.skip('local Playwright/Edge not installed'); return }
	const { chromium } = require(playwright)
	const { renderToString } = require('vue/server-renderer')
	const Preview = component('components/base/bt-preview/index.vue', { '@/utils/email-preview': preview })
	const app = Vue.createSSRApp(Preview, { width: 375, height: 'min(640px, calc(100vh - 220px))', value: '<style>.sample{color:rgb(11,22,33)}</style><table><tr><td class="sample">Business facts</td></tr></table><script>parent.previewScriptRan=true</script>' })
	app.config.globalProperties.$t = key => key
	const html = await renderToString(app)
	const browser = await chromium.launch({ executablePath: edge, headless: true })
	try {
		const page = await browser.newPage()
		await page.route('**/*', route => route.abort())
		for (const width of [1200, 320]) {
			await page.setViewportSize({ width, height: 900 })
			await page.setContent(`<div style="width:min(800px,100%);">${html}</div>`)
			await page.frameLocator('iframe').getByText('Business facts').waitFor()
			const bounds = await page.locator('iframe').boundingBox()
			assert.ok(bounds.width <= width && bounds.height > 150 && bounds.height <= 900)
			assert.equal(await page.frameLocator('iframe').locator('.sample').evaluate(el => getComputedStyle(el).color), 'rgb(11, 22, 33)')
			assert.equal(await page.evaluate(() => window.previewScriptRan), undefined)
		}
	} finally { await browser.close() }
})
