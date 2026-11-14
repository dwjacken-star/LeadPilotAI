<template>
	<main class="growth-page">
		<header>
			<div>
				<h1>{{ t('growth.outreach') }}</h1>
				<p>{{ t('growth.outreach_intro') }}</p>
			</div>
			<n-space
				><n-button v-if="principal?.super" @click="openSettings">{{
					t('growth.ai_settings')
				}}</n-button
				><n-button v-if="can('outreach:manage')" type="primary" @click="editPlan()">{{
					t('growth.add_plan')
				}}</n-button></n-space
			>
		</header>
		<n-alert type="info" class="toolbar"
			>{{ t('growth.delivery_hint') }}
			<div v-if="deliveryIdentity.from" class="identity-hint">
				{{ t('growth.from_address') }}: {{ deliveryIdentity.from }} {{ $t('ui.p1c028df5f7be') }}
				{{ deliveryIdentity.reply_to || deliveryIdentity.from }}
			</div>
		</n-alert>
		<n-space class="toolbar"
			><n-select
				v-model:value="companyID"
				clearable
				:options="companyOptions"
				:placeholder="t('growth.all_companies')"
				style="width: 300px"
				@update:value="search" /><n-button :loading="loading" @click="refresh">{{
				t('growth.refresh')
			}}</n-button
			><n-button
				v-if="['leads', 'candidates'].includes(tab) && can('outreach:export')"
				@click="run(() => workbook(tab, companyID || undefined))"
				>{{ t('growth.export') }}</n-button
			><n-button v-if="tab === 'inbox_rules' && can('inquiry:manage')" @click="editRule()">{{
				t('growth.add_rule')
			}}</n-button></n-space
		>
		<div class="stats">
			<div v-for="entry in statsCards" :key="entry.key">
				<span>{{ t('growth.' + entry.key) }}</span
				><strong>{{ entry.total }}</strong>
			</div>
		</div>
		<n-tabs v-model:value="tab" @update:value="changeTab"
			><n-tab-pane v-for="value in tabs" :key="value" :name="value" :tab="t('growth.' + value)"
		/></n-tabs>
		<n-space class="toolbar"
			><n-button
				v-if="tab === 'candidates'"
				:type="status === 'with_email' ? 'primary' : 'default'"
				@click="toggleCandidateEmails"
				>{{ t('growth.show_collected_emails') }}</n-button
			><n-input
				v-model:value="keyword"
				clearable
				:placeholder="t('growth.search')"
				style="width: 270px"
				@keyup.enter="search" /><n-select
				v-if="statusOptions.length"
				v-model:value="status"
				clearable
				:options="statusOptions"
				:placeholder="t('growth.status')"
				style="width: 200px"
				@update:value="search" /><n-button @click="search">{{
				t('growth.search')
			}}</n-button></n-space
		>
		<n-data-table
			:columns="columns"
			:data="rows"
			:row-key="r => r.id"
			:loading="loading"
			:scroll-x="tableWidth" /><n-pagination
			v-model:page="page"
			:page-size="20"
			:item-count="total"
			class="pagination"
			@update:page="refresh" />
		<n-drawer
			:show="workbenchActive && planDrawer"
			@update:show="planDrawer = $event"
			width="min(900px,97vw)"
			><n-drawer-content :title="t('growth.plan')" closable>
				<n-form label-placement="top"
					><n-form-item :label="t('growth.plan_name')"
						><n-input v-model:value="planDraft.name" /></n-form-item
					><n-form-item :label="t('growth.company')"
						><n-select
							v-model:value="planDraft.company_id"
							:options="companyOptions"
							:disabled="!!planDraft.id"
							@update:value="planCompanyChanged"
					/></n-form-item>
					<n-form-item :label="t('growth.approved_products')"
						><div style="width: 100%">
							<n-select
								v-model:value="planConfig.product_ids"
								multiple
								filterable
								remote
								:disabled="!can('product:list')"
								:options="productOptions"
								:loading="productLoading"
								:placeholder="t('growth.choose_products')"
								@search="productSearch"
								@scroll="productScroll" />
							<p class="hint">{{ t('growth.products_hint') }}</p>
						</div></n-form-item
					>
					<n-form-item :label="t('growth.markets')"
						><n-dynamic-input v-model:value="planConfig.markets" :on-create="newMarket"
							><template #default="{ value }"
								><div class="market-selects">
									<n-select
										:value="value.country"
										filterable
										:options="retainChoice(choices.countries, value.country)"
										:placeholder="t('growth.country_code')"
										@update:value="countryChanged(value, $event, marketData)" /><n-select
										v-model:value="value.language"
										filterable
										:options="retainChoice(choices.languages, value.language)"
										:placeholder="t('growth.language')" /><n-select
										v-model:value="value.timezone"
										filterable
										:options="retainChoice(choices.timezones, value.timezone)"
										:placeholder="t('growth.timezone')" /><n-input
										v-model:value="value.city"
										:placeholder="t('growth.city_optional')" /></div></template></n-dynamic-input
					></n-form-item>
					<n-form-item :label="t('growth.model')"><AIModelSelect v-model:value="planConfig.model" /></n-form-item>
