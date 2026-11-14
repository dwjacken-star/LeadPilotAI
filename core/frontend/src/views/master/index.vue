<template>
	<main class="master-page">
		<header class="master-header">
			<div>
				<span class="eyebrow">{{ $t('ui.pf0e7f1cc948c') }}</span>
				<h1>{{ t('intel.title') }}</h1>
				<p>{{ t('intel.intro') }}</p>
			</div>
			<n-space
				><n-button v-if="can('intelligence:manage')" type="primary" @click="newWorkflow()">{{
					t('intel.new_workflow')
				}}</n-button
				><n-button v-if="can('intelligence:manage')" @click="editPack()">{{
					t('intel.new_pack')
				}}</n-button
				><n-button v-if="can('intelligence:settings') && companyId" @click="run(openWA)"
					>WhatsApp</n-button
				><n-button
					v-if="principal?.super"
					:type="overview.stopped ? 'warning' : 'default'"
					@click="run(toggleStop)"
					>{{ t(overview.stopped ? 'intel.resume_all' : 'intel.pause_all') }}</n-button
				></n-space
			>
		</header>
		<n-alert :type="overview.stopped ? 'warning' : 'info'" class="notice">{{
			t(overview.stopped ? 'intel.paused_hint' : 'intel.approval_hint')
		}}</n-alert>
		<div class="master-toolbar">
			<OptionSelect
				v-model="companyId"
				kind="companies"
				:placeholder="t('intel.select_company')" /><n-button
				:loading="loading"
				@click="run(refresh)"
				>{{ t('intel.refresh') }}</n-button
			><n-button @click="router.push('/materials')">{{ t('intel.materials') }}</n-button
			><n-button @click="router.push('/crm')">{{ t('intel.crm') }}</n-button>
		</div>
		<section class="metrics">
			<n-card v-for="key in metricKeys" :key="key" size="small"
				><span>{{ t('intel.tabs.' + key) }}</span
				><strong>{{ overview.metrics?.[key] ?? '—' }}</strong></n-card
			>
		</section>
		<n-tabs v-model:value="tab" type="line" @update:value="changeTab"
			><n-tab-pane
				v-for="key in tabs.filter(v => v !== 'offerings' || can('product:list'))"
				:key="key"
				:name="key"
				:tab="t('intel.tabs.' + key)"
		/></n-tabs>
		<template v-if="tab === 'packs'"
			><section class="pack-grid">
				<n-card v-for="pack in packs" :key="pack.id" :title="packName(pack)"
					><n-space
						><n-tag>{{ t('intel.object_types.' + object(pack.config).object_type) }}</n-tag
						><n-tag>{{ t('intel.version') }} {{ pack.version }}</n-tag
						><n-tag v-if="Number(pack.dept_id) === 0">{{
							t('intel.system_template')
						}}</n-tag></n-space
					>
					<p>
						{{ object(pack.config).description || array(object(pack.config).terms).join(' · ') }}
					</p>
					<p>{{ t('intel.field_count', { count: array(object(pack.config).fields).length }) }}</p>
					<n-button v-if="can('intelligence:manage')" @click="editPack(pack)">{{
						t(Number(pack.dept_id) === 0 ? 'intel.clone_edit' : 'intel.edit')
					}}</n-button
					><n-button
						v-if="can('intelligence:settings')"
						type="error"
						secondary
						@click="run(() => deleteRow('packs', pack))"
						>{{ t('intel.delete') }}</n-button
					></n-card
				>
			</section></template
		>
		<template v-else-if="tab === 'skills'"
			><n-alert type="info" class="notice">{{ t('intel.skills_hint') }}</n-alert
			><n-data-table
				:columns="skillColumns"
				:data="skills.filter((r: any) => !skillConfigs[r.id]?.removed)"
				:row-key="r => r.id"
				:scroll-x="1200"
		/></template>
		<template v-else-if="tab === 'offerings'"
			><n-space class="notice"
				><n-select
					v-model:value="importPackId"
					:options="packOptions"
					style="width: 280px"
					:placeholder="t('intel.select_pack')" /><n-button
					:disabled="!companyId || !importPackId"
					@click="run(downloadTemplate)"
					>{{ t('intel.download_template') }}</n-button
				><n-button :disabled="!companyId || !importPackId" @click="fileInput?.click()">{{
					t('intel.import_xlsx')
				}}</n-button
				><n-button :disabled="!companyId || !importPackId" @click="newOffering()">{{
					t('intel.new_offering')
				}}</n-button
				><input
					ref="fileInput"
					type="file"
					accept=".xlsx"
					class="hidden-input"
					@change="importFile" /></n-space
			><n-alert type="info" class="notice">{{ t('intel.import_hint') }}</n-alert
			><n-data-table
				:columns="offeringColumns"
				:data="rows"
				:loading="loading"
				:scroll-x="1000" /><n-pagination
				v-model:page="page"
				:item-count="total"
				:page-size="20"
				@update:page="run(refresh)"
		/></template>
		<template v-else
			><n-space class="notice"
				><n-input
					v-model:value="keyword"
					clearable
					:placeholder="t('intel.search')"
					style="width: 280px"
					@keyup.enter="search" /><n-select
					v-if="statusOptions.length"
					v-model:value="status"
					:options="statusOptions"
					clearable
					:placeholder="t('intel.status')"
					style="width: 200px"
					@update:value="search" /><n-button @click="search">{{ t('intel.search') }}</n-button
				><n-button
					v-if="tab === 'prospects' && can('intelligence:export')"
					:disabled="!companyId"
					@click="run(() => workbook('master_prospects', Number(companyId), status || ''))"
					>{{ t('intel.export_prospects') }}</n-button
				></n-space
			><n-data-table
				:columns="columns"
				:data="rows"
				:loading="loading"
				:row-key="r => r.id"
				:scroll-x="1600" /><n-pagination
				v-model:page="page"
				:item-count="total"
				:page-size="20"
				@update:page="run(refresh)"
		/></template>

		<n-modal
			:show="workbenchActive && packModal"
			@update:show="packModal = $event"
			preset="card"
			:title="t('intel.pack_editor')"
			class="master-modal"
			><n-form label-placement="top"
				><div class="form-grid">
					<n-form-item :label="t('intel.name')"
						><n-input v-model:value="packForm.name" /></n-form-item
					><n-form-item :label="t('intel.identifier')"
						><n-input v-model:value="packForm.slug" /></n-form-item
					><n-form-item :label="t('intel.object_type')"
						><n-select
							v-model:value="packForm.config.object_type"
							:options="
								choices(['product', 'service', 'both'], 'intel.object_types.')
							" /></n-form-item
					><n-form-item :label="t('intel.description')"
						><n-input v-model:value="packForm.config.description"
					/></n-form-item>
				</div>
				<div class="form-grid">
					<n-form-item v-for="key in packListFields" :key="key" :label="t('intel.' + key)"
						><n-dynamic-tags v-model:value="packForm.config[key]" /><ExpandKeywords
							v-if="key === 'intent_terms'"
							v-model="packForm.config.intent_terms"
							:industry="packForm.name"
							:seed="packForm.config.description"
					/></n-form-item>
				</div>
				<n-form-item :label="t('intel.tone')"
					><n-input v-model:value="packForm.config.tone"
				/></n-form-item>
				<h3>{{ t('intel.field_schema') }}</h3>
				<n-alert type="info" class="notice">{{ t('intel.schema_hint') }}</n-alert>
				<div v-for="(field, index) in packForm.config.fields" :key="index" class="field-row">
					<n-input v-model:value="field.key" :placeholder="t('intel.field_key')" /><n-input
						v-model:value="field.label"
						:placeholder="t('intel.field_label')" /><n-select
						v-model:value="field.type"
						:options="choices(fieldTypes, 'intel.field_types.')" /><n-input
						v-model:value="field.unit"
						:placeholder="t('intel.unit')" /><n-checkbox v-model:checked="field.required">{{
						t('intel.required')
					}}</n-checkbox
					><n-button size="small" @click="packForm.config.fields.splice(index, 1)">{{
						t('intel.remove')
					}}</n-button
					><n-dynamic-tags v-if="field.type === 'enum'" v-model:value="field.options" />
				</div>
				<n-button
					@click="
						packForm.config.fields.push({
							key: '',
							label: '',
							type: 'text',
							required: false,
							unit: '',
							options: [],
						})
					"
					>{{ t('intel.add_field') }}</n-button
				>
				<h3>{{ t('intel.scoring') }}</h3>
				<div class="form-grid">
					<n-form-item v-for="key in scoreKeys" :key="key" :label="t('intel.score_keys.' + key)"
						><n-input-number
							v-model:value="packForm.config.score_weights[key]"
							:min="0"
							:max="100" /></n-form-item
					><n-form-item :label="t('intel.min_score')"
						><n-input-number
							v-model:value="packForm.config.min_score"
							:min="0"
							:max="100" /></n-form-item
					><n-form-item :label="t('intel.min_confidence')"
						><n-input-number
							v-model:value="packForm.config.min_confidence"
							:min="0"
							:max="1"
							:step="0.05"
					/></n-form-item></div></n-form
			><template #footer
				><n-space justify="end"
					><n-button @click="packModal = false">{{ t('intel.cancel') }}</n-button
					><n-button type="primary" :loading="busy" @click="run(savePack)">{{
						t('intel.save')
					}}</n-button></n-space
				></template
			></n-modal
		>

		<n-modal
			:show="workbenchActive && workflowModal"
			@update:show="workflowModal = $event"
			preset="card"
			:title="t('intel.workflow_editor')"
			class="master-modal"
			><n-form label-placement="top"
				><n-tabs type="line"
					><n-tab-pane name="basic" :tab="t('intel.basic')"
						><div class="form-grid">
							<n-form-item :label="t('intel.name')"
								><n-input v-model:value="workflowForm.name" /></n-form-item
							><n-form-item :label="t('intel.workflow')"
								><n-select
									:value="workflowForm.code"
									:options="flowOptions"
									@update:value="setFlow" /></n-form-item
							><n-form-item :label="t('intel.company')"
								><OptionSelect
									v-model="workflowForm.company_id"
									kind="companies"
									:disabled="!!workflowForm.id" /></n-form-item
							><n-form-item :label="t('intel.industry')"
								><n-select v-model:value="workflowForm.pack_id" :options="packOptions"
							/></n-form-item>
						</div>
						<n-form-item :label="t('growth.model')"><AIModelSelect v-model:value="workflowForm.config.model" /></n-form-item><n-form-item :label="t('intel.requirement')"
							><n-input
								v-model:value="workflowForm.config.requirement"
								type="textarea"
								:autosize="{ minRows: 3, maxRows: 8 }" /></n-form-item
						><n-form-item :label="t('intel.offerings_optional')"
							><OptionSelect
								v-model="workflowForm.config.product_ids"
								kind="products"
								:company-id="workflowForm.company_id"
								multiple /></n-form-item
						><n-form-item :label="t('intel.exclude_keywords')"
							><KeywordInput v-model="workflowForm.config.exclude_keywords"
						/></n-form-item>
						<div class="form-grid">
							<n-form-item :label="t('intel.cadence')"
								><n-select
									v-model:value="workflowForm.config.cadence"
									:options="
										choices(['manual', 'daily', 'weekly', '6h', 'event'], 'intel.cadences.')
									" /></n-form-item
							><n-form-item :label="t('intel.local_time')"
								><n-time-picker
									v-model:formatted-value="workflowForm.config.local_time"
									format="HH:mm:ss"
									value-format="HH:mm:ss" /></n-form-item
							><n-form-item :label="t('intel.timezone')"
								><n-select
									v-model:value="workflowForm.config.timezone"
									filterable
									:options="marketOptions.timezones" /></n-form-item
							><n-form-item :label="t('intel.max_items')"
								><n-input-number
									v-model:value="workflowForm.config.max_items"
									:min="1"
									:max="100" /></n-form-item
							><n-form-item :label="t('intel.queries_per_market')"
								><n-input-number
									v-model:value="workflowForm.config.queries_per_market"
									:min="1"
									:max="5"
							/></n-form-item></div
					></n-tab-pane>
					<n-tab-pane name="markets" :tab="t('intel.markets')"
						><div
							v-for="(market, index) in workflowForm.config.markets"
							:key="index"
							class="market-row">
							<n-select
								:value="market.country"
								filterable
								:options="marketOptions.countries"
								@update:value="v => countryChanged(market, v, catalogData)" /><n-select
								v-model:value="market.language"
								filterable
								:options="marketOptions.languages" /><n-select
								v-model:value="market.timezone"
								filterable
								:options="marketOptions.timezones" /><n-input
								v-model:value="market.city"
								:placeholder="t('intel.city_optional')" /><n-button
								@click="workflowForm.config.markets.splice(index, 1)"
								>{{ t('intel.remove') }}</n-button
							>
						</div>
						<n-button
							@click="
								workflowForm.config.markets.push({
									country: 'US',
									language: 'en',
									timezone: 'America/New_York',
									city: '',
								})
							"
							>{{ t('intel.add_market') }}</n-button
						></n-tab-pane
					>
					<n-tab-pane name="delivery" :tab="t('intel.channels')"
						><n-form-item :label="t('intel.channels')"
							><n-select
								v-model:value="workflowForm.config.channels"
								multiple
								:options="channelOptions" /></n-form-item
						><n-form-item :label="t('intel.senders')"
							><RemoteSelect
								v-model="workflowForm.config.senders"
								type="senders"
								multiple /></n-form-item
						><n-form-item :label="t('intel.social_accounts')"
							><AccountSelect
								v-model="workflowForm.config.social_accounts"
								:platforms="workflowForm.config.channels" /></n-form-item
						><n-form-item :label="t('intel.publishing_images')"
							><LibrarySelect
								v-model="workflowForm.config.image_asset_ids"
								kind="assets"
								:company-id="workflowForm.company_id" /></n-form-item
						><n-form-item :label="t('intel.prospects_optional')"
							><OptionSelect
								v-model="workflowForm.config.prospect_ids"
								kind="prospects"
								:company-id="workflowForm.company_id"
								multiple /></n-form-item
						><n-form-item :label="t('intel.inbox_rules')"
							><OptionSelect
								v-model="workflowForm.config.inbox_rule_ids"
								kind="rules"
								:company-id="workflowForm.company_id"
								multiple
						/></n-form-item>
						<div class="form-grid">
							<n-card v-if="workflowForm.code === 'WF05'" class="notice"
								><n-form-item :label="t('intel.auto_reply')"
									><n-switch
										v-model:value="workflowForm.config.auto_reply"
										:disabled="!can('intelligence:review')" /></n-form-item
								><n-alert type="info" class="notice">{{ t('intel.auto_reply_hint') }}</n-alert
								><template v-if="workflowForm.config.auto_reply"
									><n-card
										v-for="(rule, index) in workflowForm.config.reply_rules"
										:key="index"
										class="notice"
										><n-form-item :label="t('intel.language')"
											><n-select
												v-model:value="rule.language"
												filterable
												:options="marketOptions.languages" /></n-form-item
										><n-form-item :label="t('intel.faq_questions')"
											><n-input
												:value="rule.questions.join('\n')"
												type="textarea"
												@update:value="
													v => (rule.questions = v.split('\n').filter(Boolean))
												" /></n-form-item
										><n-form-item :label="t('intel.faq_answer')"
											><n-input v-model:value="rule.answer" type="textarea" /></n-form-item
										><n-button @click="workflowForm.config.reply_rules.splice(index, 1)">{{
											t('intel.remove')
										}}</n-button></n-card
									><n-button
										@click="
											workflowForm.config.reply_rules.push({
												questions: [],
												answer: '',
												language: 'en',
											})
										"
										>{{ t('intel.add_rule') }}</n-button
									></template
								></n-card
							><n-form-item :label="t('intel.cc_email')"
								><n-input v-model:value="workflowForm.config.cc_email" /></n-form-item
							><n-form-item :label="t('intel.cooldown_days')"
								><n-input-number
									v-model:value="workflowForm.config.cooldown_days"
									:min="1"
									:max="3650" /></n-form-item
							><n-form-item v-for="key in deliveryFlags" :key="key" :label="t('intel.' + key)"
								><n-switch v-model:value="workflowForm.config[key]" /></n-form-item
							><n-form-item :label="t('intel.interval_seconds')"
								><n-input-number
									:value="(workflowForm.config.policy.min_interval_ms || 0) / 1000"
									:min="0"
									@update:value="
										v => (workflowForm.config.policy.min_interval_ms = Number(v) * 1000)
									" /></n-form-item
							><n-form-item :label="t('intel.per_day')"
								><n-input-number
									v-model:value="workflowForm.config.policy.per_day"
									:min="0" /></n-form-item
							><n-form-item :label="t('intel.send_start')"
								><n-time-picker
									v-model:formatted-value="workflowForm.config.policy.window_start"
									format="HH:mm"
									value-format="HH:mm"
									clearable /></n-form-item
							><n-form-item :label="t('intel.send_end')"
								><n-time-picker
									v-model:formatted-value="workflowForm.config.policy.window_end"
									format="HH:mm"
									value-format="HH:mm"
									clearable
							/></n-form-item>
						</div>
						<n-alert type="info">{{ t('intel.channel_hint') }}</n-alert></n-tab-pane
					>
					<n-tab-pane name="budget" :tab="t('intel.budget')"
						><n-alert type="info" class="notice">{{ t('intel.budget_hint') }}</n-alert>
						<div class="form-grid">
							<n-form-item
								v-for="key in budgetKeys"
								:key="key"
								:label="t('intel.budget_fields.' + key)"
								><n-input-number v-model:value="workflowForm.config.budget[key]" :min="0"
							/></n-form-item></div></n-tab-pane></n-tabs></n-form
			><template #footer
				><n-space justify="end"
					><n-button @click="workflowModal = false">{{ t('intel.cancel') }}</n-button
					><n-button type="primary" :loading="busy" @click="run(saveWorkflow)">{{
						t('intel.save_draft')
					}}</n-button></n-space
				></template
			></n-modal
		>

		<n-modal
			:show="workbenchActive && skillModal"
			@update:show="skillModal = $event"
			preset="card"
			:title="t('intel.skill_config')"
			style="width: 600px"
			><n-alert type="info" class="notice">{{ t('intel.skill_snapshot_hint') }}</n-alert
			><n-form label-placement="top"
				><n-form-item :label="t('intel.enabled')"
					><n-switch v-model:value="skillForm.enabled" /></n-form-item
				><n-form-item :label="t('intel.model')"
					><AIModelSelect
						v-model:value="skillForm.model"
						:options="modelOptions"
						filterable
						clearable
						:placeholder="t('intel.default_model')"
				/></n-form-item>
				<div class="form-grid">
					<n-form-item
						v-for="key in ['timeout_seconds', 'retries', 'max_items', 'max_output']"
						:key="key"
						:label="t('intel.' + key)"
						><n-input-number v-model:value="skillForm[key]" :min="0"
					/></n-form-item></div></n-form
			><template #footer
				><n-button type="primary" :loading="busy" @click="run(saveSkill)">{{
					t('intel.save')
				}}</n-button></template
			></n-modal
		>

		<n-drawer
			:show="workbenchActive && detailModal"
			@update:show="detailModal = $event"
			:width="drawerWidth"
			><n-drawer-content :title="t('intel.details')" closable
				><template v-if="detail.record"
					><n-space class="notice"
						><n-tag>{{
							statusLabel(detail.record.status || detail.record.state || detail.record.kind)
						}}</n-tag
						><span>{{ detail.record.company_name || detail.record.company || '' }}</span></n-space
					><template v-if="detailKind === 'contents'"
						><h3>{{ detail.record.subject }}</h3>
						<iframe
							class="mail-preview"
							sandbox=""
							:srcdoc="previewDocument(detail.record.html || '')"
							:title="t('intel.preview')" /><n-form
							v-if="canReview(detail.record.status)"
							label-placement="top"
							><n-form-item :label="t('intel.subject')"
								><n-input v-model:value="contentEdit.subject" /></n-form-item
							><n-form-item :label="t('intel.content')"
								><n-input
									v-model:value="contentEdit.body"
									type="textarea"
									:autosize="{ minRows: 8, maxRows: 18 }"
									:disabled="detail.record.channel === 'whatsapp'" /></n-form-item
							><n-alert v-if="detail.record.channel === 'whatsapp'" type="info">{{
								t('intel.wa_content_hint')
							}}</n-alert></n-form
						><n-space class="notice"
							><n-button
								v-if="
									canReview(detail.record.status) &&
									can('intelligence:manage') &&
									detail.record.channel !== 'whatsapp'
								"
								:loading="busy"
								@click="run(saveContent)"
								>{{ t('intel.save') }}</n-button
							><n-button
								v-if="canReview(detail.record.status) && can('intelligence:review')"
								type="primary"
								:loading="busy"
								@click="run(() => controlContent('approve'))"
								>{{ t('intel.approve') }}</n-button
							><n-button
								v-if="canReview(detail.record.status) && can('intelligence:review')"
								:loading="busy"
								@click="run(() => controlContent('reject'))"
								>{{ t('intel.reject') }}</n-button
							></n-space
						></template
					>
					<template v-if="detailKind === 'evidence'"
						><h3>{{ detail.record.title }}</h3>
						<a
							v-if="safeLink(detail.record.url)"
							:href="detail.record.url"
							target="_blank"
							rel="noopener noreferrer"
							>{{ detail.record.url }}</a
						>
						<p class="plain-content">{{ detail.record.body }}</p></template
					>
					<template v-if="detailKind === 'prospects'"
						><n-descriptions bordered :column="1"
							><n-descriptions-item
								v-for="key in [
									'company',
									'email',
									'website',
									'phone',
									'country',
									'language',
									'score',
								]"
								:key="key"
								:label="t('intel.' + key)"
								>{{ displayValue(detail.record[key]) }}</n-descriptions-item
							></n-descriptions
						><n-form class="notice" label-placement="top"
							><n-form-item :label="t('intel.contact_permission')"
								><n-select
									v-model:value="permissionForm.contact_permission"
									:options="
										choices(['unreviewed', 'allowed', 'blocked'], 'intel.permissions.')
									" /></n-form-item
							><n-form-item :label="t('intel.permission_evidence')"
								><n-input
									v-model:value="permissionForm.permission_evidence"
									type="textarea" /></n-form-item
							><n-checkbox v-model:checked="permissionForm.wa_opt_in">{{
								t('intel.wa_opt_in')
							}}</n-checkbox></n-form
						><n-button
							v-if="can('intelligence:manage')"
							type="primary"
							:loading="busy"
							@click="run(savePermission)"
							>{{ t('intel.save') }}</n-button
						></template
					>
					<template v-if="detailKind === 'events'"
						><p class="plain-content">{{ detail.record.body }}</p>
						<n-alert
							:type="
								object(detail.record.analysis).automatic_reply_status === 'sent'
									? 'success'
									: object(detail.record.analysis).automatic_reply_id
										? 'warning'
										: 'info'
							"
							>{{ t(replyNoticeKey(detail.record.analysis)) }}</n-alert
						><iframe
							v-if="object(detail.record.analysis).reply_draft_html"
							class="mail-preview"
							sandbox=""
							:srcdoc="previewDocument(object(detail.record.analysis).reply_draft_html)"
							:title="t('intel.reply_draft')" /><n-button @click="router.push('/crm')">{{
							t('intel.crm')
						}}</n-button></template
					>
					<template v-if="detailKind === 'runs'"
						><n-alert v-if="detail.record.error" type="warning" class="notice">{{
							detail.record.error
						}}</n-alert
						><n-space class="notice"
							><n-tag>{{ t('intel.tokens') }}: {{ detail.usage?.tokens || 0 }}</n-tag
							><n-tag
								>{{ t('intel.estimated_cost') }}:
								{{ money(detail.usage?.estimated_cost_micros) }}</n-tag
							></n-space
						><n-collapse
							><n-collapse-item
								v-for="step in array(detail.steps)"
								:key="step.skill"
								:name="step.skill"
								:title="
									step.skill +
									' · ' +
									(step.skill === 'MASTER'
										? 'Growth Master'
										: t('intel.skill_names.' + step.skill)) +
									' · ' +
									statusLabel(step.status)
								"
								><p v-if="step.error">{{ step.error }}</p>
								<n-card
									v-for="(finding, index) in array(
										object(step.output).findings || object(step.output).signals
									)"
									:key="index"
									size="small"
									class="notice"
									><h4>{{ finding.title }}</h4>
									<p>{{ finding.summary }}</p>
									<blockquote>{{ finding.excerpt }}</blockquote>
									<p>{{ finding.uncertainty }}</p></n-card
								><n-descriptions :column="1" bordered
									><n-descriptions-item
										v-for="(value, key) in object(step.output)"
										:key="key"
										:label="labelKey(String(key))"
										><span class="plain-content">{{
											displayValue(value)
										}}</span></n-descriptions-item
									></n-descriptions
								></n-collapse-item
							></n-collapse
						></template
					>
					<n-collapse class="notice"
						><n-collapse-item :title="t('intel.evidence_and_snapshot')" name="snapshot">
							<pre class="json-details">{{ JSON.stringify(detail.record, null, 2) }}</pre>
						</n-collapse-item></n-collapse
					>
				</template></n-drawer-content
			></n-drawer
		>

		<n-modal
			:show="workbenchActive && offeringModal"
			@update:show="offeringModal = $event"
			preset="card"
			:title="t('intel.offering_editor')"
			class="master-modal"
			><n-form label-placement="top"
				><div class="form-grid">
					<n-form-item label="SKU"><n-input v-model:value="offeringForm.sku" /></n-form-item
					><n-form-item :label="t('intel.name')"
						><n-input v-model:value="offeringForm.name" /></n-form-item
					><n-form-item :label="t('intel.category')"
						><n-input v-model:value="offeringForm.category"
					/></n-form-item>
				</div>
				<div class="form-grid">
					<n-form-item
						v-for="field in offeringSchema"
						:key="field.key"
						:label="field.label + (field.unit ? ' (' + field.unit + ')' : '')"
						:required="field.required"
						><n-input-number
							v-if="field.type === 'number'"
							v-model:value="offeringForm.fields[field.key]" /><n-switch
							v-else-if="field.type === 'boolean'"
							v-model:value="offeringForm.fields[field.key]" /><n-select
							v-else-if="field.type === 'enum'"
							v-model:value="offeringForm.fields[field.key]"
							:options="field.options.map((v: string) => ({ label: v, value: v }))" /><KeywordInput
							v-else-if="field.type === 'list'"
							v-model="offeringForm.fields[field.key]" /><n-input
							v-else
							v-model:value="offeringForm.fields[field.key]"
					/></n-form-item></div></n-form
			><template #footer
				><n-button type="primary" :loading="busy" @click="run(saveOffering)">{{
					t('intel.save_draft')
				}}</n-button></template
			></n-modal
		>
		<n-modal
			:show="workbenchActive && importModal"
			@update:show="importModal = $event"
			preset="card"
			:title="t('intel.import_preview')"
			class="master-modal"
			><n-space class="notice"
				><n-tag type="success">{{ t('intel.valid') }} {{ importPreview.valid }}</n-tag
				><n-tag type="error">{{ t('intel.invalid') }} {{ importPreview.invalid }}</n-tag
				><n-tag>{{ t('intel.duplicate') }} {{ importPreview.duplicate }}</n-tag></n-space
			><n-data-table :columns="importColumns" :data="importRows" :scroll-x="1100" /><n-pagination
				v-model:page="importPage"
				:item-count="importPreview.total"
				:page-size="100"
				@update:page="run(loadImportRows)" /><template #footer
				><n-space justify="end"
					><n-button @click="downloadInvalid">{{ t('intel.download_invalid') }}</n-button
					><n-button
						:disabled="!importPreview.valid"
						type="primary"
						:loading="busy"
						@click="run(confirmImport)"
						>{{ t('intel.import_valid_only') }}</n-button
					></n-space
				></template
			></n-modal
		>

		<n-modal
			:show="workbenchActive && waModal"
			@update:show="waModal = $event"
			preset="card"
			:title="t('intel.wa_settings')"
			class="master-modal"
			><n-alert type="info" class="notice">{{ t('intel.wa_setup_hint') }}</n-alert
			><n-form label-placement="top"
				><n-form-item :label="t('intel.enabled')"
					><n-switch v-model:value="waForm.enabled"
				/></n-form-item>
				<div class="form-grid">
					<n-form-item
						v-for="key in ['phone_number_id', 'api_version', 'token', 'app_secret', 'verify_token']"
						:key="key"
						:label="t('intel.wa_fields.' + key)"
						><n-input
							v-model:value="waForm[key]"
							:type="['token', 'app_secret', 'verify_token'].includes(key) ? 'password' : 'text'"
							show-password-on="click"
							:placeholder="
								['token', 'app_secret', 'verify_token'].includes(key) ? t('intel.keep_secret') : ''
							"
					/></n-form-item>
				</div>
				<n-form-item :label="t('intel.webhook')"
					><n-input :value="waWebhook" readonly /></n-form-item
				><n-card v-for="(template, index) in waForm.templates" :key="index" class="notice"
					><div class="form-grid">
						<n-form-item :label="t('intel.wa_template_name')"
							><n-input v-model:value="template.name" /></n-form-item
						><n-form-item :label="t('intel.language')"
							><n-select
								v-model:value="template.market_language"
								:options="marketOptions.languages"
								filterable /></n-form-item
						><n-form-item :label="t('intel.wa_language')"
							><n-input v-model:value="template.language" placeholder="en_US"
						/></n-form-item>
					</div>
					<n-form-item :label="t('intel.wa_template_body')"
						><n-input v-model:value="template.body" type="textarea" /></n-form-item
					><n-form-item :label="t('intel.wa_variables')"
						><n-select
							v-model:value="template.variables"
							multiple
							:options="[
								{ label: t('intel.subject'), value: 'subject' },
								{ label: t('intel.content'), value: 'body' },
							]" /></n-form-item
					><n-button @click="waForm.templates.splice(index, 1)">{{
						t('intel.remove')
					}}</n-button></n-card
				><n-button
					@click="
						waForm.templates.push({
							name: '',
							language: 'en_US',
							market_language: 'en',
							body: '',
							variables: [],
						})
					"
					>{{ t('intel.add_template') }}</n-button
				></n-form
			><template #footer
				><n-button type="primary" :loading="busy" @click="run(saveWA)">{{
					t('intel.save')
				}}</n-button></template
			></n-modal
		>
	</main>
