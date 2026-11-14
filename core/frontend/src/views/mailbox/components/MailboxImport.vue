<template>
	<bt-modal
		:show="workbenchActive && show"
		@update:show="show = $event"
		:title="$t('mailbox.form.importTitle')"
		:width="480"
		@confirm="onConfirm">
		<div class="py-16px">
			<bt-file-upload
				:key="uploadKey"
				:is-upload="false"
				:accept="['xlsx']"
				@change="handleChangeFile">
			</bt-file-upload>
			<p class="mt-12px text-desc">{{ $t('repair.mailbox_excel_hint') }}</p>
			<n-button text type="primary" :loading="downloading" @click="downloadTemplate">{{
				$t('repair.download_excel')
			}}</n-button>
			<n-form-item :label="$t('repair.filter_keywords')"
				><n-input
					v-model:value="filterText"
					type="textarea"
					:rows="3"
					:placeholder="$t('repair.filter_hint')"
			/></n-form-item>
		</div>
	</bt-modal>
</template>

<script lang="ts" setup>
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { UploadFileInfo } from 'naive-ui'
import { Message } from '@/utils'
import { importMailbox } from '@/api/modules/mailbox'
import { downloadFile } from '@/api/modules/public'
import { readUpload } from '@/utils/response'

const emit = defineEmits<{
	refresh: []
}>()

const { t } = useI18n()

const filterText = ref('')
const show = ref(false)
const uploadKey = ref(0),
	reading = ref(false),
	downloading = ref(false)
const downloadTemplate = async () => {
	downloading.value = true
	try {
		await downloadFile({ file_path: './template/mailboxes.xlsx' })
	} catch {
		/* Request interceptor displays the error. */
	} finally {
		downloading.value = false
	}
}

const form = reactive({
	file_data: '',
	file_type: '',
})

const handleChangeFile = async (file: UploadFileInfo) => {
	form.file_data = ''
	form.file_type = ''
	if (!file.file) return
	if (!file.name.toLowerCase().endsWith('.xlsx')) {
		Message.error(t('repair.xlsx_required'))
		return
	}
	reading.value = true
	try {
		form.file_data = await readUpload(file.file, true)
		form.file_type = 'xlsx'
	} catch {
		Message.error(t('repair.file_read_failed'))
	} finally {
		reading.value = false
	}
}

const onConfirm = async () => {
	if (reading.value || !form.file_data || !form.file_type) {
		Message.error(t('mailbox.validation.fileRequired'))
		return false
	}
	const result: any = await importMailbox({
		...toRaw(form),
		filter_keywords: filterText.value
			.split(/[\n,，;；]+/)
			.map(s => s.trim())
			.filter(Boolean),
	})
	if (result?.filtered_count)
		Message.info(t('repair.filter_result', { count: result.filtered_count }))
	emit('refresh')
}

const resetForm = () => {
	filterText.value = ''
	form.file_data = ''
	form.file_type = ''
	uploadKey.value++
}

const open = () => {
	resetForm()
	show.value = true
}

defineExpose({
	open,
})
</script>

<style lang="scss" scoped></style>