<n-form-item :label="t('growth.requirement')"
						><n-input
							v-model:value="planConfig.requirement"
							type="textarea"
							:rows="4" /></n-form-item
					><n-form-item :label="t('growth.intent_keywords')"
						><div style="width: 100%">
							<KeywordInput v-model="planConfig.intent_keywords" /><ExpandKeywords
								v-model="planConfig.intent_keywords"
								:seed="planConfig.requirement"
								:country="planConfig.markets?.[0]?.country"
								:language="planConfig.markets?.[0]?.language" /></div></n-form-item
					><n-form-item :label="t('growth.exclude_keywords')"
						><div style="width: 100%">
							<n-input
								v-model:value="keywordText"
								type="textarea"
								:rows="3"
								:placeholder="t('repair.filter_hint')"
								@update:value="changeKeywords" />
							<p class="hint">{{ t('repair.filter_hint') }}</p>
							<n-space
								><n-tag v-for="word in planConfig.exclude_keywords" :key="word" size="small">{{
									word
								}}</n-tag></n-space
							>
						</div></n-form-item
					>
					<n-form-item :label="t('growth.serper_search_channel')"
						><n-select v-model:value="planConfig.search_channel" :options="searchChannels"
					/></n-form-item>
					<n-form-item :label="t('growth.serper_result_limit')"
						><n-input-number v-model:value="planConfig.search_results" :min="1" :max="20" /><span
							class="hint"
							>{{ t('growth.serper_pipeline_hint') }}</span
						></n-form-item
					>
					<n-form-item :label="t('growth.sender_pool')"
						><remote-select v-model="planConfig.senders" type="senders" multiple
					/></n-form-item>
					<n-grid :cols="2" :x-gap="20"
						><n-form-item-gi :label="t('growth.schedule_timezone')"
							><n-select
								v-model:value="planConfig.timezone"
								filterable
								:options="retainChoice(choices.timezones, planConfig.timezone)"
								:placeholder="t('growth.timezone')" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.daily_time')"
							><n-time-picker
								v-model:formatted-value="planConfig.local_time"
								value-format="HH:mm"
								format="HH:mm"
								:clearable="false" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.send_start')"
							><n-time-picker
								v-model:formatted-value="planConfig.send_start"
								value-format="HH:mm"
								format="HH:mm"
								:clearable="false" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.send_end')"
							><n-time-picker
								v-model:formatted-value="planConfig.send_end"
								value-format="HH:mm"
								format="HH:mm"
								:clearable="false" /></n-form-item-gi
					></n-grid>
					<n-grid :cols="3" :x-gap="16"
						><n-form-item-gi :label="t('growth.daily_leads')"
							><n-input-number
								v-model:value="planConfig.daily_leads"
								:min="1"
								:max="1000" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.daily_send')"
							><n-input-number
								v-model:value="planConfig.daily_send"
								:min="1"
								:max="500" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.cooldown_days')"
							><n-input-number
								v-model:value="planConfig.cooldown_days"
								:min="1"
								:max="365" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.search_limit')"
							><n-input-number
								v-model:value="planConfig.search_limit"
								:min="1"
								:max="20" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.page_limit')"
							><n-input-number
								v-model:value="planConfig.page_limit"
								:min="1"
								:max="10" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.interval_seconds')"
							><n-input-number
								v-model:value="planConfig.policy.min_interval_ms"
								:min="0" /></n-form-item-gi
					></n-grid>
					<n-grid :cols="3" :x-gap="16"
						><n-form-item-gi v-for="field in policyFields" :key="field.key" :label="field.label"
							><n-input-number
								v-model:value="planConfig.policy[field.key]"
								:min="0"
								:max="field.max"
								:precision="0" /></n-form-item-gi
					></n-grid>
					<n-divider>{{ t('growth.email_delivery_settings') }}</n-divider>
					<n-grid cols="1 700:2" :x-gap="24"
						><n-form-item-gi :label="t('market.task.edit.ipWarmup')"
							><n-space align="center"
								><n-switch v-model:value="planConfig.warmup" /><span class="hint">{{
									t('market.task.edit.ipWarmupTip')
								}}</span></n-space
							></n-form-item-gi
						><n-form-item-gi :label="t('market.task.edit.unsubscribeLink')"
							><n-switch v-model:value="planConfig.unsubscribe" /></n-form-item-gi
						><n-form-item-gi :label="t('market.task.edit.trackClick')"
							><n-switch v-model:value="planConfig.track_click" /></n-form-item-gi
						><n-form-item-gi :label="t('market.task.edit.trackOpen')"
							><n-switch v-model:value="planConfig.track_open" /></n-form-item-gi
					></n-grid>
					<n-checkbox v-model:checked="planConfig.auto_send" :disabled="!can('outreach:publish')">{{
						t('growth.auto_send')
					}}</n-checkbox
					><n-alert type="warning" class="toolbar">{{ t('growth.auto_send_hint') }}</n-alert>
				</n-form>
				<n-space
					><n-button
						v-if="can('outreach:manage')"
						type="primary"
						:loading="saving"
						@click="savePlan"
						>{{ t('growth.save_draft') }}</n-button
					><n-button
						v-if="planDraft.id && can('outreach:run')"
						@click="controlPlan(planDraft, 'enable')"
						>{{ t('growth.enable_schedule') }}</n-button
					><n-popconfirm
						v-if="planDraft.id && can('outreach:run')"
						@positive-click="controlPlan(planDraft, 'run')"
						><template #trigger
							><n-button>{{ t('growth.run_now') }}</n-button></template
						>{{ t('growth.run_confirm') }}</n-popconfirm
					></n-space
				>
			</n-drawer-content></n-drawer
		>
		<n-modal
			:show="workbenchActive && ruleModal"
			@update:show="ruleModal = $event"
			preset="card"
			:title="t('growth.inbox_rule')"
			style="width: min(680px, 96vw)">
			<n-form label-placement="top"
				><n-form-item :label="t('growth.company')"
					><n-select v-model:value="ruleDraft.company_id" :options="companyOptions" /></n-form-item
				><n-form-item :label="t('growth.mailbox')"
					><remote-select v-model="ruleDraft.mailbox" type="senders" /></n-form-item
				><n-form-item :label="t('growth.forward_to')"
					><n-input
						v-model:value="ruleDraft.forward_to"
						:placeholder="t('growth.optional')" /></n-form-item
				><n-form-item :label="t('growth.model')"><AIModelSelect v-model:value="ruleDraft.model" /></n-form-item><n-checkbox v-model:checked="ruleDraft.active">{{ t('growth.enable') }}</n-checkbox
				><n-alert type="info" class="toolbar">{{ t('growth.inbox_hint') }}</n-alert></n-form
			><n-button type="primary" :loading="saving" @click="saveRule">{{
				t('growth.save')
			}}</n-button>
		</n-modal>
		<n-drawer
			:show="workbenchActive && detailDrawer"
			@update:show="detailDrawer = $event"
			width="min(1000px,97vw)"
			><n-drawer-content :title="t('growth.' + tab)" closable>
				<template v-if="detail">
					<n-descriptions :column="2" bordered
						><n-descriptions-item
							v-for="entry in detailFields"
							:key="entry.key"
							:label="entry.label"
							>{{ entry.value }}</n-descriptions-item
						></n-descriptions
					>
					<template v-if="tab === 'messages'"
						><h3>{{ detail.record.subject }}</h3>
						<bt-preview :value="detail.record.html || ''" height="500" /><n-space class="toolbar"
							><n-popconfirm
								v-if="detail.record.status === 'draft' && can('outreach:publish')"
								@positive-click="controlMessage(detail.record, 'approve')"
								><template #trigger
									><n-button type="primary">{{ t('growth.approve_send') }}</n-button></template
								>{{ t('growth.approve_confirm') }}</n-popconfirm
							><n-button
								v-if="
									['draft', 'approved', 'queued'].includes(detail.record.status) &&
									can('outreach:publish')
								"
								@click="controlMessage(detail.record, 'cancel')"
								>{{ t('growth.cancel') }}</n-button
							><n-popconfirm
								v-if="
									!['sent', 'unknown'].includes(detail.record.status) &&
									can('outreach:manage') &&
									can('outreach:run')
								"
								@positive-click="regenerateMessage(detail.record)"
								><template #trigger
									><n-button :loading="saving">{{ t('growth.regenerate') }}</n-button></template
								>{{ t('growth.regenerate_hint') }}</n-popconfirm
							></n-space
						><n-collapse
							><n-collapse-item :title="t('growth.source_snapshot')">
								<pre>{{ JSON.stringify(object(detail.record.snapshot), null, 2) }}</pre>
							</n-collapse-item></n-collapse
						></template
					>
					<template v-if="tab === 'candidates'"
						><h3>{{ t('growth.discovered_emails') }}</h3>
						<n-alert v-if="!emailDetails.length" type="info">{{
							t('growth.no_email_yet')
						}}</n-alert>
						<div v-for="entry in emailDetails" :key="entry.email" class="email-detail">
							<strong>{{ entry.email }}</strong
							><n-tag>{{ stateText(entry.status) }}</n-tag
							><n-button v-if="entry.lead_id" text type="primary" @click="openEmailLead(entry)">{{
								t('growth.view_email_lead')
							}}</n-button>
							<p>{{ emailReason(entry) }}</p>
						</div>
						<p class="hint">{{ t('growth.email_visibility_hint') }}</p></template
					>
					<template v-if="['leads', 'candidates'].includes(tab)"
						><h3>{{ t('growth.company_contact_facts') }}</h3>
						<n-descriptions v-if="factsFields.length" :column="1" bordered
							><n-descriptions-item
								v-for="entry in factsFields"
								:key="entry.key"
								:label="entry.label"
								>{{ entry.value }}</n-descriptions-item
							></n-descriptions
						><n-descriptions
							v-for="(contact, index) in factContacts"
							:key="index"
							:column="1"
							bordered
							class="toolbar"
							><n-descriptions-item :label="t('growth.contact_name')">{{
								contact.name || '—'
							}}</n-descriptions-item
							><n-descriptions-item :label="t('growth.contact_role')">{{
								contact.role || '—'
							}}</n-descriptions-item
							><n-descriptions-item :label="t('growth.email')">{{
								contact.email || '—'
							}}</n-descriptions-item
							><n-descriptions-item :label="t('growth.fact_evidence')">{{
								contact.evidence || '—'
							}}</n-descriptions-item></n-descriptions
						><n-collapse class="toolbar"
							><n-collapse-item :title="t('growth.raw_facts')">
								<pre>{{ JSON.stringify(object(detail.record.facts), null, 2) }}</pre>
							</n-collapse-item></n-collapse
						></template
					>
					<template v-if="tab === 'candidates'"
						><h3>{{ t('growth.sources') }}</h3>
						<div v-for="(source, index) in list(detail.record.sources)" :key="index" class="source">
							<a
								v-if="safeLink(source.url)"
								:href="source.url"
								target="_blank"
								rel="noopener noreferrer"
								>{{ source.url }}</a
							>
							<p>{{ source.evidence }}</p>
						</div></template
					>
					<template v-if="tab === 'leads'"
						><h3>{{ t('growth.verification') }}</h3>
						<n-alert v-if="!verificationFields.length" type="info">{{
							t('growth.email_waiting_validation')
						}}</n-alert
						><n-descriptions v-else :column="1" bordered
							><n-descriptions-item
								v-for="entry in verificationFields"
								:key="entry.key"
								:label="entry.label"
								>{{ entry.value }}</n-descriptions-item
							></n-descriptions
						>
						<h3>{{ t('growth.sources') }}</h3>
						<div v-for="(source, index) in list(detail.record.sources)" :key="index" class="source">
							<a
								v-if="safeLink(source.url)"
								:href="source.url"
								target="_blank"
								rel="noopener noreferrer"
								>{{ source.url }}</a
							><span v-else>{{ source.url }}</span>
							<p>{{ source.evidence }}</p>
						</div>
						<n-space class="toolbar"
							><n-button
								v-if="detail.record.state === 'review' && can('outreach:manage')"
								@click="controlLead(detail.record, 'approve')"
								>{{ t('growth.confirm_lead') }}</n-button
							><n-button
								v-if="can('outreach:manage')"
								@click="controlLead(detail.record, 'verify')"
								>{{ t('growth.verify') }}</n-button
							><n-button
								v-if="can('outreach:manage')"
								type="error"
								@click="controlLead(detail.record, 'suppress')"
								>{{ t('growth.suppress') }}</n-button
							></n-space
						></template
					>
					<template v-if="tab === 'runs'"
						><div class="run-counts">
							<div v-for="key in runCountKeys" :key="key">
								<span>{{ t('growth.' + key) }}</span
								><strong>{{ object(detail.live_stats)[key] ?? 0 }}</strong>
							</div>
						</div>
						<n-alert v-if="runHasNoSources" type="warning" class="toolbar">{{
							t('growth.run_no_sources', { count: list(detail.queries).length })
						}}</n-alert>
						<h3>{{ t('growth.queries') }}</h3>
						<div v-for="query in list(detail.queries)" :key="query.id" class="source">
							<strong
								>{{ readable('country', query) }} · {{ readable('language', query) }} ·
								{{ query.query }}</strong
							>
							<p>{{ stateText(query.status) }} {{ growthError(query.error, displayContext) }}</p>
							<div v-for="source in list(query.sources)" :key="source.url">
								<a
									v-if="safeLink(source.url)"
									:href="source.url"
									target="_blank"
									rel="noopener noreferrer"
									>{{ source.title || source.url }}</a
								>
							</div>
						</div></template
					>
					<template v-if="tab === 'inquiries'"
						><p>{{ detail.record.summary }}</p>
						<pre>{{ JSON.stringify(object(detail.record.analysis), null, 2) }}</pre>
						<n-space class="toolbar"
							><n-button
								v-if="can('crm:list')"
								type="primary"
								@click="
									router.push({
										path: '/crm',
										query: { customer: detail.customer_id, message: detail.record.message_id },
									})
								"
								>{{ t('growth.open_customer') }}</n-button
							><n-button
								v-if="detail.record.kind === 'review' && can('inquiry:manage')"
								@click="controlInquiry(detail.record, 'confirm')"
								>{{ t('growth.confirm_inquiry') }}</n-button
							><n-button
								v-if="can('inquiry:manage')"
								@click="controlInquiry(detail.record, 'close')"
								>{{ t('growth.close') }}</n-button
							></n-space
						>
						<h3>{{ t('growth.forward_status') }}</h3>
						<div v-for="forward in list(detail.forwards)" :key="forward.id">
							<p>{{ forward.destination }} · {{ stateText(forward.status) }} {{ forward.error }}</p>
						</div></template
					>
				</template>
			</n-drawer-content></n-drawer
		>
		<n-modal
			:show="workbenchActive && settingsModal"
			@update:show="settingsModal = $event"
			preset="card"
			:title="t('growth.ai_settings')"
			style="width: min(860px, 96vw); max-height: 90vh; overflow: auto">
			<AIProviderSettings />
			<n-divider>{{ t('growth.serper_settings') }}</n-divider
			><n-alert type="info" class="toolbar">{{ t('growth.serper_pipeline_hint') }}</n-alert>
			<n-form label-placement="top"
				><n-form-item
					><n-switch v-model:value="serperSettings.enabled" /><span style="margin-left: 12px">{{
						t('growth.serper_enabled')
					}}</span></n-form-item
				><n-form-item :label="t('growth.serper_api_key')"
					><n-input
						v-model:value="serperSettings.api_key"
						type="password"
						show-password-on="click"
						autocomplete="new-password"
						:placeholder="
							serperHasKey ? t('growth.key_saved') : t('growth.key_required')
						" /></n-form-item
				><n-grid cols="1 600:3" :x-gap="16"
					><n-form-item-gi :label="t('growth.serper_daily_searches')"
						><n-input-number
							v-model:value="serperSettings.daily_searches"
							:min="1"
							:max="100000" /></n-form-item-gi
					><n-form-item-gi :label="t('growth.serper_daily_scrapes')"
						><n-input-number
							v-model:value="serperSettings.daily_scrapes"
							:min="1"
							:max="100000" /></n-form-item-gi
					><n-form-item-gi :label="t('growth.serper_daily_credits')"
						><n-input-number
							v-model:value="serperSettings.daily_credits"
							:min="40"
							:max="10000000" /></n-form-item-gi></n-grid
			></n-form>
			<p class="hint">
				{{
					t('growth.serper_today_usage', {
						searches: serperUsage.searches || 0,
						scrapes: serperUsage.scrapes || 0,
						credits: serperUsage.credits || 0,
					})
				}}
			</p>
			<p class="hint">{{ t('growth.serper_test_hint') }}</p>
			<n-space class="toolbar"
				><n-button type="primary" :loading="saving" @click="saveSerper">{{
					t('growth.save_serper')
				}}</n-button
				><n-button :loading="serperTesting" @click="testSerper">{{
					t('growth.test_serper')
				}}</n-button></n-space
			>
		</n-modal>
	</main>
