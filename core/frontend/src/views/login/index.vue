<template>
	<n-config-provider :theme="null" :theme-overrides="loginTheme" class="auth-provider">
		<main class="auth-page">
			<section class="auth-story" aria-labelledby="story-title">
				<a class="brand" href="/login" aria-label="LeadPilot AI"><img class="brand-wordmark" :src="brandMark" alt="领客AI · LeadPilot AI" /></a>
				<div class="story-copy"><span class="eyebrow">{{ $t('ui.pf59c0e3c250c') }}</span><h1 id="story-title">{{ copy.headline }}<em>{{ copy.headlineAccent }}</em></h1><p>{{ copy.description }}</p></div>
				<div class="route-art" aria-hidden="true"><img :src="tradeRoutes" alt="" class="route-map" /><div class="letter-card"><span class="letter-icon">↗</span><div><small>{{ copy.sampleLabel }}</small><strong>{{ $t('ui.pd83a106e2797') }}</strong><span>{{ copy.sampleBody }}</span></div><i>✓</i></div><div class="route-caption"><span></span>{{ copy.routeCaption }}</div></div>
				<div class="story-bottom"><span>01 / {{ copy.step1 }}</span><span>02 / {{ copy.step2 }}</span><span>03 / {{ copy.step3 }}</span></div>
			</section>
			<section class="auth-access" aria-labelledby="login-heading">
				<header class="access-top"><a href="/roundcube/">{{ copy.mailbox }} <span aria-hidden="true">↗</span></a><n-select class="language-switch" style="min-width:150px" :value="language" :options="languageOptions" :aria-label="copy.language" @update:value="setLoginLanguage" /></header>
				<div class="auth-form-card">
					<span class="access-label"><i></i>{{ copy.access }}</span><h2 id="login-heading">{{ copy.welcome }}</h2><p class="form-intro">{{ copy.intro }}</p>
					<n-form ref="formRef" size="large" label-placement="top" :model="form" :rules="rules" :show-require-mark="false">
						<n-form-item :label="copy.username" path="username"><n-input v-model:value="form.username" :placeholder="copy.usernameHint" :input-props="{ autocomplete: 'username', autocapitalize: 'none', spellcheck: false }" @keyup.enter="handleLogin" /></n-form-item>
						<n-form-item :label="copy.password" path="password"><n-input v-model:value="form.password" type="password" show-password-on="click" :placeholder="copy.passwordHint" :input-props="{ autocomplete: 'current-password' }" @keyup="checkCaps" @keydown="checkCaps" @blur="capsLock = false" @keyup.enter="handleLogin" /></n-form-item>
						<p v-if="capsLock" class="input-notice" role="status">{{ copy.capsLock }}</p>
						<n-form-item v-if="isCode" :label="copy.captcha" path="validate_code"><div class="captcha-row"><n-input v-model:value="form.validate_code" :placeholder="copy.captcha" :input-props="{ autocomplete: 'off', spellcheck: false }" @keyup.enter="handleLogin" /><button type="button" class="captcha-image" :aria-label="copy.refreshCode" @click="getCode"><n-spin :show="codeLoading" size="small"><img :src="codeUrl" :alt="copy.refreshCode" /></n-spin></button></div></n-form-item>
						<p v-if="loginError" class="login-error" role="alert">{{ loginError }}</p>
						<n-button type="primary" size="large" block :loading="loading" :disabled="loading" class="sign-in" @click="handleLogin">{{ copy.signIn }}<span v-if="!loading" class="sign-arrow" aria-hidden="true">→</span></n-button>
					</n-form>
					<div class="account-note"><span aria-hidden="true">ⓘ</span><p>{{ copy.accountNote }}</p></div>
					<div class="mailbox-entry"><div><strong>{{ copy.justMail }}</strong><span>{{ copy.mailNote }}</span></div><a href="/roundcube/" :aria-label="copy.mailbox">↗</a></div>
				</div>
				<footer class="access-footer"><span>LeadPilot AI</span><span>{{ copy.footer }}</span></footer>
			</section>
		</main>
	</n-config-provider>