</template>

<script setup lang="ts">
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { confirmRemoval } from '@/features/growth/removal'
import { h } from 'vue'
import { NButton, NSpace, NTag } from 'naive-ui'
import { Message } from '@/utils'
import { useRouter } from 'vue-router'
import { master, uploadIndustry } from '@/features/master/api'
import {
	object,
	array,
	channels,
	flows,
	canReview,
	canResume,
	offeringFields,
	previewDocument,
	replyNoticeKey,
} from '@/features/master/display'
import OptionSelect from '@/features/master/OptionSelect.vue'
import AccountSelect from '@/features/social/AccountSelect.vue'
import LibrarySelect from '@/features/social/LibrarySelect.vue'
import ExpandKeywords from '@/features/growth/ExpandKeywords.vue'
import KeywordInput from '@/features/master/KeywordInput.vue'
import RemoteSelect from '@/features/workspace/RemoteSelect.vue'
import { growth, workbook } from '@/features/growth/api'
import { can, principal } from '@/features/workspace/api'
import { marketChoices, countryChanged } from '@/features/growth/options'
import catalogData from '@/features/growth/market-data.json'
const { t, locale, te } = useI18n(),
	router = useRouter(),
	message = Message
const companyId = ref<number | null>(null),
	tab = ref('workflows'),
	page = ref(1),
	keyword = ref(''),
	status = ref<string | null>(null),
	rows = ref<any[]>([]),
	total = ref(0),
	loading = ref(false),
	busy = ref(false)
