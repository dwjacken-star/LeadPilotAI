const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const source = fs.readFileSync(path.join(__dirname, '../src/features/workspace/normalize.ts'), 'utf8')
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText
const compiled = { exports: {} }
new Function('exports', code)(compiled.exports)
const { asArray, normalizePolicy, normalizeCampaign, completePermissionSelection } = compiled.exports

test('empty and malformed collections remain arrays for table and select rendering', () => {
	for (const value of [null, undefined, 'null', {}, 0]) {
		assert.deepEqual(asArray(value), [])
		assert.deepEqual(normalizePolicy(value).weekdays, [])
		assert.deepEqual(normalizeCampaign(value).tag_ids, [])
		assert.deepEqual(normalizeCampaign(value).policy.weekdays, [])
	}
})

test('old campaign JSON with null tags and weekdays is editable', () => {
	const result = normalizeCampaign({ tag_ids: null, policy: { weekdays: null, window_start: null } })
	assert.deepEqual(result.tag_ids, [])
	assert.deepEqual(result.policy.weekdays, [])
	assert.equal(result.policy.window_start, '')
})

test('saved selections and explicit unlimited quotas survive defaults', () => {
	const result = normalizeCampaign({ tag_ids: [2], policy: { weekdays: [1, 3], per_day: 0 } }, { policy: { per_day: 2000 } })
	assert.deepEqual(result.tag_ids, [2])
	assert.deepEqual(result.policy.weekdays, [1, 3])
	assert.equal(result.policy.per_day, 0)
	assert.equal(normalizePolicy(null, { per_day: 2000 }).per_day, 2000)
})

test('role selections include required views without expanding writing-only grants', () => {
	const dependencies = {
		'social:account:manage': ['social:account:view'],
		'intelligence:run': ['intelligence:view'],
	}
	assert.deepEqual(completePermissionSelection(['social:account:manage', 'intelligence:run'], dependencies),
		['social:account:manage', 'intelligence:run', 'social:account:view', 'intelligence:view'])
	assert.deepEqual(completePermissionSelection(['campaign:create'], dependencies), ['campaign:create'])
	assert.deepEqual(completePermissionSelection(['social:account:view'], dependencies), ['social:account:view'])
	assert.deepEqual(completePermissionSelection(['cycle'], { cycle: ['cycle'] }), ['cycle'])
})