</template>
<script setup lang="ts">
import {uiText} from '@/i18n/labels'
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import i18n from '@/i18n'
import type { FormInst, FormRules } from 'naive-ui'
import { useUserStore, useGlobalStore } from '@/store'
import { getValidateCode, login } from '@/api/modules/user'
import brandMark from '@/assets/brand/leadpilot-wordmark.png'
import tradeRoutes from '@/assets/brand/trade-routes.svg'
const router = useRouter(), userStore = useUserStore(), globalStore=useGlobalStore()
const { locale, t } = useI18n()
const languageOptions=[{value:'zh',label:'简体中文'},{value:'en',label:'English'},{value:'ja',label:'日本語'},{value:'de',label:'Deutsch'},{value:'tr',label:'Türkçe'},{value:'vi',label:'Tiếng Việt'}]
function storedLanguage(){try{const value=localStorage.getItem('bestmaple.login-language');return languageOptions.some(x=>x.value===value)?value!:'zh'}catch{return 'zh'}}
const language=ref(storedLanguage())
function setLoginLanguage(value:string){language.value=value;globalStore.applyLanguage(value);locale.value=value;try{localStorage.setItem('bestmaple.login-language',value)}catch{/* Storage may be unavailable. */}}
globalStore.applyLanguage(language.value)
locale.value=language.value
const copy=computed(()=>Object.fromEntries(Object.keys((i18n.global.messages.value.en as any).admin_login).map(key=>[key,t('admin_login.'+key)])))
const formRef = ref<FormInst | null>(null)
const form = reactive({ username: '', password: '', validate_code: '', validate_code_id: '' })
const loading = ref(false), isCode = ref(false), codeLoading = ref(false), codeUrl = ref(''), capsLock = ref(false), loginError = ref('')
const rules = computed<FormRules>(() => ({ username: { required: true, message: t('login.validation.usernameRequired'), trigger: ['blur', 'input'] }, password: { required: true, message: t('login.validation.passwordRequired'), trigger: ['blur', 'input'] }, validate_code: { required: isCode.value, message: t('login.validation.captchaRequired'), trigger: ['blur', 'input'] } }))
const loginTheme = { common: { primaryColor: '#176f65', primaryColorHover: '#125d55', primaryColorPressed: '#0d4d46', borderRadius: '8px' }, Input: { color: '#fff', textColor: '#183b42', border: '1px solid #d6dedb', heightLarge: '50px', boxShadowFocus: '0 0 0 3px rgba(23,111,101,.10)' }, Button: { heightLarge: '52px', fontWeight: '600' }, Form: { labelTextColor: '#24434a', labelFontSizeTopLarge: '13px' } }
function checkCaps(event: KeyboardEvent) { capsLock.value = event.getModifierState?.('CapsLock') || false }
async function getCode() {
	codeLoading.value = true
	try { const res = await getValidateCode(); isCode.value = !!res?.mustValidateCode; codeUrl.value = res?.validateCodeBase64 || ''; form.validate_code_id = res?.validateCodeId || ''; form.validate_code = '' } catch { /* Request interceptor displays network errors. */ } finally { codeLoading.value = false }
}
async function handleLogin() {
	if (loading.value) return
	try { await formRef.value?.validate() } catch { return }
	loading.value = true; loginError.value = ''
	try {
		const res = await login({ ...form, username: form.username.trim() })
		if (!res?.token) throw new Error('Login not completed')
		globalStore.applyLanguage(language.value)
		userStore.setLoginInfo({ token: res.token, refresh_token: res.refreshToken || res.refresh_token, ttl: res.ttl })
		await router.replace('/')
	} catch { loginError.value = copy.value.failure; await getCode() } finally { loading.value = false }
}
watch(() => [form.username, form.password], () => { loginError.value = '' })
void getCode()
</script>
<style scoped src="./login.css"></style>
<style>
body:has(.auth-page) { min-width: 0 !important; }
</style>