const packs = ref<any[]>([]),
	skills = ref<any[]>([]),
	skillConfigs = ref<any>({}),
	overview = ref<any>({}),
	defaults = ref<any>({}),
	modelOptions = ref<any[]>([])
const tabs = [
		'workflows',
		'runs',
		'packs',
		'skills',
		'evidence',
		'prospects',
		'contents',
		'events',
		'offerings',
		'audit',
	],
	metricKeys = ['evidence', 'prospects', 'contents', 'events', 'runs', 'workflows']
const scoreKeys = ['relevance', 'market', 'contact', 'intent', 'completeness'],
	fieldTypes = ['text', 'number', 'boolean', 'enum', 'list'],
	packListFields = [
		'terms',
		'buyer_types',
		'intent_terms',
		'exclude_keywords',
		'source_domains',
		'forbidden_claims',
	],
	deliveryFlags = ['unsubscribe', 'track_open', 'track_click', 'warmup']
const budgetKeys = [
	'calls',
	'tokens',
	'searches',
	'scrapes',
	'sends',
	'cost_micros',
	'ai_input_micros_per_million',
	'ai_output_micros_per_million',
	'search_micros',
	'scrape_micros',
]
const packModal = ref(false),
	workflowModal = ref(false),
	skillModal = ref(false),
	offeringModal = ref(false),
	detailModal = ref(false),
	importModal = ref(false),
	waModal = ref(false)
