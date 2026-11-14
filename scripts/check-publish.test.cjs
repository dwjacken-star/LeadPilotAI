'use strict'
const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { execFileSync, spawnSync } = require('node:child_process')
const checker = path.join(__dirname, 'check-publish.cjs')

function fixture(t) {
	const cwd = fs.mkdtempSync(path.join(os.tmpdir(), 'leadpilot-publish-'))
	t.after(() => fs.rmSync(cwd, { recursive: true, force: true }))
	const git = (...args) => execFileSync('git', args, { cwd, stdio: 'pipe' })
	git('init', '-q')
	return {
		cwd, git,
		write(name, value) { fs.writeFileSync(path.join(cwd, name), value) },
		run() { return spawnSync(process.execPath, [checker], { cwd, encoding: 'utf8' }) },
	}
}

test('accepts clean source and truncated private-key placeholders', t => {
	const f = fixture(t)
	f.write('source.txt', 'No credentials here.\n-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----')
	f.git('add', 'source.txt')
	assert.equal(f.run().status, 0)
})

test('rejects force-added ignored files', t => {
	const f = fixture(t)
	f.write('.gitignore', '.env\n*.sql\n*.exe\n')
	for (const name of ['.env', 'database.sql', 'installer.exe']) {
		f.write(name, 'local only')
		f.git('add', '-f', name)
	}
	const result = f.run()
	assert.equal(result.status, 1)
	assert.match(result.stderr, /Blocked ignored file/)
})

test('rejects access tokens without printing their values', t => {
	const f = fixture(t)
	const token = ['gh', 'p_', 'A'.repeat(36)].join('')
	f.write('source.txt', token)
	f.git('add', 'source.txt')
	const result = f.run()
	assert.equal(result.status, 1)
	assert.match(result.stderr, /Potential GitHub token/)
	assert.ok(!result.stderr.includes(token))
})

test('examines staged data rather than unstaged replacement', t => {
	const f = fixture(t)
	const key = ['-----BEGIN ', 'PRIVATE KEY-----\n', 'A'.repeat(128), '\n-----END ', 'PRIVATE KEY-----'].join('')
	f.write('source.txt', key)
	f.git('add', 'source.txt')
	f.write('source.txt', 'clean working tree content')
	assert.equal(f.run().status, 1)
})

test('rejects database URLs containing passwords', t => {
	const f = fixture(t)
	f.write('source.txt', ['postgresql', '://user:TestOnlyPassword@localhost/db'].join(''))
	f.git('add', 'source.txt')
	const result = f.run()
	assert.equal(result.status, 1)
	assert.match(result.stderr, /database connection password/)
})