</template>
<script setup lang="tsx">
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { confirmRemoval } from '@/features/growth/removal'
import AIProviderSettings from '@/features/growth/AIProviderSettings.vue'
import ExpandKeywords from '@/features/growth/ExpandKeywords.vue'
import KeywordInput from '@/features/master/KeywordInput.vue'
import { NButton, NTag } from 'naive-ui'
import RemoteSelect from '@/features/workspace/RemoteSelect.vue'
import { can, principal } from '@/features/workspace/api'
import { growth, list, object, workbook } from '@/features/growth/api'
import marketData from '@/features/growth/market-data.json'
import {
	marketChoices,
	retainChoice,
	countryChanged,
	normalizeDeliveryOptions,
} from '@/features/growth/options'
import {
	growthValue,
	growthEmails,
	growthDetailFields,
	growthError,
	growthVerificationFields,
	growthFactsFields,
} from '@/features/growth/display'
import { parseImportKeywords } from '@/features/contacts/import-state'
const keywordText = ref('')
function changeKeywords(text: string) {
	planConfig.value.exclude_keywords = parseImportKeywords(text)
}
const choices = computed(() => marketChoices(marketData, locale.value))
const serperSettings = ref<any>({
		enabled: false,
		api_key: '',
		daily_searches: 100,
		daily_scrapes: 300,
		daily_credits: 5000,
	}),
	serperHasKey = ref(false),
	serperUsage = ref<any>({}),
	serperTesting = ref(false)