const packForm = ref<any>({ config: {} }),
	workflowForm = ref<any>({ config: { markets: [], policy: {}, budget: {} } }),
	skillForm = ref<any>({}),
	skillId = ref(''),
	detail = ref<any>({}),
	detailKind = ref(''),
	contentEdit = ref<any>({}),
	permissionForm = ref<any>({})
const importPackId = ref<number | null>(null),
	offeringForm = ref<any>({ fields: {} }),
	importPreview = ref<any>({}),
	importRows = ref<any[]>([]),
	importPage = ref(1),
	fileInput = ref<HTMLInputElement | null>(null)
const waForm = ref<any>({ templates: [] }),
	waWebhook = ref(''),
	drawerWidth = ref(Math.min(window.innerWidth - 24, 900))
const marketOptions = computed(() => marketChoices(catalogData, locale.value)),
	flowOptions = computed(() =>
		flows.map(value => ({ value, label: value + ' · ' + t('intel.flow_names.' + value) }))
	)
const packOptions = computed(() =>
		packs.value.map(p => ({ value: Number(p.id), label: packName(p) }))
	),
	channelOptions = computed(() => channels.map(value => ({ value, label: channelName(value) })))
const offeringSchema = computed(() =>
	offeringFields(
		packs.value.find(p => Number(p.id) === Number(offeringForm.value.industry_pack_id)),
		offeringForm.value.fields
	)
)
const choices = (values: string[], prefix: string) =>
	values.map(value => ({ value, label: t(prefix + value) }))
