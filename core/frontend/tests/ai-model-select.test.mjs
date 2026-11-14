import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import * as Vue from 'vue'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

function setup(api, initialValue = '', kind = 'text') {
	const { descriptor } = parse(
		readFileSync(new URL('../src/features/ai/AIModelSelect.vue', import.meta.url), 'utf8')
	)
	const script = compileScript(descriptor, { id: 'ai-model-test' })
	const code = ts.transpileModule(script.content, {
		compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
	}).outputText
	const props = Vue.reactive({ value: initialValue, kind, disabled: false }),
		mounted = [],
		exposed = {},
		unmounted = []
	const module = { exports: {} }
	vm.runInNewContext(code, {
		module,
		exports: module.exports,
		require(name) {
			if (name === 'vue')
				return {
					...Vue,
					onMounted: callback => mounted.push(callback),
					onBeforeUnmount: callback => unmounted.push(callback),
					useModel: (props, key) =>
						Vue.computed({ get: () => props[key], set: value => (props[key] = value) }),
				}
			if (name === 'vue-i18n') return { useI18n: () => ({ t: key => key }) }
			if (name === '@/features/growth/api') return { growth: api }
			throw Error(`Unexpected import: ${name}`)
		},
	})
	const scope = Vue.effectScope()
	const state = scope.run(() =>
		module.exports.default.setup(props, { expose: value => Object.assign(exposed, value) })
	)
	return {
		state,
		props,
		reload: exposed.reload,
		async mount() {
			mounted[0]()
			await new Promise(resolve => setImmediate(resolve))
		},
		stop() {
			unmounted.forEach(callback => callback())
			scope.stop()
		},
	}
}

function option(provider, model, disabled = false) {
	return {
		value: `ai::${provider}::${model}`,
		provider_id: provider,
		provider_name: provider,
		model_id: model,
		model_name: model,
		disabled,
	}
}

test('all consumers see enabled catalog models only and use its Qwen default', async t => {
	const qwen = option('gateway', 'qwen3'),
		openai = option('openai', 'gpt-4o'),
		disabled = option('disabled-provider', 'disabled', true),
		allOnly = option('all-only-provider', 'inactive')
	let catalog = {
		models: [openai, qwen, disabled],
		all_models: [openai, qwen, disabled, allOnly],
		model: qwen.value,
	}
	const calls = []
	const component = setup(async (action, input) => {
		calls.push([action, input.data.kind])
		return catalog
	})
	t.after(component.stop)
	await component.mount()
	assert.equal(component.props.value, qwen.value)
	assert.deepEqual(Array.from(component.state.platforms.value, p => p.value), ['openai', 'gateway'])
	assert.deepEqual(Array.from(component.state.available.value, m => m.value), [qwen.value])
	assert.deepEqual(calls, [['model_catalog', 'text']])
	component.state.choosePlatform('openai')
	assert.equal(component.props.value, openai.value)
	await component.reload()
	assert.equal(component.props.value, openai.value, 'refresh must preserve an explicit choice')
	catalog = { models: [qwen], model: qwen.value, all_models: [openai, qwen] }
	await component.reload()
	assert.deepEqual(Array.from(component.state.platforms.value, p => p.value), ['gateway'])
})

test('empty catalogs never revive all_models or select an unavailable default', async t => {
	const inactive = option('disabled-provider', 'qwen3')
	const component = setup(async () => ({
		models: [],
		all_models: [inactive],
		model: inactive.value,
	}))
	t.after(component.stop)
	await component.mount()
	assert.equal(component.props.value, '')
	assert.equal(component.state.platforms.value.length, 0)
	assert.equal(component.state.available.value.length, 0)
})

test('legacy choices resolve without being replaced by the default', async t => {
	const qwen = option('text_qwen', 'qwen3'),
		openai = option('text_openai', 'gpt-4o')
	const component = setup(
		async () => ({
			models: [qwen, openai],
			model: qwen.value,
			legacy_aliases: { 'openai::gpt-4o': openai.value },
		}),
		'openai::gpt-4o'
	)
	t.after(component.stop)
	await component.mount()
	assert.equal(component.props.value, 'openai::gpt-4o')
	assert.equal(component.state.platform.value, 'text_openai')
})

test('late catalog responses cannot overwrite a newer capability request', async t => {
	const requests = [],
		component = setup(
			(action, { data }) =>
				new Promise(resolve => requests.push({ kind: data.kind, resolve }))
		)
	t.after(component.stop)
	const first = component.mount()
	component.props.kind = 'image'
	await Vue.nextTick()
	assert.deepEqual(requests.map(r => r.kind), ['text', 'image'])
	const image = option('qwen', 'qwen-image')
	requests[1].resolve({ models: [image], model: image.value })
	await new Promise(resolve => setImmediate(resolve))
	requests[0].resolve({ models: [option('other', 'text')], model: 'ai::other::text' })
	await first
	assert.equal(component.props.value, image.value)
	assert.deepEqual(Array.from(component.state.available.value, m => m.value), [image.value])
})