const modelOptions = ref<{ value: string; label: string }[]>([]),
	modelsLoading = ref(false),
	deliveryIdentity = ref<{ from: string; reply_to: string }>({ from: '', reply_to: '' })
let modelsSerial = 0
const selectedModelOptions = computed(() => retainChoice(modelOptions.value, settings.value.model))
import { Message, confirm } from '@/utils'
const { t, te, locale } = useI18n(),
	router = useRouter(),
	stateText = (v: string) => (te('growth.states.' + v) ? t('growth.states.' + v) : v || '—')
const displayContext = computed(() => ({ t, te, locale: locale.value }))
const tab = ref('plans'),
	companyID = ref<number | null>(null),
	companies = ref<any[]>([]),
	rows = ref<any[]>([]),
	page = ref(1),
	total = ref(0),
	keyword = ref(''),
	status = ref<string | null>(null),
	loading = ref(false),
	saving = ref(false),
	stats = ref<any>({})
const planDrawer = ref(false),
	planDraft = ref<any>({}),
	planConfig = ref<any>({}),
	productOptions = ref<any[]>([]),
	productLoading = ref(false),
	ruleModal = ref(false),
	ruleDraft = ref<any>({}),
	detailDrawer = ref(false),
	detail = ref<any>(null),
	settingsModal = ref(false),
	settings = ref<any>({}),
	hasKey = ref(false)
