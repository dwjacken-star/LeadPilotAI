#!/usr/bin/env node
'use strict'

// Inspect staged blobs, never print the credential values themselves.
const { execFileSync, spawnSync } = require('node:child_process')

function git(args, options = {}) {
	return execFileSync('git', args, { maxBuffer: 128 * 1024 * 1024, ...options })
}

const files = git(['diff', '--cached', '--name-only', '--diff-filter=ACMR', '-z'])
	.toString('utf8').split('\0').filter(Boolean)
if (files.length === 0) {
	console.log('Publish check: no staged files.')
	process.exit(0)
}

const ignored = spawnSync('git', ['check-ignore', '--no-index', '--stdin', '-z'], {
	input: Buffer.from(files.join('\0') + '\0'),
	maxBuffer: 16 * 1024 * 1024,
})
if (ignored.error) throw ignored.error
if (ignored.status !== 0 && ignored.status !== 1) {
	process.stderr.write(ignored.stderr)
	process.exit(1)
}
const violations = ignored.stdout.toString('utf8').split('\0').filter(Boolean)
for (const file of violations) console.error('Blocked ignored file: ' + file)

const rules = [
	['private key', /-----BEGIN (?:[A-Z ]*PRIVATE KEY)-----\s*[A-Za-z0-9+/=\s]{80,}-----END (?:[A-Z ]*PRIVATE KEY)-----/],
	['GitHub token', /\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})\b/],
	['AWS access key', /\b(?:AKIA|ASIA)[A-Z0-9]{16}\b/],
	['AI API key', /\bsk-(?:proj-|ant-)?[A-Za-z0-9_-]{24,}\b/],
	['Slack token', /\bxox[baprs]-[A-Za-z0-9-]{20,}\b/],
	['Google API key', /\bAIza[A-Za-z0-9_-]{35}\b/],
	['Meta access token', /\bEAA[A-Za-z0-9]{70,}\b/],
	['database connection password', /\b(?:postgres(?:ql)?|mysql|redis):\/\/[^\s:'"/]+:[^\s@'"$<>]+@/],
]
// Read every staged blob in one Git process, avoiding per-file startup costs.
if (files.some(file => /[\r\n]/.test(file))) {
	console.error('Publish check does not support filenames containing line breaks.')
	process.exit(1)
}
const staged = git(['cat-file', '--batch'], { input: files.map(file => ':' + file + '\n').join('') })
let offset = 0
let findings = violations.length
let bytes = 0
for (const file of files) {
	const end = staged.indexOf(10, offset)
	const header = staged.subarray(offset, end).toString('utf8')
	const match = /^[0-9a-f]+ blob (\d+)$/.exec(header)
	if (end < 0 || !match) throw new Error('Cannot read staged blob: ' + file)
	const length = Number(match[1])
	const blob = staged.subarray(end + 1, end + 1 + length)
	if (blob.length !== length) throw new Error('Truncated staged blob: ' + file)
	offset = end + 1 + length + 1
	bytes += blob.length
	if (blob.length > 50 * 1024 * 1024) {
		console.error('Blocked large file (>50 MiB): ' + file)
		findings++
	}
	if (blob.subarray(0, 8192).includes(0)) continue
	const text = blob.toString('utf8')
	for (const [label, pattern] of rules) {
		if (pattern.test(text)) {
			console.error('Potential ' + label + ': ' + file)
			findings++
		}
	}
}
if (findings) {
	console.error('Publish check failed. Review flagged paths before committing; no secret values were printed.')
	process.exit(1)
}
console.log('Publish check passed: ' + files.length + ' staged files, ' + bytes + ' bytes. Pattern detection is not a complete security audit.')
