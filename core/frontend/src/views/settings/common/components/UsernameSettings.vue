<template>
	<n-form-item ref="formItemRef" :label="$t('settings.common.security.username')" path="username">
		<div class="flex-1 grid grid-cols-1 md:grid-cols-2 gap-4">
			<div>
				<n-input :value="currentUsername" disabled />
				<p class="mt-1 text-xs text-desc">
					{{ $t('settings.common.security.currentUsername') }}
				</p>
			</div>
			<div>
				<n-input
					v-model:value="securityForm.username"
					:placeholder="$t('settings.common.security.newUsernamePlaceholder')">
				</n-input>
				<p class="mt-1 text-xs text-desc">
					{{ $t('settings.common.security.usernameRule') }}
				</p>
			</div>
		</div>
	</n-form-item>
	<div class="mt-4px">
		<n-button type="primary" :loading="saving" :disabled="!securityForm.username || !securityForm.currentPassword" @click="handleSubmitUsername">
			{{ $t('settings.common.confirm') }}
		</n-button>
	</div>
</template>

<script lang="ts" setup>
import { setSystemConfigKey } from '@/api/modules/settings/common'
import { getSettingsStore } from '@/views/settings/common/store'
import { FormRules } from 'naive-ui'
import { credentialChangeComplete } from '../credentials'

const { t } = useI18n()

const { currentUsername, securityForm } = getSettingsStore()

const formItemRef = useTemplateRef('formItemRef')
const saving = ref(false)

const rules: FormRules = {
	username: {
		trigger: ['blur', 'input'],
		validator: () => {
			const { username } = securityForm.value
			if (username.length < 4 || username.length > 20) {
				return new Error(t('settings.common.security.usernameValidation.lengthError'))
			}
			if (!/^[a-zA-Z0-9_]+$/.test(username)) {
				return new Error(t('settings.common.security.usernameValidation.formatError'))
			}
			if (username === currentUsername.value) {
				return new Error(t('settings.common.security.usernameValidation.duplicateError'))
			}
			return true
		},
	},
}

const handleSubmitUsername = async () => {
	if (saving.value || !securityForm.value.currentPassword) return
	await formItemRef.value?.validate()
	saving.value = true
	try {
		await setSystemConfigKey({ key: 'admin_username', value: securityForm.value.username, current_password: securityForm.value.currentPassword })
		currentUsername.value = securityForm.value.username
		securityForm.value.username = ''; securityForm.value.currentPassword = ''
		await credentialChangeComplete()
	} finally { saving.value = false }
}

defineExpose({
	rules,
})
</script>