const companyOptions = computed(() => companies.value.map(c => ({ value: c.id, label: c.name }))),
	tabs = computed(() => [
		...(can('outreach:list') ? ['plans', 'runs', 'candidates', 'leads', 'messages'] : []),
		...(can('inquiry:list') ? ['inquiries'] : []),
		...(can('inquiry:manage') ? ['inbox_rules'] : []),
		...(can('material:list') ? ['jobs'] : []),
	])
const statsCards = computed(() =>
	['candidates', 'leads', 'messages', 'inquiries']
		.filter(key => stats.value[key])
		.map(key => ({
			key,
			total: list(stats.value[key]).reduce((sum, r) => sum + Number(r.count), 0),
		}))
)
const displayState = (value: string) =>
	tab.value === 'runs' && value === 'completed' ? t('growth.run_finished') : stateText(value)
const statusOptions = computed(() =>
	(tab.value === 'leads'
		? [
				'review',
				'collected',
				'verified',
				'unconfirmed',
				'invalid',
				'contacted',
				'replied',
				'suppressed',
			]
		: tab.value === 'messages'
			? ['draft', 'approved', 'queued', 'sent', 'failed', 'unknown', 'cancelled']
			: tab.value === 'candidates'
				? [
						'with_email',
						'discovered',
						'collecting',
						'collected',
						'no_email',
						'no_website',
						'filtered',
						'unmatched',
						'review',
						'partial',
						'limited',
						'failed',
					]
				: tab.value === 'runs'
					? ['queued', 'running', 'completed', 'no_results', 'attention']
					: tab.value === 'jobs'
						? ['queued', 'running', 'completed', 'failed', 'configuration_required', 'cancelled']
						: tab.value === 'inquiries'
							? ['new', 'confirmed', 'closed', 'ignored']
							: []
	).map(value => ({ value, label: displayState(value) }))
)
const btn = (key: string, action: () => any) =>
	h(
		NButton,
		{ size: 'small', secondary: true, type: 'primary', onClick: () => run(action) },
		{ default: () => t('growth.' + key) }
	)
const runCountKeys = [
	'query_count',
	'source_count',
	'candidate_count',
	'email_count',
	'filtered_count',
	'imported_count',
	'verified_count',
	'unconfirmed_count',
	'invalid_count',
	'draft_count',
]
const searchChannels = computed(() =>
	['search', 'places', 'both'].map(value => ({ value, label: t('growth.channel_' + value) }))
)
const columns = computed(() => {
	let fields: string[] = []
	if (tab.value === 'plans') fields = ['name', 'company_id', 'active', 'next_at', 'error']
	if (tab.value === 'runs')
		fields = [
			'plan_id',
			'due_at',
			'status',
			'source_count',
			'candidate_count',
			'email_count',
			'verified_count',
			'draft_count',
			'error',
		]
	if (tab.value === 'candidates')
		fields = [
			'company',
			'website',
			'target_country',
			'country',
			'address',
			'emails',
			'phone',
			'state',
			'error',
		]
	if (tab.value === 'leads')
		fields = ['email', 'company', 'country', 'language', 'state', 'assigned_sender', 'error']
	if (tab.value === 'messages') fields = ['subject', 'language', 'status', 'task_id', 'error']
	if (tab.value === 'inquiries') fields = ['summary', 'kind', 'confidence', 'state', 'created_at']
	if (tab.value === 'inbox_rules') fields = ['mailbox', 'forward_to', 'active', 'error']
	if (tab.value === 'jobs') fields = ['kind', 'status', 'progress', 'error']
	const cols = fields.map(key => ({
		key,
		title: t(
			'growth.' +
				(key === 'company' && ['candidates', 'leads'].includes(tab.value)
					? 'customer_company'
					: key)
		),
		minWidth:
			key === 'emails'
				? 260
				: key === 'error' || key === 'summary' || key === 'subject'
					? 230
					: 140,
		render: (r: any) => {
			if (runCountKeys.includes(key)) return String(object(r.live_stats)[key] ?? 0)
			if (key === 'emails') {
				const emails = growthEmails(r.emails)
				return emails.length
					? h('div', { class: 'growth-email-list' }, [
							...emails.slice(0, 2).map(email => h('span', email)),
							emails.length > 2
								? h(
										NButton,
										{ text: true, type: 'primary', onClick: () => run(() => openDetail(r.id)) },
										{ default: () => t('growth.view_all_emails', { count: emails.length }) }
									)
								: null,
						])
					: t('growth.no_email_yet')
			}
			if (['status', 'state', 'kind'].includes(key))
				return h(
					NTag,
					{
						type: ['failed', 'invalid', 'unknown', 'filtered'].includes(r[key])
							? 'warning'
							: 'info',
					},
					{ default: () => growthValue(tab.value, key, r, displayContext.value) }
				)
			return growthValue(tab.value, key, r, displayContext.value)
		},
	}))
	return [
		...cols,
		{
			key: 'actions',
			title: t('growth.actions'),
			fixed: 'right',
			width: tab.value === 'plans' ? 440 : 240,
			render: (r: any) =>
				withDelete(
					r,
					h(
						'div',
						{ class: 'growth-row-actions' },
						tab.value === 'plans'
							? [
									btn('view', () => editPlan(r)),
									can('outreach:run')
										? btn(r.active ? 'pause' : 'enable', () =>
												controlPlan(r, r.active ? 'pause' : 'enable')
											)
										: null,
									can('outreach:run') ? btn('run_now', () => confirmRun(r)) : null,
								]
							: tab.value === 'inbox_rules'
								? [can('inquiry:manage') ? btn('edit', () => editRule(r)) : null]
								: tab.value === 'jobs'
									? [
											btn(
												['failed', 'configuration_required', 'cancelled'].includes(r.status)
													? 'retry'
													: 'cancel',
												() => jobControl(r)
											),
										]
									: [btn('view', () => openDetail(r.id))]
					)
				),
		},
	]
})
const deleteAllowed = computed(() =>
	can(
		['inquiries', 'inbox_rules'].includes(tab.value)
			? 'inquiry:manage'
			: tab.value === 'jobs'
				? 'outreach:manage'
				: 'outreach:manage'
	)
)
const withDelete = (r: any, node: any) =>
	h('div', { class: 'growth-row-actions' }, [
		node,
		deleteAllowed.value
			? h(
					NButton,
					{ size: 'small', type: 'error', secondary: true, onClick: () => run(() => removeRow(r)) },
					{ default: () => t('growth.delete') }
				)
			: null,
	])
