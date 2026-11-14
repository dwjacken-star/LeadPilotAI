import test from 'node:test'
import assert from 'node:assert/strict'
import {
	applyScheduleDate,
	singleFlight,
	movePostImage,
	selectPostCandidate,
	canRunSocialNow,
	canProcessPostImages,
} from '../src/features/social/composer.ts'
test('run now is limited to eligible plan states', () => {
	for (const status of ['draft', 'paused', 'scheduled']) assert.equal(canRunSocialNow(status), true)
	for (const status of ['generating', 'completed', 'archived', 'unknown', ''])
		assert.equal(canRunSocialNow(status), false)
})
test('image generation requires a prompt without source images and generated candidates need explicit selection', () => {
	assert.equal(canProcessPostImages([], ''), false)
	assert.equal(canProcessPostImages([], ' \n '), false)
	assert.equal(canProcessPostImages([], 'Product on a neutral background'), true)
	assert.equal(canProcessPostImages(['task:1'], ''), true)
	const selected = []
	assert.deepEqual(selectPostCandidate(selected, '', 'task:99'), ['task:99'])
	assert.deepEqual(selected, [])
})
test('editing the start date preserves all market-specific clocks', () => {
	const config = {
		start_date: '2027-01-10',
		markets: [{ local_time: '09:20:00' }, { local_time: '11:22:33' }],
	}
	applyScheduleDate(config, '2027-02-11')
	assert.equal(config.start_date, '2027-02-11')
	assert.deepEqual(
		config.markets.map(m => m.local_time),
		['09:20:00', '11:22:33']
	)
	applyScheduleDate(config, null)
	applyScheduleDate(config, '2027-02-12 11:12:13')
	assert.equal(config.start_date, '2027-02-11')
})
test('rapid draft saves share the same result and failed saves can be retried', async () => {
	let calls = 0,
		finish
	const save = singleFlight(async () => {
		calls++
		return new Promise(resolve => (finish = resolve))
	})
	const first = save(),
		second = save()
	assert.equal(first, second)
	assert.equal(calls, 1)
	finish(42)
	assert.equal(await second, 42)
	const third = save()
	assert.equal(calls, 2)
	finish(43)
	assert.equal(await third, 43)
	let attempts = 0
	const retry = singleFlight(async () => {
		attempts++
		if (attempts === 1) throw Error('network')
		return 44
	})
	await assert.rejects(retry())
	assert.equal(await retry(), 44)
})
test('photo ordering and explicit AI result selection preserve other photos and originals', () => {
	const originals = ['task:first', 'task:second', 'task:third']
	assert.deepEqual(movePostImage(originals, 1, -1), ['task:second', 'task:first', 'task:third'])
	assert.deepEqual(movePostImage(originals, 0, -1), originals)
	assert.deepEqual(selectPostCandidate(originals, 'task:second', 'task:ai-result'), [
		'task:first',
		'task:ai-result',
		'task:third',
	])
	assert.deepEqual(originals, ['task:first', 'task:second', 'task:third'])
	const full = Array.from({ length: 10 }, (_, i) => 'task:' + i)
	assert.deepEqual(selectPostCandidate(full, 'unselected', 'new'), full)
	assert.deepEqual(selectPostCandidate(originals, 'task:second', 'task:first'), originals)
})