const clone = (value: any) => JSON.parse(JSON.stringify(value)),
	packName = (p: any) =>
		Number(p.dept_id) === 0 && te('intel.pack_names.' + p.slug)
			? t('intel.pack_names.' + p.slug)
			: p.name
const statusLabel = (value: any) =>
		te('intel.states.' + value) ? t('intel.states.' + value) : String(value || '—'),
	channelName = (v: string) =>
		v === 'email'
			? t('intel.email')
			: v === 'whatsapp'
				? 'WhatsApp Business'
				: v === 'facebook'
					? 'Facebook'
					: 'LinkedIn'
const safeLink = (v: string) => /^https?:\/\//i.test(v || ''),
	money = (v: any) => '$' + (Number(v || 0) / 1000000).toFixed(4),
	labelKey = (key: string) =>
		te('intel.output.' + key) ? t('intel.output.' + key) : key.replaceAll('_', ' ')
function displayValue(value: any): string {
	if (value === null || value === undefined || value === '') return '—'
	if (typeof value === 'boolean') return t(value ? 'intel.yes' : 'intel.no')
	if (Array.isArray(value)) return value.map(displayValue).join('\n')
	if (typeof value === 'object')
		return Object.entries(value)
			.map(([k, v]) => `${labelKey(k)}: ${displayValue(v)}`)
			.join('\n')
	return String(value)
}
let alive = true,
	serial = 0,
	timer: ReturnType<typeof setInterval> | undefined