async function removeRow(r: any) {
	if (!(await confirmRemoval(t('growth.delete'), t('growth.delete_confirm')))) return
	await growth('delete_record', { kind: tab.value, id: r.id })
	if (rows.value.length === 1 && page.value > 1) page.value--
	await refresh()
}
function toggleCandidateEmails() {
	status.value = status.value === 'with_email' ? null : 'with_email'
	search()
}
const runHasNoSources = computed(
	() =>
		tab.value === 'runs' &&
		['completed', 'no_results'].includes(detail.value?.record?.status) &&
		list(detail.value?.queries).length > 0 &&
		list(detail.value?.queries).every(query => list(query.sources).length === 0)
)
const tableWidth = computed(() =>
	Math.max(
		1300,
		columns.value.reduce((sum, column) => sum + Number(column.width || column.minWidth || 140), 0)
	)
)
const detailFields = computed(() =>
	detail.value ? growthDetailFields(tab.value, detail.value.record, displayContext.value) : []
)
const verificationFields = computed(() =>
	growthVerificationFields(detail.value?.record?.verification, displayContext.value)
)
const factsFields = computed(() =>
	growthFactsFields(detail.value?.record?.facts, displayContext.value)
)
const emailDetails = computed(() => list(detail.value?.email_details))
const factContacts = computed(() => list(object(detail.value?.record?.facts).contacts))
const readable = (key: string, row: any) => growthValue(tab.value, key, row, displayContext.value)
const emailReason = (row: any) =>
	row.reason === 'keyword_match'
		? t('growth.email_filter_reason', { keyword: row.keyword })
		: row.reason
			? growthError(row.reason, displayContext.value)
			: ['collected', 'review'].includes(row.status)
				? t('growth.email_waiting_validation')
				: '—'

watch(
	() => list(planConfig.value.markets).length,
	(count, previous) => {
		if (
			planDrawer.value &&
			count > 0 &&
			(Number(planConfig.value.search_limit) < count ||
				Number(planConfig.value.search_limit) === previous)
		)
			planConfig.value.search_limit = Math.min(count, 20)
	}
)
let serial = 0,
	productPage = 1,
	productTotal = 0,
	productKeyword = '',
	productSerial = 0,
	productTimer: ReturnType<typeof setTimeout> | undefined,
	poll: ReturnType<typeof setInterval>,
	alive = true
const safeLink = (value: string) => /^https?:\/\//i.test(value || '')
async function run(fn: () => any) {
	try {
		return await fn()
	} catch {
		/* Shared API presents the error. */
	}
}
async function refresh(silent = false) {
	const call = ++serial,
		kind = tab.value,
		detailId =
			detailDrawer.value && ['candidates', 'leads', 'runs'].includes(kind)
				? detail.value?.record?.id
				: 0
	if (!silent) loading.value = true
	try {
		const [result, counts, opened] = await Promise.all([
			growth('list', {
				kind,
				company_id: companyID.value || 0,
				page: page.value,
				keyword: keyword.value,
				status: status.value || '',
			}),
			growth('stats'),
			detailId ? growth('detail', { kind, id: detailId }) : Promise.resolve(null),
		])
		if (call === serial) {
			rows.value = list(result.list)
			total.value = Number(result.total) || 0
			stats.value = counts
			if (
				opened &&
				detailDrawer.value &&
				detail.value?.record?.id === detailId &&
				tab.value === kind
			)
				detail.value = opened
		}
	} catch {
		/* Shared API displays errors. */
	} finally {
		if (call === serial) loading.value = false
	}
}

