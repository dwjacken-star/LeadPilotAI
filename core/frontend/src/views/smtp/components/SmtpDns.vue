<template>
	<modal :title="$t('ui.p20d3a1251626')" :width="750" :footer="false">
		<div class="record-desc">
			{{ $t('ui.p5658008809a0') }}
		</div>
		<n-table size="small">
			<thead>
				<tr>
					<th width="140">{{ $t('domain.dns.table.recordType') }}</th>
					<th width="160">{{ $t('domain.dns.table.host') }}</th>
					<th>{{ $t('domain.dns.table.value') }}</th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="item of list">
					<td>{{ $t('ui.p7475c2bd1230') }}</td>
					<td>{{ item.host }}</td>
					<td>
						<n-ellipsis class="mr-4px max-w-320px!">{{ item.value }}</n-ellipsis>
						<n-button text type="primary" @click="() => copyText(item.value)">
							{{ $t('common.actions.copy') }}
						</n-button>
					</td>
				</tr>
			</tbody>
		</n-table>
	</modal>
</template>

<script lang="ts" setup>
import { useCopy } from '@/hooks/useCopy'
import { useModal } from '@/hooks/modal/useModal'
import { SmtpService } from '../types/base'

const { copyText } = useCopy()

const list = ref<
	Array<{
		type: string
		host: string
		value: string
	}>
>([])

const [Modal, modalApi] = useModal({
	onChangeState: isOpen => {
		if (isOpen) {
			const { smtp } = modalApi.getState<{ smtp: SmtpService }>()
			if (smtp) {
				list.value = smtp.spf_records.filter(item => item.check === 1).map(item => item.dns_record)
			}
			// const state = modalApi.getState<{ row: MailDomain | null }>()
			// const { row } = state
			// if (row) {
			// 	statusData.value = [row]
			// }
		}
	},
})
</script>

<style lang="scss" scoped>
.record-desc {
	margin-bottom: 10px;
	font-size: 12px;
	color: var(--color-text-2);
}
</style>
