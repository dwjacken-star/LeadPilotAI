<template>
	<n-form-item
		ref="formItemRef"
		:label="$t('settings.common.security.changePassword')"
		path="password">
		<div class="flex-1 flex flex-col gap-20px">
			<div>
				<n-input
					v-model:value="securityForm.newPassword"
					type="password"
					class="password-input"
					show-password-on="click"
					:placeholder="t('settings.common.security.newPassword')"
					@update:value="handleUpdatePassword">
				</n-input>
				<password-strength-indicator :strength="passwordStrength" />
			</div>
			<div>
				<n-input
					v-model:value="securityForm.confirmPassword"
					type="password"
					class="password-input"
					show-password-on="click"
					:placeholder="t('settings.common.security.confirmPassword')">
				</n-input>
			</div>
		</div>
	</n-form-item>
	<div>
		<n-button type="primary" :loading="saving" :disabled="submitDisabled" @click="handleSubmitPassword">
			{{ t('settings.common.confirm') }}
		</n-button>
	</div>
</template>

<script lang="ts" setup>
import { FormRules } from 'naive-ui'
import { getSettingsStore } from '@/views/settings/common/store'
import { setSystemConfigKey } from '@/api/modules/settings/common'
import PasswordStrengthIndicator from './PasswordStrengthIndicator.vue'
import { credentialChangeComplete } from '../credentials'

const { t } = useI18n()
const { securityForm, passwordStrength, checkPasswordStrength } = getSettingsStore()

const formItemRef = useTemplateRef('formItemRef')
const saving = ref(false)

const rules: FormRules = {
	password: {
		trigger: ['input', 'blur'],
		validator: () => {
			if (!securityForm.value.newPassword || !securityForm.value.confirmPassword) {
				return true
			}
			if (securityForm.value.newPassword !== securityForm.value.confirmPassword) {
				return new Error(t('settings.common.security.passwordMismatch'))
			}
			if ([...securityForm.value.newPassword].length < 8 || new TextEncoder().encode(securityForm.value.newPassword).length > 72) {
				return new Error(t('accountSecurity.passwordLength'))
			}
			return true
		},
	},
}

const submitDisabled = computed(() => {
	return saving.value || !securityForm.value.currentPassword || !securityForm.value.newPassword || !securityForm.value.confirmPassword
})

const handleUpdatePassword = () => {
	checkPasswordStrength()
}

const handleSubmitPassword = async () => {
	if (submitDisabled.value) return
	await formItemRef.value?.validate()
	saving.value = true
	try {
		await setSystemConfigKey({ key: 'admin_password', value: securityForm.value.newPassword, current_password: securityForm.value.currentPassword })
		securityForm.value.newPassword = ''; securityForm.value.confirmPassword = ''; securityForm.value.currentPassword = ''
		await credentialChangeComplete()
	} finally { saving.value = false }
}

defineExpose({
	rules,
})
</script>