async function run(fn: () => Promise<any>) {
	if (busy.value) return
	busy.value = true
	try {
		await fn()
	} catch {
		/* API layer displays sanitized errors */
	} finally {
		if (alive) busy.value = false
	}
}
async function initialize() {
	const c = await master('catalog')
	if (!alive) return
	packs.value = array(c.packs)
	skills.value = array(c.skills)
	defaults.value = c.defaults
	for (const config of Object.values(defaults.value) as any[]) {
		for (const key of [
			'product_ids',
			'prospect_ids',
			'channels',
			'social_accounts',
			'senders',
			'exclude_keywords',
			'inbox_rule_ids',
		])
			config[key] ||= []
		config.image_asset_ids ||= []
		config.reply_rules ||= []
	}
	const first = array(c.companies)[0]
	if (first) companyId.value = Number(first.id)
	await refresh()
}
async function refresh(silent = false) {
	const seq = ++serial
	if (!silent) loading.value = true
	try {
		const summary = await master('overview', { company_id: companyId.value })
		if (!alive || seq !== serial) return
		overview.value = summary
		if (tab.value === 'packs') return
		if (tab.value === 'skills') {
			if (companyId.value)
				skillConfigs.value = await master('skills', { company_id: companyId.value })
			return
		}
		let result: any
		if (tab.value === 'offerings') {
			result = companyId.value
				? await growth('list', {
						kind: 'products',
						company_id: companyId.value,
						page: page.value,
						keyword: keyword.value,
					})
				: { list: [], total: 0 }
		} else {
			result = await master('list', {
				company_id: companyId.value,
				page: page.value,
				keyword: keyword.value,
				status: status.value || '',
				data: { resource: tab.value },
			})
		}
		if (alive && seq === serial) {
			rows.value = array(result.list)
			total.value = Number(result.total || 0)
		}
	} finally {
		if (alive && seq === serial) loading.value = false
	}
}
function changeTab() {
	page.value = 1
	status.value = null
	keyword.value = ''
	void run(refresh)
}
function search() {
	page.value = 1
	void run(refresh)
}
watch(companyId, () => {
	page.value = 1
	void refresh()
})
const date = (value: any) => {
	if (!value) return '—'
	const d = new Date(value)
	return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleString(locale.value)
}
const button = (
	label: string,
	fn: () => Promise<any> | void,
	type: 'default' | 'primary' | 'warning' = 'default'
) =>
	h(
		NButton,
		{ size: 'small', type, loading: busy.value, onClick: () => void run(async () => fn()) },
		{ default: () => t('intel.' + label) }
	)
const actionCell = (r: any) =>
	h(
		NSpace,
		{ size: 10, wrap: true },
		{
			default: () => {
				const a: any[] =
					tab.value === 'audit' ? [] : [button('details', () => openDetail(tab.value, r))]
				if (tab.value === 'workflows') {
					if (can('intelligence:manage')) a.push(button('edit', () => newWorkflow(r)))
					if (can('intelligence:run')) {
						a.push(button('run_now', () => workflowControl(r, 'run'), 'primary'))
						a.push(
							button(r.active ? 'pause' : 'enable', () =>
								workflowControl(r, r.active ? 'pause' : 'enable')
							)
						)
					}
				}
				if (tab.value === 'runs' && can('intelligence:run')) {
					if (canResume(r.status))
						a.push(button('resume', () => runControl(r, 'resume'), 'primary'))
					if (!['completed', 'no_results', 'cancelled'].includes(r.status))
						a.push(button('cancel', () => runControl(r, 'cancel')))
				}
				if (tab.value === 'audit' ? can('intelligence:settings') : can('intelligence:manage'))
					a.push(button('delete', () => deleteRow(tab.value, r), 'warning'))
				return a
			},
		}
	)
