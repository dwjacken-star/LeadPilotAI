<template>
	<modal :title="$t('domain.blacklistDetection.title')" :footer="false" :width="660">
		<div class="pt-8px">
			<n-alert type="default" :show-icon="false">
				<div class="text-basic text-center">
					{{ $t('ui.p97876b8380ad') }} <b>{{ result.results || '--' }}</b> {{ $t('ui.p9a64ec952084') }}
					<b>{{ result.ip || '--' }}</b> {{ $t('ui.pb51c31d9cefe') }} <b>{{ result.tested }}</b> {{ $t('ui.p84fbaa6328b3') }}
				</div>
			</n-alert>
			<div class="my-24px text-center font-bold">
				{{ t('domain.blacklistDetection.listed', { count: result.blacklisted || 0 }) }}
			</div>
			<n-alert v-if="!result.blacklisted" class="success-alert" type="success">
				<span>{{ $t('domain.blacklistDetection.success') }}</span>
			</n-alert>
			<n-data-table v-else max-height="200" :columns="columns" :data="result.black_list">
			</n-data-table>
		</div>
	</modal>
</template>

<script lang="ts" setup>
import { DataTableColumns } from 'naive-ui'
import { formatTime } from '@/utils'
import { useModal } from '@/hooks/modal/useModal'
import { DomainBlackCheckResult, MailDomain } from '../interface'

const { t } = useI18n()

const result = ref<Partial<DomainBlackCheckResult>>({})

const columns = ref<DataTableColumns<{ blacklist: string; time: number }>>([
	{
		key: 'blacklist',
		title: t('domain.blacklistDetection.columns.blacklist'),
	},
	{
		key: 'response',
		title: t('domain.blacklistDetection.columns.responseIp'),
	},
	{
		key: 'time',
		title: t('domain.blacklistDetection.columns.checkTime'),
		render: row => formatTime(row.time),
	},
])

const [Modal, modalApi] = useModal({
	onChangeState: isOpen => {
		if (isOpen) {
			const { row } = modalApi.getState<{ row: MailDomain }>()
			result.value = row.black_check_result || {}
		} else {
			result.value = {}
		}
	},
})
</script>

<style lang="scss" scoped>
.success-alert {
	display: flex;
	align-items: center;
	justify-content: center;
	:deep(.n-alert__icon) {
		position: relative;
		margin: 0;
	}
	:deep(.n-alert-body) {
		padding-left: 8px;
	}
}
</style>
