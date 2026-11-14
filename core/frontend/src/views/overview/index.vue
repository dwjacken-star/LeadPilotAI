<template>
	<div class="email-analytics-container">
		<div class="bt-title">{{ t('layout.menu.overview') }}</div>
		<!-- Top Filter Area -->
		<filter-bar
			v-model:domain="domain"
			v-model:date-range="dateRange"
			@update:domain="handleDataUpdate"
			@update:date-range="handleDataUpdate">
		</filter-bar>

		<n-alert type="info" :show-icon="false" class="mb-4"
			>{{ t('overview.trackingNotice') }}<br />{{
				t('overview.trackingCoverage', coverage)
			}}<br />{{ t('overview.clickTrackingCoverage', coverage) }}</n-alert
		>
		<n-alert v-if="queueAccess && delayedQueue < 0" type="warning" class="mb-4">
			{{ t('overview.queueUnavailable') }}
		</n-alert>
		<!-- Main Metric Cards -->
		<div class="metrics-cards">
			<metric-card
				v-for="(item, key) in rateData"
				:key="key"
				:title="item.label"
				:value="item.value < 0 ? '—' : item.value"
				:unit="item.value < 0 ? '' : item.unit">
			</metric-card>
			<metric-card
				v-if="queueAccess"
				class="cursor-pointer"
				:title="$t('overview.delayedQueue')"
				:value="delayedQueue < 0 ? '—' : delayedQueue"
				:text-color="delayedQueue > 0 ? theme.warningColor : ''"
				@click="onClickDelayedQueue">
			</metric-card>
		</div>

		<!-- Data Details Area -->
		<div class="detail-row">
			<!-- Left: Mail Provider Table -->
			<n-card class="provider-table-card" :title="$t('overview.mailProviders')">
				<provider-table v-model:value="providers" />
			</n-card>

			<!-- Right: Today's Sending Statistics -->
			<n-card class="send-today-card" :title="$t('overview.sendStats')">
				<send-today-stats :data="sendMail" @show-fail="handleShowFail" />
			</n-card>
		</div>

		<!-- Bottom Rate Charts Area -->
		<div class="rate-charts-card">
			<rate-chart-panel :bounce="bounceRate" :click="clickRate" :open="openRate" />
		</div>

		<fail-modal />
	</div>
</template>

<script lang="ts" setup>
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { useDebounceFn } from '@vueuse/core'
import { useThemeVars } from 'naive-ui'
import { getDayTimeRange, isArray, isObject } from '@/utils'
import { useModal } from '@/hooks/modal/useModal'
import { getOverviewInfo } from '@/api/modules/overview'
import type { MailOverview, MailProvider, RateData } from './types'
import { overviewRate, deferredQueueCount } from './metrics'

import FilterBar from './components/FilterBar.vue'
import MetricCard from './components/MetricCard.vue'
import ProviderTable from './components/ProviderTable.vue'
import SendTodayStats from './components/SendTodayStats.vue'
import RateChartPanel from './components/RateChartPanel.vue'
import SendFailDetails from './components/SendFailDetails.vue'

const { t } = useI18n()

const theme = useThemeVars()

const domain = ref('')

const dateRange = ref(getDayTimeRange())

const providers = ref<MailProvider[]>([])
const coverage = reactive({ tracked: 0, clickTracked: 0, total: 0 })
let overviewSerial = 0
let overviewInFlight = 0
let refreshTimer: ReturnType<typeof setInterval> | undefined

const rateData = reactive<RateData>({
	delivery_rate: { label: t('overview.delivered'), value: -1, unit: '%' },
	open_rate: { label: t('overview.opened'), value: -1, unit: '%' },
	click_rate: { label: t('overview.clicked'), value: -1, unit: '%' },
	bounce_rate: { label: t('overview.bounced'), value: -1, unit: '%' },
})

const delayedQueue = ref(-1)
const queueAccess = ref(false)

const sendMail = ref<MailOverview['send_mail_chart']>({
	column_type: 'hourly',
	dashboard: {
		delivered: 0,
		delivery_rate: 0,
		failed: 0,
		failure_rate: 0,
		sends: 0,
	},
	data: [],
})

const bounceRate = ref<MailOverview['bounce_rate_chart']>({
	column_type: 'hourly',
	data: [],
})

const clickRate = ref<MailOverview['click_rate_chart']>({
	column_type: 'hourly',
	data: [],
})

const openRate = ref<MailOverview['open_rate_chart']>({
	column_type: 'hourly',
	data: [],
})

const router = useRouter()

const onClickDelayedQueue = () => {
	if (!queueAccess.value) return
	router.push('/settings/send-queue')
}

const [FailModal, failModalApi] = useModal({
	component: SendFailDetails,
})

const handleShowFail = () => {
	failModalApi.setState({
		domain: domain.value,
		startTime: dateRange.value[0],
		endTime: dateRange.value[1],
	})
	failModalApi.open()
}

// Function to handle data update
const handleDataUpdate = useDebounceFn(fetchOverviewData, 300)

// Function to update rate data
const updateRateData = (dashboard: MailOverview['dashboard']) => {
	Object.keys(rateData).forEach(key => {
		rateData[key].value = overviewRate(dashboard[key as keyof typeof dashboard])
	})
	delayedQueue.value = deferredQueueCount(dashboard.delayed_queue)
}

// Function to fetch overview data
async function fetchOverviewData() {
	const serial = ++overviewSerial
	overviewInFlight++
	try {
		const res = await getOverviewInfo({
			domain: domain.value,
			start_time: Math.floor(dateRange.value[0] / 1000),
			end_time: Math.floor(dateRange.value[1] / 1000),
		})

		if (serial === overviewSerial && isObject<MailOverview>(res)) {
			queueAccess.value = res.queue_access === true
			coverage.tracked = Number(res.dashboard.open_tracked) || 0
			coverage.clickTracked = Number(res.dashboard.click_tracked) || 0
			coverage.total = Number(res.dashboard.sends) || 0
			updateRateData(res.dashboard)

			providers.value = isArray(res.mail_providers) ? res.mail_providers : []
			sendMail.value = res.send_mail_chart
			bounceRate.value = res.bounce_rate_chart
			clickRate.value = res.click_rate_chart
			openRate.value = res.open_rate_chart
		}
	} finally {
		overviewInFlight--
	}
}

onMounted(() => {
	fetchOverviewData()
	refreshTimer = setInterval(() => {
		if (!workbenchActive.value) return
		if (!overviewInFlight && document.visibilityState === 'visible')
			void fetchOverviewData().catch(() => {})
	}, 30000)
})
onBeforeUnmount(() => {
	clearInterval(refreshTimer)
	overviewSerial++
})
</script>

<style lang="scss" scoped>
.email-analytics-container {
	padding: 20px;
}

.metrics-cards {
	display: flex;
	justify-content: space-between;
	gap: 20px;
	margin-bottom: 20px;
}

.detail-row {
	display: flex;
	gap: 20px;
	margin-bottom: 20px;

	.provider-table-card,
	.send-today-card {
		flex: 1;
	}
}

.rate-charts-card {
	display: flex;
	gap: 20px;
}
</style>