const columns = computed(() => {
	const base: any[] = [
		{ title: t('intel.company'), key: 'company_name', width: 200 },
		{
			title: t('intel.status'),
			key: 'status',
			width: 150,
			render: (r: any) =>
				h(
					NTag,
					{
						type: ['verified', 'completed', 'published', 'accepted'].includes(r.status || r.state)
							? 'success'
							: ['failed', 'invalid', 'blocked', 'configuration_required'].includes(
										r.status || r.state
								  )
								? 'warning'
								: 'default',
					},
					{ default: () => statusLabel(r.status || r.state || (r.active ? 'enabled' : 'draft')) }
				),
		},
	]
	if (tab.value === 'workflows')
		base.splice(
			1,
			0,
			{ title: t('intel.name'), key: 'name', width: 240 },
			{ title: t('intel.industry'), key: 'pack_name', width: 180 },
			{
				title: t('intel.workflow'),
				key: 'code',
				width: 220,
				render: (r: any) => t('intel.flow_names.' + r.code),
			},
			{
				title: t('intel.next_at'),
				key: 'next_at',
				width: 200,
				render: (r: any) => (r.active ? date(r.next_at) : '—'),
			}
		)
	if (tab.value === 'runs')
		base.splice(
			1,
			0,
			{ title: t('intel.workflow'), key: 'workflow_name', width: 260 },
			{
				title: t('intel.progress'),
				key: 'step_index',
				width: 180,
				render: (r: any) => {
					const s = object(r.snapshot)
					return `${r.step_index} / ${array(flowsMap[s.code]).length + 1 || '—'}`
				},
			},
			{ title: t('intel.reason'), key: 'error', width: 360, ellipsis: { tooltip: true } }
		)
	if (tab.value === 'prospects')
		base.splice(
			1,
			0,
			{ title: t('intel.customer_company'), key: 'company', width: 240 },
			{ title: t('intel.email'), key: 'email', width: 260 },
			{ title: t('intel.phone'), key: 'phone', width: 170 },
			{
				title: t('intel.country'),
				key: 'country',
				width: 160,
				render: (r: any) =>
					marketOptions.value.countries.find(o => o.value === r.country)?.label || '—',
			},
			{ title: t('intel.score'), key: 'score', width: 90 },
			{
				title: t('intel.contact_permission'),
				key: 'contact_permission',
				width: 160,
				render: (r: any) => t('intel.permissions.' + r.contact_permission),
			}
		)
	if (tab.value === 'evidence')
		base.splice(
			1,
			0,
			{ title: t('intel.name'), key: 'title', width: 350 },
			{ title: t('intel.source'), key: 'url', width: 360, ellipsis: { tooltip: true } },
			{
				title: t('intel.collected_at'),
				key: 'collected_at',
				width: 190,
				render: (r: any) => date(r.collected_at),
			}
		)
	if (tab.value === 'contents')
		base.splice(
			1,
			0,
			{ title: t('intel.subject'), key: 'subject', width: 300 },
			{
				title: t('intel.channel'),
				key: 'channel',
				width: 150,
				render: (r: any) => channelName(r.channel),
			},
			{
				title: t('intel.target'),
				key: 'target',
				width: 240,
				render: (r: any) =>
					['email', 'whatsapp'].includes(r.channel)
						? r.target
						: channelName(r.channel) + ' · ' + t('intel.authorized_page'),
			},
			{
				title: t('intel.language'),
				key: 'language',
				width: 140,
				render: (r: any) =>
					marketOptions.value.languages.find(o => o.value === r.language)?.label || r.language,
			}
		)
	if (tab.value === 'events')
		base.splice(
			1,
			0,
			{
				title: t('intel.channel'),
				key: 'channel',
				width: 140,
				render: (r: any) => channelName(r.channel),
			},
			{ title: t('intel.sender'), key: 'sender', width: 250 },
			{ title: t('intel.content'), key: 'body', width: 400, ellipsis: { tooltip: true } },
			{
				title: t('intel.classification'),
				key: 'kind',
				width: 150,
				render: (r: any) => statusLabel(r.kind),
			}
		)
	if (tab.value === 'audit')
		base.splice(
			1,
			0,
			{ title: t('intel.action'), key: 'action', width: 260 },
			{
				title: t('intel.collected_at'),
				key: 'created_at',
				width: 190,
				render: (r: any) => date(r.created_at),
			}
		)
	base.push({
		title: t('intel.operations'),
		key: 'actions',
		width: 360,
		fixed: 'right',
		render: actionCell,
	})
	return base
})
async function deleteRow(kind: string, row: any) {
	if (!(await confirmRemoval(t('growth.delete'), t('growth.delete_confirm')))) return
	if (kind === 'offerings') await growth('delete_record', { kind: 'products', id: Number(row.id) })
	else await master('delete', { id: Number(row.id), data: { resource: kind } })
	if (rows.value.length === 1 && page.value > 1) page.value--
	const c = await master('catalog')
	packs.value = array(c.packs)
	await refresh()
}
async function deleteSkill(row: any) {
	if (!companyId.value) {
		message.warning(t('intel.select_company'))
		return
	}
	if (!(await confirmRemoval(t('growth.delete'), t('growth.delete_confirm')))) return
	await master('delete_skill', { company_id: companyId.value, data: { skill: row.id } })
	await refresh()
}
const flowsMap: Record<string, string[]> = {
	WF01: ['S01', 'S02', 'S03', 'S14'],
	WF02: ['S04', 'S05', 'S06', 'S07', 'S08', 'S09', 'S14'],
	WF03: ['S05', 'S06', 'S07', 'S08', 'S09', 'S14'],
	WF04: ['S10', 'S11', 'S14'],
	WF05: ['S12', 'S08', 'S09', 'S14'],
	WF06: ['S13', 'S02', 'S14'],
}
const statusOptions = computed(() => {
	const map: Record<string, string[]> = {
		runs: [
			'queued',
			'running',
			'waiting_approval',
			'waiting_external',
			'completed',
			'no_results',
			'failed',
			'configuration_required',
			'budget_exhausted',
			'paused',
			'cancelled',
		],
		prospects: [
			'discovered',
			'collected',
			'verified',
			'invalid',
			'unconfirmed',
			'filtered',
			'suppressed',
			'no_email',
		],
		contents: [
			'draft',
			'approved',
			'rejected',
			'queued',
			'accepted',
			'published',
			'unknown',
			'failed',
		],
		events: ['new', 'processed'],
	}
	return choices(map[tab.value] || [], 'intel.states.')
})
const skillColumns = computed(() => [
	{ title: t('intel.skill'), key: 'id', width: 100 },
	{
		title: t('intel.name'),
		key: 'name',
		width: 250,
		render: (r: any) => t('intel.skill_names.' + r.id),
	},
	{
		title: t('intel.tools'),
		key: 'tools',
		width: 300,
		render: (r: any) => array(r.tools).join(' · '),
	},
	{
		title: t('intel.quality_gate'),
		key: 'gate',
		width: 300,
		render: (r: any) => t('intel.gates.' + r.id),
	},
	{
		title: t('intel.operations'),
		key: 'action',
		width: 180,
		fixed: 'right' as const,
		render: (r: any) =>
			can('intelligence:manage')
				? h(
						NSpace,
						{ size: 10 },
						{
							default: () => [
								button('configure', () => editSkill(r)),
								button('delete', () => deleteSkill(r), 'warning'),
							],
						}
					)
				: null,
	},
])
const offeringColumns = computed(() => [
	{ title: 'SKU', key: 'sku', width: 180 },
	{ title: t('intel.name'), key: 'name', width: 300 },
	{ title: t('intel.category'), key: 'category', width: 180 },
	{
		title: t('intel.status'),
		key: 'published',
		width: 180,
		render: (r: any) => t(r.published ? 'intel.published' : 'intel.draft'),
	},
	{
		title: t('intel.operations'),
		key: 'action',
		width: 300,
		render: (r: any) =>
			h(
				NSpace,
				{ size: 10 },
				{
					default: () => [
						button('edit', () => newOffering(r)),
						button('delete', () => deleteRow('offerings', r), 'warning'),
						button(r.published ? 'unpublish' : 'publish', async () => {
							await growth('publish_product', { id: Number(r.id), data: { publish: !r.published } })
							await refresh()
						}),
					],
				}
			),
	},
])
const importColumns = computed(() => [
	{ title: t('intel.row'), key: 'row', width: 80 },
	{ title: 'SKU', key: 'sku', width: 160 },
	{ title: t('intel.name'), key: 'name', width: 220 },
	{ title: t('intel.status'), key: 'state', width: 140, render: (r: any) => t('intel.' + r.state) },
	{
		title: t('intel.reason'),
		key: 'errors',
		width: 400,
		render: (r: any) => array(r.errors).join('; '),
	},
])
function editPack(pack?: any) {
	const source = pack || packs.value.find(p => p.slug === 'general')
	packForm.value = {
		id: pack && Number(pack.dept_id) !== 0 ? Number(pack.id) : 0,
		name: pack?.name || '',
		slug: pack ? pack.slug : 'industry_' + Date.now(),
		config: clone(object(source?.config)),
	}
	for (const key of [...packListFields, 'fields']) packForm.value.config[key] ||= []
	packModal.value = true
}
async function savePack() {
	await master('save_pack', { id: packForm.value.id, data: packForm.value })
	packModal.value = false
	const c = await master('catalog')
	packs.value = array(c.packs)
	message.success(t('intel.saved'))
}
async function newWorkflow(row?: any) {
	if (row) {
		workflowForm.value = {
			id: Number(row.id),
			name: row.name,
			code: row.code,
			company_id: Number(row.company_id),
			pack_id: Number(row.pack_id),
			config: { ...clone(defaults.value[row.code]), ...object(row.config) },
		}
	} else {
		workflowForm.value = {
			id: 0,
			name: '',
			code: 'WF03',
			company_id: companyId.value,
			pack_id: importPackId.value || Number(packs.value[0]?.id),
			config: clone(defaults.value.WF03),
		}
	}
	workflowForm.value.config.policy = object(workflowForm.value.config.policy)
	for (const key of [
		'product_ids',
		'prospect_ids',
		'channels',
		'social_accounts',
		'senders',
		'exclude_keywords',
		'inbox_rule_ids',
		'image_asset_ids',
		'reply_rules',
	])
		workflowForm.value.config[key] ||= []
	workflowModal.value = true
}
function setFlow(code: string) {
	const old = workflowForm.value.config
	workflowForm.value.code = code
	workflowForm.value.config = {
		...clone(defaults.value[code]),
		requirement: old.requirement,
 model:old.model,
		markets: old.markets,
		product_ids: old.product_ids || [],
		policy: {},
	}
}
async function saveWorkflow() {
	await master('save_workflow', { id: workflowForm.value.id, data: workflowForm.value })
	workflowModal.value = false
	tab.value = 'workflows'
	await refresh()
	message.success(t('intel.saved'))
}
async function workflowControl(row: any, operation: string) {
	const result = await master('workflow_control', { id: Number(row.id), data: { operation } })
	if (operation === 'run') {
		message.success(t('intel.run_queued'))
		tab.value = 'runs'
	}
	await refresh()
	return result
}
async function runControl(row: any, operation: string) {
	await master('run_control', { id: Number(row.id), data: { operation } })
	await refresh()
}
async function editSkill(row: any) {
	if (!companyId.value) {
		message.warning(t('intel.select_company'))
		return
	}
	skillId.value = row.id
	skillForm.value = {
		enabled: true,
		model: '',
		timeout_seconds: 240,
		retries: 2,
		max_items: 20,
		max_output: 2400,
		...object(skillConfigs.value[row.id]),
	}
	skillModal.value = true
	const result = await master('models')
	modelOptions.value = [{ label: t('intel.default_model'), value: '' }, ...array(result.models)]
}
async function saveSkill() {
	await master('save_skill', {
		company_id: companyId.value,
		data: { skill: skillId.value, config: skillForm.value },
	})
	skillModal.value = false
	await refresh()
}
async function openDetail(kind: string, row: any) {
	detailKind.value = kind
	detail.value = await master('detail', { id: Number(row.id), data: { resource: kind } })
	const r = detail.value.record
	contentEdit.value = { subject: r.subject || '', body: r.body || '' }
	permissionForm.value = {
		contact_permission: r.contact_permission || 'unreviewed',
		permission_evidence: r.permission_evidence || '',
		wa_opt_in: !!r.wa_opt_in,
	}
	detailModal.value = true
}
async function saveContent() {
	await master('content_control', {
		id: Number(detail.value.record.id),
		data: { operation: 'save', ...contentEdit.value },
	})
	await openDetail('contents', detail.value.record)
	await refresh()
}
async function controlContent(operation: string) {
	await master('content_control', { id: Number(detail.value.record.id), data: { operation } })
	await openDetail('contents', detail.value.record)
	await refresh()
}
async function savePermission() {
	await master('prospect_control', {
		id: Number(detail.value.record.id),
		data: permissionForm.value,
	})
	message.success(t('intel.saved'))
	await openDetail('prospects', detail.value.record)
	await refresh()
}
function newOffering(row?: any) {
	offeringForm.value = {
		id: row ? Number(row.id) : 0,
		company_id: companyId.value,
		industry_pack_id: row?.industry_pack_id ? Number(row.industry_pack_id) : importPackId.value,
		sku: row?.sku || '',
		name: row?.name || '',
		category: row?.category || '',
		fields: row ? object(row.fields) : {},
	}
	for (const f of offeringFields(
		packs.value.find(p => Number(p.id) === Number(offeringForm.value.industry_pack_id)),
		offeringForm.value.fields
	)) {
		if (offeringForm.value.fields[f.key] === undefined) offeringForm.value.fields[f.key] = f.value
	}
	offeringModal.value = true
}
async function saveOffering() {
	await growth('save_product', { id: offeringForm.value.id, data: offeringForm.value })
	offeringModal.value = false
	await refresh()
}
async function downloadTemplate() {
	await workbook('industry_template', companyId.value!, String(importPackId.value))
}
async function importFile(event: Event) {
	const input = event.target as HTMLInputElement,
		file = input.files?.[0]
	input.value = ''
	if (!file || !companyId.value || !importPackId.value) return
	await run(async () => {
		importPreview.value = await uploadIndustry(file, companyId.value!, importPackId.value!)
		importRows.value = array(importPreview.value.rows)
		importPage.value = 1
		importModal.value = true
	})
}
async function loadImportRows() {
	const result = await master('import_rows', {
		page: importPage.value,
		data: { token: importPreview.value.token },
	})
	importRows.value = array(result.rows)
}
async function confirmImport() {
	const result = await master('confirm_import', { data: { token: importPreview.value.token } })
	message.success(t('intel.imported', { count: result.imported }))
	importModal.value = false
	await refresh()
}
async function downloadInvalid() {
	await workbook('industry_invalid', companyId.value!, importPreview.value.token)
}
async function openWA() {
	const result = await master('wa_settings', { company_id: companyId.value })
	waForm.value = {
		enabled: false,
		phone_number_id: '',
		api_version: '',
		token: '',
		app_secret: '',
		verify_token: '',
		templates: [],
		...result.config,
	}
	waForm.value.templates ||= []
	waWebhook.value = window.location.origin + result.webhook_path
	waModal.value = true
}
async function saveWA() {
	await master('save_wa_settings', { company_id: companyId.value, data: waForm.value })
	waModal.value = false
	message.success(t('intel.saved'))
}
async function toggleStop() {
	await master('emergency_stop', { data: { enabled: !overview.value.stopped } })
	await refresh()
}
function resize() {
	drawerWidth.value = Math.min(window.innerWidth - 24, 900)
}
let backgroundRefreshing = false
onMounted(() => {
	void run(initialize)
	window.addEventListener('resize', resize)
	timer = setInterval(() => {
		if (!workbenchActive.value) return
		if (
			!document.hidden &&
			!busy.value &&
			!loading.value &&
			!detailModal.value &&
			!workflowModal.value &&
			['runs', 'workflows', 'contents', 'events'].includes(tab.value)
		)
			if (!backgroundRefreshing) {
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
	clearInterval(timer)
	window.removeEventListener('resize', resize)
})
</script>

<style scoped>
.master-page {
	padding: 24px;
	max-width: 1800px;
	margin: auto;
}
.master-header {
	display: flex;
	justify-content: space-between;
	gap: 24px;
	align-items: center;
	margin-bottom: 20px;
}
.master-header h1 {
	font-size: 27px;
	margin: 6px 0;
}
.master-header p {
	opacity: 0.65;
	max-width: 780px;
	margin: 0;
}
.eyebrow {
	font-size: 12px;
	letter-spacing: 2px;
	color: #278564;
	font-weight: 700;
}
.master-toolbar {
	display: flex;
	gap: 12px;
	margin: 20px 0;
}
.master-toolbar .option-picker {
	max-width: 380px;
}
.metrics {
	display: grid;
	grid-template-columns: repeat(6, minmax(0, 1fr));
	gap: 14px;
	margin-bottom: 24px;
}
.metrics span {
	opacity: 0.65;
}
.metrics strong {
	display: block;
	font-size: 28px;
	font-weight: 650;
	margin-top: 8px;
}
.notice {
	margin: 16px 0;
}
.pack-grid {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
	gap: 20px;
}
.master-modal {
	width: min(1060px, 96vw);
}
.form-grid {
	display: grid;
	grid-template-columns: repeat(2, minmax(0, 1fr));
	gap: 0 20px;
}
.field-row {
	display: grid;
	grid-template-columns: 1fr 1fr 120px 100px 80px 70px;
	gap: 10px;
	align-items: center;
	margin: 14px 0;
}
.market-row {
	display: grid;
	grid-template-columns: 1fr 1fr 1.2fr 1fr 70px;
	gap: 10px;
	margin: 14px 0;
}
.mail-preview {
	width: 100%;
	height: 420px;
	border: 1px solid #d9e4df;
	border-radius: 10px;
	background: #fff;
}
.plain-content {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
.json-details {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	font-size: 12px;
	opacity: 0.8;
}
.hidden-input {
	display: none;
}
.n-pagination {
	margin: 20px 0;
	justify-content: flex-end;
}
blockquote {
	border-left: 3px solid #278564;
	margin: 12px 0;
	padding-left: 16px;
	opacity: 0.75;
}
@media (max-width: 900px) {
	.master-page {
		padding: 14px;
	}
	.master-header {
		display: block;
	}
	.master-header .n-space {
		margin-top: 16px;
	}
	.metrics {
		grid-template-columns: repeat(3, minmax(0, 1fr));
	}
	.field-row,
	.market-row {
		grid-template-columns: 1fr 1fr;
	}
	.form-grid {
		grid-template-columns: 1fr;
	}
	.master-toolbar {
		flex-wrap: wrap;
	}
	.master-toolbar .option-picker {
		max-width: none;
	}
}
</style>
