const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const http = require('node:http')
const root = path.resolve(__dirname, '../../..')
const publicRoot = path.join(root, 'core/public/html')
const names = fs.readdirSync(publicRoot).filter(name => name.endsWith('.html'))

test('public pages place language controls inside the card and preserve subscription fields', () => {
	for (const name of names) {
		const html = fs.readFileSync(path.join(publicRoot, name), 'utf8')
		assert.ok(html.includes('/leadpilot-public-layout.js'), name)
		assert.ok(!html.includes('document.body.insertBefore(select,document.body.firstChild)'), name)
		for (const match of html.matchAll(/<script(?:\s[^>]*)?>([\s\S]*?)<\/script>/gi)) new vm.Script(match[1])
	}
	const legacy = fs.readFileSync(path.join(root, 'core/template/subscription_form.html'), 'utf8')
	assert.ok(legacy.includes('{{.CSRFToken}}') && legacy.includes('name="group_ids"'))
	assert.ok(!legacy.includes('cdn.tailwindcss.com'))
})

test('real browser public layouts are responsive, localized and do not alter form tokens', async t => {
	const browserModule = path.join(root, '.tools/social-browser-runtime/node_modules/playwright-core')
	const executablePath = process.env.DOC_BROWSER || 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
	if (!fs.existsSync(browserModule) || !fs.existsSync(executablePath)) { t.skip('local Playwright/Edge not installed'); return }
	const { chromium } = require(browserModule)
	const routes = [...names.map(name => '/' + name), '/legacy-form', '/legacy-success']
	const fakeJwt = 'fixture.' + Buffer.from(JSON.stringify({ email: 'reader@example.org', group_id: 1, exp: 9999999999 })).toString('base64url') + '.fixture'
	const errors = []
	let posts = 0
	const server = http.createServer((request, response) => {
		const url = new URL(request.url, 'http://localhost')
		if (url.pathname.startsWith('/api/')) {
			posts++; response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify({ success: true, data: [{ id: 1, name: 'Demo list' }] })); return
		}
		let file, html
		if (url.pathname.endsWith('.js')) {
			file = path.join(root, 'core/frontend/public', path.basename(url.pathname))
			response.setHeader('Content-Type', 'application/javascript'); response.end(fs.readFileSync(file)); return
		}
		if (url.pathname === '/legacy-form' || url.pathname === '/legacy-success') {
			file = path.join(root, 'core/template', url.pathname === '/legacy-form' ? 'subscription_form.html' : 'subscription_success.html')
			html = fs.readFileSync(file, 'utf8').replaceAll('{{.CSRFToken}}', 'fixture-csrf').replaceAll('{{.EmailAddress}}', 'reader@example.org')
			html = html.replace(/{{range[^}]*}}/g, '').replaceAll('{{end}}', '').replaceAll('{{$group.Id}}', '1').replaceAll('{{$group.Name}}', 'Demo list')
		} else html = fs.readFileSync(path.join(publicRoot, path.basename(url.pathname)), 'utf8')
		response.setHeader('Content-Type', 'text/html; charset=utf-8'); response.end(html)
	})
	await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
	const origin = 'http://127.0.0.1:' + server.address().port
	const browser = await chromium.launch({ executablePath, headless: true })
	try {
		for (const language of ['en', 'zh']) {
			const context = await browser.newContext()
			await context.addInitScript(language => localStorage.setItem('bestmaple.login-language', language), language)
			await context.route('**/*', route => route.request().url().startsWith(origin) ? route.continue() : route.abort())
			const page = await context.newPage(); page.on('pageerror', error => errors.push(error.message)); page.on('dialog', dialog => dialog.dismiss())
			for (const width of [1440, 390, 320]) {
				await page.setViewportSize({ width, height: 900 })
				for (const route of routes) {
					const query = new URLSearchParams({ token: 'group_fixture', jwt: fakeJwt, email: 'reader@example.org', url_type: origin + '/api/groups', url_unsubscribe: origin + '/api/unsubscribe' })
					await page.goto(origin + route + '?' + query, { waitUntil: 'networkidle' })
					await page.locator('#leadpilot-public-language select').waitFor()
					assert.equal(await page.locator('#leadpilot-public-language').count(), 1)
					assert.equal(await page.locator('.brand #leadpilot-public-language').count(), 1)
					const metrics = await page.evaluate(() => ({ viewport: innerWidth, scroll: document.documentElement.scrollWidth, brand: document.querySelector('.brand').getBoundingClientRect().toJSON(), control: document.querySelector('#leadpilot-public-language').getBoundingClientRect().toJSON() }))
					assert.ok(metrics.scroll <= metrics.viewport + 1, route + ' horizontal overflow at ' + width)
					assert.ok(metrics.control.x >= metrics.brand.x - 1 && metrics.control.right <= metrics.brand.right + 1, route + ' misplaced language control')
					assert.equal(await page.locator('.brand-name').innerText(), language === 'zh' ? '领客AI' : 'LeadPilot AI')
					assert.equal(await page.locator('.brand-caption').innerText(), language === 'zh' ? '领客智邮' : 'LeadPilot Smart Mail')
					assert.ok(!(await page.locator('body').innerText()).includes('BESTMAPLE'), route + ' old visible brand remains')
					const token = await page.locator('input[name="token"]').evaluateAll(inputs => inputs.map(input => input.value))
					await page.locator('#leadpilot-public-language select').selectOption(language === 'zh' ? 'en' : 'zh')
					assert.deepEqual(await page.locator('input[name="token"]').evaluateAll(inputs => inputs.map(input => input.value)), token)
					assert.equal(await page.locator('input[name="email"]').evaluateAll(inputs => inputs.some(input => input.placeholder.includes("{'@'}"))), false)
				}
			}
			await context.close()
		}
		assert.deepEqual(errors, [])
		assert.equal(posts, 6, 'only fake list reads are expected; no subscription/unsubscription submission')
	} finally { await browser.close(); await new Promise(resolve => server.close(resolve)) }
})