function search() {
	page.value = 1
	return refresh()
}
function changeTab() {
	status.value = null
	keyword.value = ''
	detailDrawer.value = false
	search()
}
const newMarket = () => ({ country: '', language: '', timezone: '', city: '' })
function invalidateModels() {
	modelsSerial++
	modelOptions.value = []
	modelsLoading.value = false
}
async function loadModels() {
	const call = ++modelsSerial
	modelsLoading.value = true
	try {
		const options = await growth('models', {
			data: { base_url: settings.value.base_url, api_key: settings.value.api_key },
		})
		if (call === modelsSerial) modelOptions.value = list(options)
	} catch {
		/* The shared API displays discovery errors. */
	} finally {
		if (call === modelsSerial) modelsLoading.value = false
	}
}
const policyFields = computed(() => [
	{ key: 'per_minute', label: t('workspaceCopy.w381'), max: 100000 },
	{ key: 'per_hour', label: t('workspaceCopy.w382'), max: 1000000 },
	{ key: 'concurrency', label: t('workspaceCopy.w355'), max: 100 },
	{ key: 'batch_size', label: t('workspaceCopy.w384'), max: 1000000 },
	{ key: 'batch_rest_seconds', label: t('workspaceCopy.w374'), max: 2592000 },
	{ key: 'recipient_cooldown_hours', label: t('workspaceCopy.w354'), max: 8760 },
])
function defaults() {
	return {
		...normalizeDeliveryOptions({}),
		product_ids: [],
		search_results: 3,
		search_channel: 'search',
		markets: [{ country: 'US', language: 'en', timezone: 'America/New_York' }],
		requirement: '',
		exclude_keywords: [],
		senders: [],
		daily_leads: 20,
		daily_send: 10,
		search_limit: 1,
		page_limit: 3,
		cadence: 'daily',
		timezone: 'Asia/Shanghai',
		local_time: '09:00',
		send_start: '09:00',
		send_end: '17:00',
		cooldown_days: 30,
		auto_send: false,
		policy: {
			min_interval_ms: 60000,
			batch_size: 1,
			batch_rest_seconds: 60,
			per_minute: 0,
			per_hour: 50,
			per_day: 0,
			recipient_cooldown_hours: 24,
			concurrency: 1,
		},
	}
}
async function editPlan(row?: any) {
	planDraft.value = {
		id: 0,
		name: '',
		company_id: companyID.value || companies.value[0]?.id,
		...row,
	}
	planConfig.value = {
		intent_keywords: [],
		...defaults(),
		...object(row?.config),
		...normalizeDeliveryOptions(object(row?.config), !!row?.id),
	}
	keywordText.value = list(planConfig.value.exclude_keywords).join('\n')
	planConfig.value.policy = { ...defaults().policy, ...object(planConfig.value.policy) }
	productOptions.value = []
	productPage = 1
	productKeyword = ''
	await loadProducts(false)
	for (const id of list(planConfig.value.product_ids)) {
		if (!productOptions.value.some(p => p.value === id)) {
			try {
				const result = await growth('product', { id })
				productOptions.value.push({
					value: id,
					label: result.product.sku + ' · ' + result.product.name,
				})
			} catch {
				/* The shared API displays errors. */
			}
		}
	}
	planDrawer.value = true
}
async function planCompanyChanged() {
	planConfig.value.product_ids = []
	productOptions.value = []
	productPage = 1
	await loadProducts(false)
}
async function loadProducts(append: boolean) {
	if (!planDraft.value.company_id || !can('product:list')) return
	const serial = ++productSerial
	productLoading.value = true
	try {
		const result = await growth('list', {
			kind: 'products',
			company_id: planDraft.value.company_id,
			page: productPage,
			keyword: productKeyword,
			status: 'published',
		})
		if (serial !== productSerial) return
		productTotal = Number(result.total) || 0
		const options = list(result.list).map(r => ({ value: r.id, label: r.sku + ' · ' + r.name }))
		const selected = productOptions.value.filter(p =>
			list(planConfig.value.product_ids).includes(p.value)
		)
		productOptions.value = Array.from(
			new Map(
				[...(append ? productOptions.value : selected), ...options].map(r => [r.value, r])
			).values()
		)
	} finally {
		if (serial === productSerial) productLoading.value = false
	}
}
function productSearch(value: string) {
	clearTimeout(productTimer)
	productKeyword = value
	productPage = 1
	productTimer = setTimeout(() => run(() => loadProducts(false)), 250)
}
function productScroll(event: Event) {
	const el = event.target as HTMLElement
	if (
		!productLoading.value &&
		productPage * 20 < productTotal &&
		el.scrollHeight - el.scrollTop - el.clientHeight < 30
	) {
		productPage++
		run(() => loadProducts(true))
	}
}
async function savePlan() {
	saving.value = true
	try {
		planDraft.value.id = await growth('save_plan', {
			id: planDraft.value.id,
			data: { ...planDraft.value, config: planConfig.value },
		})
		await refresh()
		Message.success(t('growth.plan_saved'))
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
function confirmRun(row: any) {
	confirm({
		title: t('growth.run_now'),
		content: t('growth.run_confirm'),
		onConfirm: () => controlPlan(row, 'run'),
	})
}
async function controlPlan(row: any, operation: string) {
	await run(async () => {
		await growth('plan_control', { id: row.id, data: { operation } })
		await refresh()
		Message.success(t('growth.saved'))
	})
}
async function openEmailLead(entry: any) {
	detailDrawer.value = false
	tab.value = 'leads'
	status.value = null
	keyword.value = entry.email
	page.value = 1
	await refresh()
	await openDetail(entry.lead_id)
}
async function openDetail(id: number) {
	detail.value = await growth('detail', { id, kind: tab.value })
	detailDrawer.value = true
}
async function controlMessage(row: any, operation: string) {
	await run(async () => {
		await growth('message_control', { id: row.id, data: { operation } })
		await openDetail(row.id)
		await refresh()
	})
}
async function regenerateMessage(row: any) {
	saving.value = true
	try {
		await growth('regenerate_message', { id: row.id })
		detailDrawer.value = false
		await refresh()
		Message.success(t('growth.queued'))
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function controlLead(row: any, operation: string) {
	await run(async () => {
		await growth('lead_control', { id: row.id, data: { operation } })
		await openDetail(row.id)
		await refresh()
	})
}
async function controlInquiry(row: any, operation: string) {
	await run(async () => {
		await growth('inquiry_control', { id: row.id, data: { operation } })
		await openDetail(row.id)
		await refresh()
	})
}
async function jobControl(row: any) {
	await growth('job_control', {
		id: row.id,
		data: {
			operation: ['failed', 'configuration_required', 'cancelled'].includes(row.status)
				? 'retry'
				: 'cancel',
		},
	})
	await refresh()
}
function editRule(row?: any) {
	ruleDraft.value = {model: row?.ai_model || '',
		id: 0,
		company_id: companyID.value || companies.value[0]?.id,
		mailbox: '',
		forward_to: '',
		active: false,
		...row,
	}
	ruleModal.value = true
}
async function saveRule() {
	saving.value = true
	try {
		await growth('save_inbox_rule', { id: ruleDraft.value.id, data: ruleDraft.value })
		ruleModal.value = false
		await refresh()
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function openSettings() {
	await run(async () => {
		const result = await growth('settings')
		settings.value = { ...result.config, api_key: '' }
		hasKey.value = result.has_key
		serperSettings.value = { ...object(result.serper?.config), api_key: '' }
		serperHasKey.value = !!result.serper?.has_key
		serperUsage.value = object(result.serper?.usage)
		modelOptions.value = []
		settingsModal.value = true
		if (hasKey.value) await loadModels()
	})
}
async function saveSettings() {
	saving.value = true
	try {
		await growth('save_settings', { data: settings.value })
		settings.value.api_key = ''
		settingsModal.value = false
		Message.success(t('growth.settings_saved'))
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function saveSerper() {
	saving.value = true
	try {
		await growth('save_serper_settings', { data: serperSettings.value })
		serperSettings.value.api_key = ''
		const result = await growth('serper_settings')
		serperSettings.value = { ...result.config, api_key: '' }
		serperHasKey.value = result.has_key
		serperUsage.value = result.usage
		Message.success(t('growth.settings_saved'))
	} catch {
		/* Shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function testSerper() {
	serperTesting.value = true
	try {
		if (serperSettings.value.api_key) {
			await growth('save_serper_settings', { data: serperSettings.value })
			serperSettings.value.api_key = ''
			serperHasKey.value = true
		}
		const result = await growth('test_serper')
		Message.success(
			t('growth.serper_test_success', {
				count: result.search_results,
				characters: result.scrape_characters,
			})
		)
		const current = await growth('serper_settings')
		serperUsage.value = current.usage
	} catch {
		/* Shared API displays errors. */
	} finally {
		serperTesting.value = false
	}
}
let backgroundRefreshing = false
onMounted(async () => {
	tab.value = tabs.value[0] || 'plans'
	await run(async () => {
		deliveryIdentity.value = await growth('delivery_identity')
		if (can('company:list')) companies.value = list(await growth('companies'))
		stats.value = await growth('stats')
		await refresh()
	})
	poll = setInterval(() => {
		if (!workbenchActive.value) return
		if (
			alive &&
			document.visibilityState === 'visible' &&
			['jobs', 'runs', 'candidates', 'leads', 'messages', 'inquiries'].includes(tab.value)
		)
			if (!backgroundRefreshing && !loading.value) {
				backgroundRefreshing = true
				void refresh(true)
					.catch(() => {})
					.finally(() => {
						backgroundRefreshing = false
					})
			}
	}, 15000)
})
onBeforeUnmount(() => {
	alive = false
	serial++
	productSerial++
	modelsSerial++
	clearInterval(poll)
	clearTimeout(productTimer)
})
</script>
<style scoped>
.market-selects {
	display: grid;
	grid-template-columns: 1fr 1fr 1.4fr 1fr;
	gap: 10px;
	width: 100%;
}
.email-detail {
	padding: 14px 0;
	border-bottom: 1px solid #8883;
	overflow-wrap: anywhere;
}
.email-detail strong {
	display: inline-block;
	margin-right: 14px;
	font-size: 16px;
}
.email-detail p {
	margin: 8px 0 0;
}
.growth-page :deep(.growth-email-list) {
	display: flex;
	flex-direction: column;
	gap: 6px;
	overflow-wrap: anywhere;
	white-space: normal;
}
.identity-hint {
	margin-top: 8px;
	font-weight: 600;
}
@media (max-width: 700px) {
	.market-selects {
		grid-template-columns: 1fr;
	}
}
.run-counts {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
	gap: 12px;
	margin: 20px 0;
}
.run-counts > div {
	padding: 14px;
	border: 1px solid #8883;
	border-radius: 8px;
}
.run-counts strong {
	display: block;
	font-size: 24px;
	margin-top: 6px;
}
.growth-page {
	padding: 26px;
	min-width: 0;
}
.growth-page header {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 20px;
	margin-bottom: 24px;
}
.growth-page h1 {
	font-size: 26px;
	margin: 0 0 8px;
}
.growth-page header p,
.hint {
	opacity: 0.65;
	margin: 0;
}
.toolbar {
	margin: 18px 0;
}
.pagination {
	margin-top: 20px;
	justify-content: flex-end;
}
.growth-page :deep(.growth-row-actions) {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	column-gap: 16px;
	row-gap: 10px;
	padding: 6px 0;
}
.growth-page :deep(.growth-row-actions .n-button) {
	min-height: 32px;
	min-width: 64px;
	padding: 0 12px;
	flex-shrink: 0;
}
.stats {
	display: grid;
	grid-template-columns: repeat(3, 1fr);
	gap: 18px;
}
.stats > div {
	border: 1px solid #8883;
	border-radius: 10px;
	padding: 18px 22px;
	display: flex;
	align-items: center;
	justify-content: space-between;
}
.stats strong {
	font-size: 28px;
	color: #218577;
}
.source {
	padding: 16px 0;
	border-bottom: 1px solid #8883;
	overflow-wrap: anywhere;
}
.source a {
	color: #218577;
	text-decoration: underline;
}
pre {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	font-size: 12px;
}
@media (max-width: 700px) {
	.run-counts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
		gap: 12px;
		margin: 20px 0;
	}
	.run-counts > div {
		padding: 14px;
		border: 1px solid #8883;
		border-radius: 8px;
	}
	.run-counts strong {
		display: block;
		font-size: 24px;
		margin-top: 6px;
	}
	.growth-page {
		padding: 14px;
	}
	.growth-page header {
		align-items: flex-start;
		flex-direction: column;
	}
	.stats {
		grid-template-columns: 1fr;
	}
}
</style>
