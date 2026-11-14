import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import {
	metricValue,
	metricSummary,
	mergeOptions,
	socialDate,
	syncNotice,
	reportMetricKeys,
} from '../src/features/social/display.ts'
import { routeAllowed } from '../src/features/growth/access.ts'
test('missing platform metrics stay unavailable while true zero remains zero', () => {
	assert.equal(
		metricValue({ counts: { comments: 0 }, availability: { comments: 'available' } }, 'comments'),
		0
	)
	assert.equal(
		metricValue(
			{ counts: { comments: 0 }, availability: { comments: 'no_permission' } },
			'comments'
		),
		null
	)
	assert.equal(metricValue({ counts: '{}', availability: '{}' }, 'comments'), null)
	assert.deepEqual(
		metricSummary(
			[
				{ counts: { comments: 2 }, availability: { comments: 'available' } },
				{ counts: {}, availability: {} },
			],
			'comments'
		),
		{ value: 2, available: 1, total: 2 }
	)
})
test('calendar and timestamp labels preserve UTC and explicit offsets', () => {
	assert.equal(socialDate('2027-01-10 14:00:00').toISOString(), '2027-01-10T14:00:00.000Z')
	assert.equal(socialDate('2027-01-10T09:00:00-05:00').toISOString(), '2027-01-10T14:00:00.000Z')
})
test('social menu respects existing role permissions', () => {
	assert.equal(routeAllowed({ roles: [] }, '/social'), false)
	assert.equal(routeAllowed({ roles: [{ permissions: ['social:plan:view'] }] }, '/social'), true)
	assert.equal(
		routeAllowed({ roles: [{ permissions: ['social:interaction:view'] }] }, '/social'),
		true
	)
	assert.equal(routeAllowed({ roles: [{ permissions: ['crm:list'] }] }, '/social'), false)
})
test('remote options retain previous selections and update labels', () => {
	assert.deepEqual(
		mergeOptions(
			[{ value: 'id-a', label: 'A' }],
			[
				{ value: 'id-b', label: 'B' },
				{ value: 'id-a', label: 'Updated A' },
			]
		),
		[
			{ value: 'id-a', label: 'Updated A' },
			{ value: 'id-b', label: 'B' },
		]
	)
})
test('social UI literal translations exist in all supported languages', () => {
	const files = [
		'../src/views/social/index.vue',
		...fs
			.readdirSync(new URL('../src/features/social/', import.meta.url))
			.filter(file => file.endsWith('.vue'))
			.map(file => '../src/features/social/' + file),
	]
	const source = files
		.map(file => fs.readFileSync(new URL(file, import.meta.url), 'utf8'))
		.join('\n')
	const keys = [...source.matchAll(/t\('(social\.[a-z_]+)'\s*[,)]/g)].map(m => m[1])
	for (const language of ['zh', 'en', 'ja', 'de', 'tr', 'vi']) {
		const messages = JSON.parse(
			fs.readFileSync(new URL(`../src/i18n/lang/${language}.json`, import.meta.url), 'utf8')
		)
		for (const key of keys) {
			assert.equal(
				typeof key.split('.').reduce((a, k) => a?.[k], messages),
				'string',
				`${language}:${key}`
			)
		}
		for (const key of ['target_page', 'target_person', 'mode_automatic', 'mode_manual'])
			assert.equal(typeof messages.social[key], 'string', `${language}:${key}`)
		for (const key of ['pending', 'confirmed', 'expired', 'manual_due', 'manual_confirmed'])
			assert.equal(typeof messages.social.states[key], 'string', `${language}:states.${key}`)
	}
})
test('manual declarations never become automated platform metrics even if counts are present', () => {
	const row = {
		publish_mode: 'manual',
		counts: { comments: 12 },
		availability: { comments: 'available' },
	}
	assert.equal(metricValue(row, 'comments'), null)
	assert.deepEqual(
		metricSummary(
			[
				row,
				{
					publish_mode: 'automatic',
					counts: { comments: 0 },
					availability: { comments: 'available' },
				},
			],
			'comments'
		),
		{ value: 0, available: 1, total: 2 }
	)
})

test('unreadable posts are shown as per-post sync issues and never as zero metrics', () => {
	assert.deepEqual(syncNotice('POST_SYNC_UNAVAILABLE:1'), {
		key: 'social.sync_partial',
		values: { count: 1 },
	})
	assert.deepEqual(syncNotice('FACEBOOK_POST_UNAVAILABLE:10/0'), {
		key: 'social.post_unavailable_hint',
		values: { code: '10/0' },
	})
	assert.equal(syncNotice('provider detail'), null)
	assert.equal(
		metricValue(
			{ counts: { comments: 3 }, availability: { comments: 'object_unavailable' } },
			'comments'
		),
		null
	)
})

test('Facebook views and LinkedIn impressions stay separate in aggregate statistics', () => {
	assert.ok(reportMetricKeys('facebook', []).includes('views'))
	assert.ok(!reportMetricKeys('facebook', []).includes('impressions'))
	assert.ok(reportMetricKeys('linkedin', []).includes('impressions'))
	assert.ok(!reportMetricKeys('linkedin', []).includes('views'))
	assert.ok(
		reportMetricKeys(null, [{ platform: 'facebook' }, { platform: 'linkedin' }]).includes('views')
	)
	assert.ok(
		reportMetricKeys(null, [{ platform: 'facebook' }, { platform: 'linkedin' }]).includes(
			'impressions'
		)
	)
	assert.equal(
		metricValue({ counts: { views: 17 }, availability: { views: 'available' } }, 'views'),
		17
	)
})
