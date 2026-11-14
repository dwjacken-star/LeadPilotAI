<template>
	<main class="social-page">
		<header class="page-heading">
			<div>
				<h1>{{ t('social.title') }}</h1>
				<p>{{ t('social.intro') }}</p>
			</div>
			<n-space
				><n-button v-if="can('social:plan:manage')" type="primary" @click="newPlan">{{
					t('social.new_plan')
				}}</n-button
				><n-button v-if="principal?.super" @click="run(() => openSettings())">{{
					t('social.settings')
				}}</n-button></n-space
			>
		</header>
		<n-alert type="info" class="mb-16px">{{ t('social.setup_hint') }}</n-alert>
		<ManualReminders
			v-if="can('social:publish')"
			ref="manualReminders"
			@open="row => run(() => openDelivery(row))"
			@view="viewManualRecords" />
		<AccountAuthorization
			v-if="can('social:account:manage')"
			ref="accountAuthorization"
			:availability="availability"
			:show-connect="tab === 'accounts'"
			@changed="search" />
		<nav class="legal-links" :aria-label="t('social.legal_notice')">
			<span>{{ t('social.legal_notice') }}</span>
			<a href="/legal/privacy" target="_blank" rel="noopener noreferrer">{{
				t('social.privacy_policy')
			}}</a>
			<a href="/legal/terms" target="_blank" rel="noopener noreferrer">{{
				t('social.terms_service')
			}}</a>
			<a href="/legal/data-deletion" target="_blank" rel="noopener noreferrer">{{
				t('social.data_deletion')
			}}</a>
		</nav>
		<n-space class="toolbar"
			><n-tag
				v-for="platform in platforms"
				:key="platform"
				:type="availability[platform]?.publishing ? 'success' : 'default'"
				>{{ platformName(platform) }} ·
				{{
					t(availability[platform]?.publishing ? 'social.publishing_on' : 'social.publishing_off')
				}}</n-tag
			></n-space
		>
		<n-tabs v-model:value="tab" type="line" @update:value="changeTab"
			><n-tab-pane
				v-for="item in visibleTabs"
				:key="item"
				:name="item"
				:tab="t('social.tabs.' + item)"
		/></n-tabs>
		<n-space class="toolbar"
			><n-select
				v-model:value="filterPlatform"
				clearable
				:options="platforms.map(value => ({ value, label: platformName(value) }))"
				:placeholder="t('social.all_platforms')"
				style="width: 200px"
				@update:value="platformFilterChanged" /><AccountFilter
				v-model="filterAccount"
				:platform="filterPlatform"
				:permission="filterPermission"
				@update:model-value="search"
		/></n-space>
		<template v-if="tab !== 'reports' && tab !== 'calendar'">
			<n-space class="toolbar"
				><n-input
					v-model:value="keyword"
					clearable
					:placeholder="t('social.search')"
					style="width: 260px"
					@keyup.enter="search" /><n-button :loading="loading" @click="search">{{
					t('growth.refresh')
				}}</n-button></n-space
			>
			<n-data-table
				:columns="columns"
				:data="rows"
				:loading="loading"
				:row-key="r => r.id"
				:scroll-x="1600" />
			<n-pagination
				v-model:page="page"
				:item-count="total"
				:page-size="20"
				class="pagination"
				@update:page="refresh" />
		</template>
		<template v-else-if="tab === 'calendar'"
			><n-space class="toolbar"
				><n-date-picker v-model:value="calendarRange" type="daterange" /><n-button
					@click="run(() => loadCalendar())"
					>{{ t('growth.refresh') }}</n-button
				></n-space
			><n-calendar v-model:value="calendarDate"
				><template #default="{ year, month, date }"
					><div
						v-for="event in dayEvents(year, month, date)"
						:key="event.id"
						class="calendar-event"
						@click.stop="openDelivery(event)">
						<b>{{ timeText(event.due_at) }}</b> {{ event.account_name }}
						<div>
							{{ event.plan_name }} · {{ countryText(event.country) }} ·
							{{ languageText(event.language) }}
						</div>
						<n-tag size="small" :type="socialStateClass(event.status)">{{
							stateText(event.status)
						}}</n-tag>
						<n-tag size="small">{{
							t('social.mode_' + (event.publish_mode || 'automatic'))
						}}</n-tag>
						<n-tag size="small">{{ t('social.target_' + (event.target_type || 'page')) }}</n-tag>
					</div></template
				></n-calendar
			></template
		>
		<template v-else>
			<n-space class="toolbar"
				><n-button :loading="loading" @click="run(() => loadReports())">{{
					t('growth.refresh')
				}}</n-button
				><n-button :loading="syncing" @click="run(syncReports)">{{
					t('social.manual_sync')
				}}</n-button
				><span class="hint">{{ t('social.sync_four_hours') }}</span
				><n-button @click="exportSocial(0, currentFilters())">{{
					t('social.export_aggregate')
				}}</n-button></n-space
			><n-alert type="info"
				>{{ t('social.metrics_hint') }}
				<p>{{ t('social.facebook_views_hint') }}</p></n-alert
			>
			<div class="metric-grid">
				<n-card v-for="key in reportMetricKeys(filterPlatform, reportRows)" :key="key"
					><n-statistic
						:label="t('social.metrics.' + key)"
						:value="metricSummary(reportRows, key).value ?? '—'" />
					<p>
						{{
							t('social.metric_coverage', {
								available: metricSummary(reportRows, key).available,
								total: reportRows.length,
							})
						}}
					</p></n-card
				>
			</div>
			<n-data-table
				:columns="reportColumns"
				:data="reportRows"
				:row-key="r => r.id"
				:scroll-x="1500" />
		</template>
		<n-drawer
			:show="workbenchActive && drawer"
			@update:show="drawer = $event"
			:width="1180"
			placement="right"
			:mask-closable="false"
			><n-drawer-content :title="draft.id ? draft.name : t('social.new_plan')" closable>
				<n-tabs v-model:value="step"
					><n-tab-pane name="basics" :tab="t('social.basics')">
						<n-form label-placement="top"
							><n-form-item :label="t('social.plan_name')"
								><n-input v-model:value="draft.name" maxlength="200" /></n-form-item
							><n-form-item :label="t('growth.company')"
								><n-select
									v-model:value="draft.company_id"
									:disabled="!!draft.id"
									filterable
									:options="companies.map(c => ({ value: c.id, label: c.name }))"
							/></n-form-item>
							<n-form-item :label="t('social.platforms')"
								><n-checkbox-group v-model:value="config.platforms"
									><n-space
										><n-checkbox v-for="platform in platforms" :key="platform" :value="platform">{{
											platformName(platform)
										}}</n-checkbox></n-space
									></n-checkbox-group
								></n-form-item
							>
							<n-form-item :label="t('social.accounts')"
								><account-select v-model="config.accounts" :platforms="config.platforms"
							/></n-form-item>
							<n-form-item :label="t('growth.model')"
								><div class="field-row">
									<AIModelSelect
										ref="modelSelect"
										v-model:value="config.model"
										filterable
										:options="availableModels"
										:loading="modelsLoading" /><n-button @click="run(() => refreshModels())">{{
										t('social.refresh_models')
									}}</n-button>
								</div></n-form-item
							>
							<n-form-item :label="t('social.requirement')"
								><n-input
									v-model:value="config.requirement"
									class="large-textarea"
									type="textarea"
									:rows="6"
									maxlength="8000"
									show-count
									:placeholder="t('social.requirement_hint')" /></n-form-item
							><n-form-item :label="t('social.landing_link')"
								><n-input v-model:value="config.link" placeholder="https://www.best-maple.com/"
							/></n-form-item>
							<div class="section-bar">
								<b>{{ t('social.markets') }}</b
								><n-button size="small" @click="config.markets.push(newMarket())">{{
									t('social.add_market')
								}}</n-button>
							</div>
							<div v-for="(market, index) in config.markets" :key="index" class="market-row">
								<n-select
									:value="market.country"
									filterable
									:options="choices.countries"
									@update:value="value => countryChanged(market, value, marketData)" /><n-select
									v-model:value="market.language"
									filterable
									:options="choices.languages" /><n-select
									v-model:value="market.timezone"
									filterable
									:options="choices.timezones" /><n-time-picker
									v-model:formatted-value="market.local_time"
									format="HH:mm:ss"
									value-format="HH:mm:ss" /><n-button
									:disabled="config.markets.length === 1"
									@click="config.markets.splice(index, 1)"
									>{{ t('common.actions.delete') }}</n-button
								>
							</div>
							<p class="hint">{{ t('social.market_hint') }}</p>
						</n-form>
					</n-tab-pane>
					<n-tab-pane name="materials" :tab="t('social.materials')">
						<n-alert type="info">{{ t('social.material_hint') }}</n-alert
						><n-form label-placement="top" class="mt-16px"
							><n-form-item :label="t('social.select_products')"
								><library-select
									v-model="config.products"
									:company-id="draft.company_id"
									kind="products" /></n-form-item
							><n-form-item :label="t('social.select_assets')"
								><library-select
									v-model="config.assets"
									:company-id="draft.company_id"
									kind="assets" /></n-form-item
						></n-form>
						<div class="section-bar">
							<b>{{ t('social.task_materials') }}</b
							><n-space
								><n-checkbox v-model:checked="uploadAI">{{ t('social.allow_ai') }}</n-checkbox
								><n-button
									v-if="can('social:content:edit')"
									:loading="uploading"
									:disabled="!draft.company_id || !draft.name"
									@click="fileInput?.click()"
									>{{ t('social.upload') }}</n-button
								><input
									ref="fileInput"
									type="file"
									multiple
									hidden
									accept=".xlsx,.zip,.pdf,.docx,.pptx,.txt,.md,.jpg,.jpeg,.png,.webp"
									@change="uploadFiles"
							/></n-space>
						</div>
						<n-data-table
							:columns="assetColumns"
							:data="taskAssets"
							:row-key="r => r.id"
							:scroll-x="900" />
					</n-tab-pane>
					<n-tab-pane name="content" :tab="t('social.content')">
						<n-space class="toolbar"
							><n-button
								v-if="can('social:content:edit')"
								type="primary"
								:disabled="!writerEnabled"
								:loading="working"
								@click="generate"
								>{{ t('social.generate') }}</n-button
							><n-button v-if="can('social:content:edit')" @click="manualVariants">{{
								t('social.manual')
							}}</n-button
							><n-button
								v-if="can('social:content:review') && variants.length"
								@click="approveAll"
								>{{ t('social.review_all') }}</n-button
							><n-button @click="run(() => refreshDetail())">{{
								t('growth.refresh')
							}}</n-button></n-space
						><n-alert v-if="!writerEnabled" type="info">{{
							t('social.writer_not_configured')
						}}</n-alert
						><n-alert v-if="draft.status === 'generating'" type="info">{{
							t('social.generating')
						}}</n-alert
						><n-alert v-else-if="draft.error" type="error">{{
							syncErrorText(draft.error)
						}}</n-alert>
						<VariantTabs
							v-if="variants.length"
							v-model="selectedVariant"
							:options="
								variants.map(item => ({
									value: item.id,
									label:
										platformName(item.platform) +
										' · ' +
										countryText(item.country) +
										' · ' +
										languageText(item.language),
								}))
							" />
						<n-tabs
							v-if="variants.length"
							v-model:value="selectedVariant"
							class="variant-panels"
							type="card"
							><n-tab-pane
								v-for="item in variants"
								:key="item.id"
								:name="item.id"
								:tab="
									platformName(item.platform) +
									' · ' +
									countryText(item.country) +
									' · ' +
									languageText(item.language)
								">
								<div class="content-grid">
									<n-form label-placement="top"
										><n-tag :type="socialStateClass(item.state)">{{ stateText(item.state) }}</n-tag
										><n-form-item :label="t('social.post_title')"
											><n-input v-model:value="item.title" maxlength="200" /></n-form-item
										><n-form-item :label="t('social.body')"
											><n-input
												v-model:value="item.body"
												class="body-textarea"
												type="textarea"
												:rows="14"
												show-count /></n-form-item
										><n-form-item :label="t('social.landing_link')"
											><n-input v-model:value="item.link" /></n-form-item
										><n-form-item :label="t('social.media_type')">
											<n-select :value="item.media_type || 'images'" :options="mediaTypeOptions(item.platform)" @update:value="value => changeMediaType(item, value)" />
										</n-form-item>
										<n-form-item v-if="item.media_type === 'video' || item.media_type === 'reels'" :label="t('social.video')">
											<post-video-editor v-model:video="item.video" :variant-id="item.id" :editable="can('social:content:edit')" />
											<n-checkbox v-if="item.platform === 'facebook' && item.media_type === 'reels'" v-model:checked="item.public_reels_confirmed">
												{{ t('social.video_public') }}
											</n-checkbox>
										</n-form-item>
										<n-form-item v-else :label="t('social.images')"
											><post-image-editor
												:key="item.id"
												v-model:images="item.images"
												:variant-id="item.id"
												:options="imageOptions"
												:editable="can('social:content:edit')"
												@uploaded="refreshDetail(false)" /><n-button
												v-if="moreImages"
												size="small"
												@click="run(() => loadLibraryImages(imagePage + 1))"
												>{{ t('social.load_more') }}</n-button
											></n-form-item
										><n-space
											><n-button
												v-if="can('social:content:edit')"
												@click="run(() => saveVariant(item))"
												>{{ t('social.save_content') }}</n-button
											><n-button
												v-if="can('social:content:review')"
												type="primary"
												@click="run(() => approveOne(item))"
												>{{ t('social.approve') }}</n-button
											></n-space
										>
										<p class="hint">{{ t('social.edit_review_hint') }}</p></n-form
									>
									<div class="post-preview">
										<div class="post-brand">
											<span class="brand-avatar">M</span>
											<div>
												<b>{{
													companies.find(c => c.id === draft.company_id)?.brand || 'LeadPilot AI'
												}}</b>
												<p>{{ platformName(item.platform) }} · {{ languageText(item.language) }}</p>
											</div>
										</div>
										<div class="post-body">{{ item.body || t('social.empty_body') }}</div>
										<div v-if="item.link" class="post-link">{{ item.link }}</div>
										<div v-if="item.images?.length" class="post-images">
											<div v-for="image in item.images" :key="image">
												<img
													v-if="imageURLs[image]"
													:src="imageURLs[image]"
													:alt="imageLabel(image)" /><span v-else>{{ imageLabel(image) }}</span>
											</div>
										</div>
										<p class="hint">{{ t('social.preview_hint') }}</p>
									</div>
								</div>
							</n-tab-pane></n-tabs
						><n-empty v-if="!variants.length" :description="t('social.no_variants')" />
					</n-tab-pane>
					<n-tab-pane name="schedule" :tab="t('social.schedule')">
						<n-form label-placement="top"
							><n-form-item :label="t('social.start_date')"
								><n-date-picker
									v-model:formatted-value="publishingDate"
									type="date"
									format="yyyy-MM-dd"
									value-format="yyyy-MM-dd"
									:clearable="false" />
								<p class="hint">{{ t('social.schedule_seconds_hint') }}</p></n-form-item
							><n-form-item :label="t('social.cadence')"
								><n-select
									v-model:value="config.cadence"
									:options="
										['once', 'daily', 'weekly'].map(value => ({
											value,
											label: t('social.cadences.' + value),
										}))
									" /></n-form-item
							><n-alert type="info">{{ t('social.repeat_hint') }}</n-alert
							><n-form-item :label="t('social.daily_posts')"
								><n-input-number
									v-model:value="config.daily_posts"
									:min="1"
									:max="20" /></n-form-item
							><n-form-item :label="t('social.interval')"
								><n-input-number
									v-model:value="config.min_interval_minutes"
									:min="5"
									:max="1440" /></n-form-item
							><n-form-item :label="t('social.sync_interactions')"
								><n-switch v-model:value="config.sync_interactions" /></n-form-item
							><n-form-item :label="t('social.analyze_comments')"
								><n-switch v-model:value="config.analyze_comments" /></n-form-item
							><n-alert type="info">{{ t('social.interaction_policy') }}</n-alert></n-form
						><n-space class="toolbar"
							><n-button
								v-if="can('social:publish')"
								type="primary"
								:disabled="!canSchedule"
								@click="schedulePlan"
								>{{ t('social.confirm_schedule') }}</n-button
							></n-space
						>
						<p class="hint">{{ t('social.schedule_hint') }}</p>
					</n-tab-pane></n-tabs
				>
				<template #footer
					><n-space justify="end"
						><span class="hint">{{ stateText(draft.status || 'draft') }}</span
						><n-button v-if="can('social:plan:manage')" :loading="saving" @click="run(savePlan)">{{
							t('social.save_draft')
						}}</n-button
						><n-button @click="drawer = false">{{ t('social.close') }}</n-button></n-space
					></template
				>
			</n-drawer-content></n-drawer
		>
		<n-modal
			:show="workbenchActive && settingsModal"
			@update:show="settingsModal = $event"
			preset="card"
			:title="t('social.settings')"
			style="width: 850px; max-width: 96vw">
			<n-tabs
				><n-tab-pane
					v-for="platform in platforms"
					:key="platform"
					:name="platform"
					:tab="platformName(platform)"
					><n-form label-placement="top"
						><template v-if="platform === 'instagram' || platform === 'threads'">
							<n-alert type="warning">{{ t('social.meta_notice') }}</n-alert>
							<n-form-item v-if="platform === 'instagram'" :label="t('social.meta_auth_mode')">
								<n-select
									v-model:value="settings.instagram.auth_mode"
									clearable
									:options="[{ value: 'unified', label: 'Unified Login' }]" />
							</n-form-item>
							<n-form-item v-if="platform === 'instagram'" :label="t('social.meta_config_id')">
								<n-input v-model:value="settings.instagram.config_id" autocomplete="off" />
							</n-form-item>
							<n-checkbox
								v-model:checked="settings[platform].test_access_confirmed"
								class="mb-16px"
								>{{ t('social.meta_test_confirmation') }}</n-checkbox
							> </template
						><n-form-item :label="t('social.enable_authorization')"
							><n-switch v-model:value="settings[platform].enabled" /></n-form-item
						><n-form-item :label="t('social.enable_publishing')"
							><n-switch v-model:value="settings[platform].publishing" /></n-form-item
						><n-form-item :label="t('social.client_id')"
							><n-input
								v-model:value="settings[platform].client_id"
								autocomplete="off" /></n-form-item
						><n-form-item :label="t('social.client_secret')"
							><n-input
								v-model:value="settings[platform].client_secret"
								type="password"
								autocomplete="new-password"
								show-password-on="click"
								:placeholder="
									savedKeys[platform + '_secret'] ? t('social.secret_saved') : ''
								" /></n-form-item
						><n-form-item :label="t('social.callback')"
							><n-input
								v-model:value="settings[platform].redirect_url"
								:placeholder="callbackSuggestion" /></n-form-item
						><n-form-item :label="t('social.api_version')"
							><n-input v-model:value="settings[platform].version" /></n-form-item
						><n-form-item :label="t('social.scopes')"
							><n-select
								v-model:value="settings[platform].scopes"
								multiple
								:options="
									scopeOptions[platform].map(value => ({ value, label: value }))
								" /></n-form-item
						><n-form-item :label="t('social.daily_api_calls')"
							><n-input-number
								v-model:value="settings[platform].daily_calls"
								:min="1"
								:max="100000" /></n-form-item
						><n-form-item v-if="platform === 'facebook'" :label="t('social.verify_token')"
							><n-input
								v-model:value="settings.facebook.verify_token"
								type="password"
								autocomplete="new-password"
								:placeholder="
									savedKeys.facebook_verify_token ? t('social.secret_saved') : ''
								" /></n-form-item></n-form
					><n-alert type="info">{{ t('social.authorization_hint') }}</n-alert></n-tab-pane
				><n-tab-pane name="meta_preparation" :tab="t('social.meta_title')"
					><MetaPreparation :settings="settings" /></n-tab-pane
				><n-tab-pane v-if="principal?.super" name="ai_providers" :tab="t('ai_providers.title')"
					><AIProviderSettings @saved="loadModels" /></n-tab-pane
				><n-tab-pane name="analysis" :tab="t('social.analysis_settings')"
					><n-form label-placement="top"
						><n-form-item :label="t('social.sync_interval')"
							><n-input-number
								v-model:value="settings.sync_minutes"
								:disabled="true"
								:min="5"
								:max="1440" /></n-form-item
						><n-form-item :label="t('social.enable_analysis')"
							><n-switch v-model:value="settings.analysis_enabled" /></n-form-item
						><n-form-item :label="t('social.analysis_model')"
							><AIModelSelect
								v-model:value="settings.analysis_model"
								filterable
								:options="availableModels" /></n-form-item
						><n-checkbox v-model:checked="settings.analysis_provider_approved">{{
							t('social.processor_confirmation')
						}}</n-checkbox
						><n-alert type="info" class="mt-16px">{{
							t('social.interaction_policy')
						}}</n-alert></n-form
					></n-tab-pane
				></n-tabs
			><n-space justify="end" class="toolbar"
				><n-button type="primary" :loading="saving" @click="run(() => saveSettings())">{{
					t('growth.save')
				}}</n-button></n-space
			>
		</n-modal>
		<n-modal
			:show="workbenchActive && materialModal"
			@update:show="materialModal = $event"
			preset="card"
			:title="material?.asset?.filename || t('social.materials')"
			style="width: 1050px; max-width: 96vw"
			><template v-if="material"
				><n-space class="toolbar"
					><n-tag>{{ categoryText(material.asset.category) }}</n-tag
					><n-tag :type="socialStateClass(material.asset.status)">{{
						stateText(material.asset.status)
					}}</n-tag
					><n-button @click="run(() => downloadAsset(material.asset))">{{
						t('social.download')
					}}</n-button></n-space
				>
				<p v-if="material.asset.error">{{ material.asset.error }}</p>
				<n-descriptions bordered :column="1"
					><n-descriptions-item
						v-for="fact in values(valueObject(material.asset.extracted).facts)"
						:key="fact.key"
						:label="fact.key"
						>{{ fact.value }}
						<p class="hint">{{ fact.evidence }}</p></n-descriptions-item
					></n-descriptions
				>
				<p>{{ valueObject(material.asset.extracted).summary || material.asset.content }}</p>
				<template v-if="material.total"
					><n-input
						v-model:value="materialKeyword"
						:placeholder="t('social.search_products')"
						@keyup.enter="searchMaterial" /><n-data-table
						:columns="materialRowColumns"
						:data="material.rows"
						:row-key="r => r.id"
						:checked-row-keys="config.row_ids"
						:scroll-x="850"
						@update:checked-row-keys="keys => (config.row_ids = keys)" /><n-pagination
						v-model:page="materialPage"
						:item-count="material.total"
						:page-size="20"
						class="pagination"
						@update:page="loadMaterial" />
					<p class="hint">
						{{ t('social.rows_hint', { count: config.row_ids.length }) }}
					</p></template
				></template
			></n-modal
		>
		<n-modal
			:show="workbenchActive && grantModal"
			@update:show="grantModal = $event"
			preset="card"
			:title="t('social.grant')"
			style="width: 600px; max-width: 94vw"
			><n-form label-placement="top"
				><n-form-item :label="t('social.department')"
					><n-select
						v-model:value="grant.dept_id"
						filterable
						:options="departments.map(d => ({ value: d.id, label: d.name }))" /></n-form-item
				><n-space
					><n-checkbox v-model:checked="grant.can_read">{{ t('social.allow_read') }}</n-checkbox
					><n-checkbox v-model:checked="grant.can_publish">{{
						t('social.allow_publish')
					}}</n-checkbox></n-space
				><n-checkbox v-model:checked="grant.eligible_page_team" class="mt-16px">{{
					t('social.team_confirmation')
				}}</n-checkbox></n-form
			><n-space class="toolbar"
				><n-button type="primary" @click="saveGrant">{{ t('growth.save') }}</n-button
				><n-button @click="revokeGrant">{{ t('social.revoke_grant') }}</n-button></n-space
			></n-modal
		>
		<n-modal
			:show="workbenchActive && interactionModal"
			@update:show="interactionModal = $event"
			preset="card"
			:title="t('social.handle_interaction')"
			style="width: 600px; max-width: 94vw"
			><p class="comment-body">{{ handling.body }}</p>
			<n-form label-placement="top"
				><n-form-item :label="t('social.handling_state')"
					><n-select
						v-model:value="handling.state"
						:options="
							['new', 'processing', 'handled', 'ignored'].map(value => ({
								value,
								label: stateText(value),
							}))
						" /></n-form-item
				><n-form-item :label="t('social.assignee')"
					><n-select
						v-model:value="handling.assigned_to"
						clearable
						filterable
						:options="assignees.map(u => ({ value: u.id, label: u.name }))" /></n-form-item></n-form
			><n-button type="primary" @click="saveHandling">{{ t('growth.save') }}</n-button></n-modal
		>
		<n-modal
			:show="workbenchActive && deliveryModal"
			@update:show="deliveryModal = $event"
			preset="card"
			:title="t('social.delivery_details')"
			style="width: 760px; max-width: 94vw"
			><p>{{ deliveryDetail.plan_name }} · {{ deliveryDetail.account_name }}</p>
			<n-tag>{{ stateText(deliveryDetail.status) }}</n-tag>
			<n-tag>{{ t('social.mode_' + (deliveryDetail.publish_mode || 'automatic')) }}</n-tag>
			<p class="hint">{{ deliveryDetail.error }}</p>
			<pre v-if="deliveryDetail.provider_progress" class="post-body">{{
				JSON.stringify(valueObject(deliveryDetail.provider_progress), null, 2)
			}}</pre>
			<ManualDelivery
				v-if="deliveryDetail.publish_mode === 'manual'"
				:delivery="deliveryDetail"
				@changed="manualDeliveryChanged" />
			<template v-else>
				<div class="post-body">{{ valueObject(deliveryDetail.snapshot).body }}</div>
				<n-input
					v-if="!deliveryDetail.remote_id"
					v-model:value="verifyID"
					class="mt-16px"
					:placeholder="t('social.actual_post_id')" /><n-space class="toolbar"
					><n-button @click="verifyDelivery">{{ t('social.verify') }}</n-button
					><n-button v-if="deliveryDetail.status === 'unknown' && deliveryDetail.platform === 'facebook' && valueObject(deliveryDetail.snapshot).video" @click="reconcileVideo">{{ t('social.video_reconcile') }}</n-button
					><n-popconfirm
						v-if="['unknown', 'failed', 'blocked', 'retry'].includes(deliveryDetail.status)"
						@positive-click="retryDelivery"
						><template #trigger
							><n-button type="warning">{{ t('social.confirm_retry') }}</n-button></template
						>{{ t('social.retry_warning') }}</n-popconfirm
					><n-button
						v-if="deliveryDetail.remote_url"
						tag="a"
						:href="deliveryDetail.remote_url"
						target="_blank"
						rel="noopener noreferrer"
						>{{ t('social.open_post') }}</n-button
					></n-space
				>
			</template>
		</n-modal>
	</main>
</template>

<script setup lang="tsx">
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { confirmRemoval } from '@/features/growth/removal'
import { NButton, NTag, NCheckbox } from 'naive-ui'
import AccountFilter from '@/features/social/AccountFilter.vue'
import VariantTabs from '@/features/social/VariantTabs.vue'
import AIProviderSettings from '@/features/growth/AIProviderSettings.vue'
import AccountSelect from '@/features/social/AccountSelect.vue'
import { can, principal } from '@/features/workspace/api'
import marketData from '@/features/growth/market-data.json'
import { marketChoices, countryChanged, retainChoice } from '@/features/growth/options'
import { social, uploadSocial, socialFile, exportSocial } from '@/features/social/api'
import {
	mergeOptions,
	socialDate,
	valueObject,
	values,
	metricValue,
	metricSummary,
	socialStateClass,
	syncNotice,
	reportMetricKeys,
	socialPlatformName,
} from '@/features/social/display'
import LibrarySelect from '@/features/social/LibrarySelect.vue'
import PostImageEditor from '@/features/social/PostImageEditor.vue'
import PostVideoEditor from '@/features/social/PostVideoEditor.vue'
import AccountAuthorization from '@/features/social/AccountAuthorization.vue'
import MetaPreparation from '@/features/social/MetaPreparation.vue'
import ManualDelivery from '@/features/social/ManualDelivery.vue'
import ManualReminders from '@/features/social/ManualReminders.vue'
import { publishingTime, applyScheduleDate, singleFlight } from '@/features/social/composer'
import { canRunSocialNow } from '@/features/social/composer'
import { Message, confirm } from '@/utils'
const { t, te, locale } = useI18n(),
	route = useRoute(),
	router = useRouter()
const platforms = ['facebook', 'linkedin', 'instagram', 'threads'],
	tab = ref('plans'),
	rows = ref<any[]>([]),
	page = ref(1),
	total = ref(0),
	keyword = ref(''),
	loading = ref(false),
	saving = ref(false),
	working = ref(false),
	availability = ref<any>({})
const accountAuthorization = ref<InstanceType<typeof AccountAuthorization>>(),
	manualReminders = ref<InstanceType<typeof ManualReminders>>()
const drawer = ref(false),
	step = ref('basics'),
	draft = ref<any>({}),
	config = ref<any>({}),
	taskAssets = ref<any[]>([]),
	variants = ref<any[]>([]),
	selectedVariant = ref<number | null>(null),
	companies = ref<any[]>([]),
	fileInput = ref<HTMLInputElement>(),
	uploadAI = ref(true),
	uploading = ref(false)
const choices = computed(() => marketChoices(marketData, locale.value)),
	modelOptions = ref<{ value: string; label: string }[]>([]),
	modelsLoading = ref(false),
	writerEnabled = ref(false),
	availableModels = computed(() => retainChoice(modelOptions.value, config.value.model))
const publishingDate = computed({
	get: () => config.value.start_date,
	set: (value: string | null) => applyScheduleDate(config.value, value),
})
const settingsModal = ref(false),
	settings = ref<any>({ facebook: {}, linkedin: {}, instagram: {}, threads: {} }),
	savedKeys = ref<any>({}),
	callbackSuggestion = location.origin + '/social/callback'
const scopeOptions: Record<string, string[]> = {
	instagram: ['instagram_basic', 'instagram_content_publish'],
	threads: ['threads_basic', 'threads_content_publish'],
	facebook: [
		'pages_show_list',
		'pages_manage_posts',
		'pages_read_engagement',
		'pages_read_user_content',
		'pages_manage_metadata',
		'read_insights',
	],
	linkedin: [
		'openid',
		'profile',
		'w_member_social',
		'rw_organization_admin',
		'r_organization_admin',
		'w_organization_social',
		'r_organization_social',
		'r_organization_social_feed',
		'w_organization_social_feed',
	],
}
const materialModal = ref(false),
	material = ref<any>(null),
	materialPage = ref(1),
	materialKeyword = ref(''),
	materialID = ref(''),
	departments = ref<any[]>([]),
	grantModal = ref(false),
	grant = ref<any>({}),
	grantAccount = ref(0)
const interactionModal = ref(false),
	handling = ref<any>({}),
	assignees = ref<any[]>([]),
	deliveryModal = ref(false),
	deliveryDetail = ref<any>({}),
	verifyID = ref(''),
	calendarDate = ref(Date.now()),
	calendarRange = ref<[number, number]>([Date.now() - 7 * 86400000, Date.now() + 35 * 86400000]),
	calendarEvents = ref<any[]>([]),
	reportRows = ref<any[]>([])
let alive = true,
	serial = 0,
	detailSerial = 0,
	contentBaseline = JSON.stringify(variants.value)
const platformName = socialPlatformName
const runningPlans = ref<number[]>([]),
	filterPlatform = ref<string | null>(null),
	filterAccount = ref<number | null>(null),
	syncing = ref(false)
const filterPermission = computed(() =>
	tab.value === 'reports'
		? 'social:report:view'
		: tab.value === 'interactions'
			? 'social:interaction:view'
			: tab.value === 'accounts'
				? 'social:account:view'
				: 'social:plan:view'
)
const currentFilters = () => ({
	platform: filterPlatform.value || '',
	account_id: filterAccount.value || 0,
})
function platformFilterChanged() {
	filterAccount.value = null
	search()
}

const stateText = (value: string) =>
	te('social.states.' + value) ? t('social.states.' + value) : value || '—'
const categoryText = (value: string) =>
	te('social.categories.' + value) ? t('social.categories.' + value) : value
const countryText = (code: string) =>
	choices.value.countries.find(c => c.value === code)?.label || code
const languageText = (code: string) =>
	choices.value.languages.find(c => c.value === code)?.label || code
const dateText = (value: any) => (value ? socialDate(value).toLocaleString(locale.value) : '—')
const timeText = (value: any) =>
	socialDate(value).toLocaleTimeString(locale.value, {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
	})
const visibleTabs = computed(() => [
	...(can('social:plan:view') ? ['plans', 'deliveries', 'calendar', 'jobs'] : []),
	...(can('social:account:view') ? ['accounts'] : []),
	...(can('social:interaction:view') ? ['interactions'] : []),
	...(can('social:report:view') ? ['reports'] : []),
])
const canSchedule = computed(
	() =>
		JSON.stringify(variants.value) === contentBaseline &&
		variants.value.every(v => v.state === 'approved') &&
		config.value.accounts?.length > 0 &&
		config.value.platforms?.every((p: string) => availability.value[p]?.publishing) &&
		variants.value.length > 0
)
const imageOptions = computed(() => [
	...taskAssets.value
		.filter(
			a =>
				(a.approved || valueObject(a.extracted).purpose === 'post_image') &&
				a.status === 'ready' &&
				['image/jpeg', 'image/png'].includes(a.mime)
		)
		.map(a => ({ value: 'task:' + a.id, label: a.filename })),
	...libraryImages.value,
])
const imageURLs = ref<Record<string, string>>({})
const libraryImages = ref<{ value: string; label: string }[]>([])
const imageLabel = (ref: string) =>
	imageOptions.value.find(o => o.value === ref)?.label || t('social.selected_image')
const newMarket = () => ({
	country: 'US',
	language: 'en',
	timezone: 'America/New_York',
	local_time: '09:00:00',
})
function newConfig() {
	return {
		platforms: ['facebook', 'linkedin'],
		accounts: [],
		products: [],
		assets: [],
		task_assets: [],
		row_ids: [],
		markets: [newMarket()],
		requirement: '',
		model: '',
		link: '',
		start_date: new Date(Date.now() + 86400000).toLocaleDateString('en-CA'),
		cadence: 'once',
		reuse_approved: false,
		daily_posts: 3,
		min_interval_minutes: 30,
		sync_interactions: true,
		analyze_comments: false,
	}
}
async function run(action: () => Promise<any>) {
	try {
		return await action()
	} catch {
		/* Shared API reports errors; keep current inputs. */
	}
}
const button = (label: string, fn: () => Promise<any>) =>
	h(
		NButton,
		{ size: 'small', secondary: true, onClick: () => run(fn) },
		{ default: () => t('social.' + label) }
	)
const syncErrorText = (value: any) => {
	const notice = syncNotice(value)
	return notice ? t(notice.key, notice.values) : String(value || '\u2014')
}
const cell = (row: any, key: string) =>
	['error', 'sync_error'].includes(key)
		? syncErrorText(row[key])
		: key.endsWith('_at')
			? dateText(row[key])
			: ['status', 'state', 'sync_state'].includes(key)
				? h(
						NTag,
						{ size: 'small', type: socialStateClass(row[key]) },
						{ default: () => stateText(row[key]) }
					)
				: key === 'platform'
					? platformName(row[key])
					: key === 'target_type'
						? h(
								NTag,
								{ size: 'small' },
								{ default: () => t('social.target_' + (row[key] || 'page')) }
							)
						: key === 'publish_mode'
							? h(
									NTag,
									{ size: 'small', type: row[key] === 'manual' ? 'warning' : 'success' },
									{ default: () => t('social.mode_' + (row[key] || 'automatic')) }
								)
							: key === 'country'
								? countryText(row[key])
								: key === 'language'
									? languageText(row[key])
									: key === 'category'
										? categoryText(row[key])
										: String(row[key] ?? '—')
const columns = computed(() => {
	let fields: string[] = []
	if (tab.value === 'plans')
		fields = ['name', 'company_name', 'status', 'version', 'next_at', 'error']
	if (tab.value === 'accounts')
		fields = [
			'name',
			'platform',
			'target_type',
			'publish_mode',
			'status',
			'capabilities',
			'expires_at',
			'last_sync_at',
			'error',
		]
	if (tab.value === 'deliveries')
		fields = [
			'plan_name',
			'account_name',
			'platform',
			'target_type',
			'publish_mode',
			'country',
			'language',
			'status',
			'due_at',
			'published_at',
			'sync_state',
			'last_sync_at',
			'sync_error',
			'error',
		]
	if (tab.value === 'jobs')
		fields = ['plan_name', 'kind', 'status', 'attempts', 'updated_at', 'error']
	if (tab.value === 'interactions')
		fields = [
			'account_name',
			'plan_name',
			'kind',
			'actor_name',
			'body',
			'analysis',
			'state',
			'assigned_name',
			'occurred_at',
			'expires_at',
		]
	const cols: any[] = fields.map(key => ({
		key,
		title: t(
			['target_type', 'publish_mode'].includes(key) ? 'social.' + key : 'social.fields.' + key
		),
		minWidth: key === 'body' ? 300 : key === 'error' ? 240 : 150,
		render: (row: any) =>
			row.publish_mode === 'manual' &&
			['capabilities', 'sync_state', 'last_sync_at', 'sync_error'].includes(key)
				? t('social.manual_no_metrics')
				: key === 'capabilities'
					? h(
							'span',
							Object.entries(valueObject(row[key]))
								.filter(([, v]) => v)
								.map(([k]) => t('social.capabilities.' + k))
								.join(' · ') || t('social.no_capability')
						)
					: key === 'analysis'
						? analysisText(row.analysis)
						: key === 'actor_name'
							? row.actor_name || t('social.not_provided')
							: key === 'assigned_name'
								? row.assigned_name || t('social.unassigned')
								: key === 'kind'
									? te('social.kinds.' + row.kind)
										? t('social.kinds.' + row.kind)
										: row.kind
									: cell(row, key),
	}))
	cols.push({
		key: 'actions',
		title: t('social.actions'),
		width: tab.value === 'plans' ? 470 : tab.value === 'accounts' ? 340 : 280,
		fixed: 'right',
		render: (row: any) =>
			withDelete(
				row,
				h(
					'div',
					{ class: 'row-actions' },
					tab.value === 'plans'
						? [
								button('open', () => openPlan(row.id)),
								...(can('social:publish')
									? [
											h(
												NButton,
												{
													size: 'small',
													type: 'primary',
													secondary: true,
													loading: runningPlans.value.includes(row.id),
													disabled:
														!canRunSocialNow(row.status) || runningPlans.value.includes(row.id),
													title: t('social.run_now_hint'),
													onClick: () => run(() => runNow(row)),
												},
												{ default: () => t('social.run_now') }
											),
										]
									: []),
								...(can('social:plan:manage')
									? [
											button(row.status === 'paused' ? 'resume' : 'pause', () =>
												planAction(row, row.status === 'paused' ? 'resume' : 'pause')
											),
											button('archive', () => planAction(row, 'archive')),
										]
									: []),
							]
						: tab.value === 'accounts'
							? [
									...(can('social:account:manage')
										? [
												...(row.publish_mode !== 'manual'
													? [
															...(row.target_type !== 'person'
																? [button('sync', () => accountAction(row, 'sync'))]
																: []),
															button('reconnect', async () => {
																await accountAuthorization.value?.reconnectAccount(row)
															}),
														]
													: []),
												button('disconnect', () => disconnect(row)),
												...(principal.value?.super &&
												row.publish_mode !== 'manual' &&
												row.target_type !== 'person'
													? [button('grant', () => openGrant(row))]
													: []),
											]
										: []),
								]
							: tab.value === 'deliveries'
								? [button('open', () => openDelivery(row))]
								: tab.value === 'jobs' && ['failed', 'configuration_required'].includes(row.status)
									? [button('retry', () => retryJob(row))]
									: tab.value === 'interactions'
										? [
												...(can('social:interaction:manage')
													? [button('handle', () => openHandling(row))]
													: []),
												...(can('social:interaction:analyze') && row.kind === 'comment'
													? [button('analyze', () => analyze(row))]
													: []),
												h(
													NButton,
													{
														size: 'small',
														tag: 'a',
														href: row.remote_url,
														target: '_blank',
														rel: 'noopener noreferrer',
													},
													{ default: () => t('social.open_post') }
												),
											]
										: []
				)
			),
	})
	return cols
})
function analysisText(value: any) {
	const a = valueObject(value)
	if (!a.topic) return t('social.not_analyzed')
	return `${t('social.topics.' + a.topic)} · ${t('social.sentiments.' + a.sentiment)} · ${t('social.priorities.' + a.priority)} · ${Math.round(Number(a.confidence) * 100)}%`
}
const assetColumns = computed(() => [
	{
		key: 'selected',
		title: t('social.use_material'),
		width: 100,
		render: (row: any) =>
			h(NCheckbox, {
				checked: config.value.task_assets?.includes(row.id),
				disabled:
					!row.approved || !row.ai_allowed || row.status !== 'ready' || row.category === 'internal',
				onUpdateChecked: (checked: boolean) => {
					const ids = new Set(config.value.task_assets)
					if (checked) ids.add(row.id)
					else ids.delete(row.id)
					config.value.task_assets = [...ids]
				},
			}),
	},
	...['filename', 'category', 'status', 'approved', 'error'].map(key => ({
		key,
		title: t('social.fields.' + key),
		minWidth: key === 'filename' ? 240 : 120,
		render: (r: any) =>
			key === 'approved' ? t(r.approved ? 'social.approved' : 'social.unapproved') : cell(r, key),
	})),
	{
		key: 'actions',
		title: t('social.actions'),
		width: 340,
		render: (row: any) =>
			h('div', { class: 'row-actions' }, [
				button('preview', () => openMaterial(row)),
				...(can('social:content:edit')
					? [
							button('approve', () => materialAction(row, 'approve')),
							button('recognize', () => materialAction(row, 'recognize')),
							button('archive_library', () => materialAction(row, 'archive_to_library')),
						]
					: []),
			]),
	},
])
const materialRowColumns = computed(() => [
	{ type: 'selection', disabled: (row: any) => row.state !== 'valid' },
	...['sheet', 'row_number', 'sku', 'name', 'fields', 'state'].map(key => ({
		key,
		title: t('social.fields.' + key),
		minWidth: 140,
		render: (r: any) =>
			key === 'fields'
				? Object.entries(valueObject(r.fields))
						.map(([k, v]) => `${k}: ${v}`)
						.join(' · ')
				: cell(r, key),
	})),
])
const reportColumns = computed(() => [
	...['plan_name', 'account_name', 'platform', 'target_country', 'language'].map(key => ({
		key,
		title: t('social.fields.' + key),
		minWidth: 150,
		render: (r: any) => (key === 'target_country' ? countryText(r[key]) : cell(r, key)),
	})),
	...['reactions', 'comments', 'shares', 'views', 'impressions', 'reach', 'clicks'].map(key => ({
		key,
		title: t('social.metrics.' + key),
		minWidth: 110,
		render: (r: any) => metricValue(r, key) ?? '—',
	})),
	{
		key: 'as_of',
		title: t('social.fields.as_of'),
		minWidth: 180,
		render: (r: any) => dateText(r.as_of),
	},
	{
		key: 'sync_error',
		title: t('social.fields.sync_error'),
		minWidth: 320,
		render: (r: any) => syncErrorText(r.sync_error),
	},
	{
		key: 'actions',
		title: t('social.actions'),
		width: 130,
		fixed: 'right' as const,
		render: (r: any) =>
			can(deletePermission('reports'))
				? h(
						NButton,
						{ size: 'small', type: 'error', onClick: () => run(() => deleteSocialRow(r)) },
						{ default: () => t('social.delete') }
					)
				: null,
	},
])
async function refresh(silent = false) {
	if (!visibleTabs.value.includes(tab.value)) tab.value = visibleTabs.value[0] || 'plans'
	if (tab.value === 'calendar') return loadCalendar(silent)
	if (tab.value === 'reports') return loadReports(silent)
	const call = ++serial
	if (!silent) loading.value = true
	try {
		const result = await social('list', {
			kind: tab.value,
			page: page.value,
			keyword: keyword.value,
			...currentFilters(),
		})
		if (alive && call === serial) {
			rows.value = result.list || []
			total.value = Number(result.total) || 0
		}
	} finally {
		if (alive && call === serial) loading.value = false
	}
}
function search() {
	page.value = 1
	void run(refresh)
}
function viewManualRecords() {
	tab.value = 'deliveries'
	search()
}
function changeTab() {
	page.value = 1
	keyword.value = ''
	void run(refresh)
}
async function loadReferences() {
	if (can('social:plan:view')) companies.value = values(await social('companies'))
}
async function loadModels() {
	if (!can('social:content:edit')) return
	modelsLoading.value = true
	try {
		const result = await social('models')
		modelOptions.value = result.models || []
		writerEnabled.value = !!result.enabled
		if (!config.value.model) config.value.model = result.model || ''
	} finally {
		modelsLoading.value = false
	}
}
const modelSelect = ref<InstanceType<typeof AIModelSelect> | null>(null)
async function refreshModels() {
	await Promise.all([loadModels(), modelSelect.value?.reload()])
}
async function newPlan() {
	await run(async () => {
		await loadReferences()
		draft.value = {
			id: 0,
			name: '',
			company_id: companies.value[0]?.id || null,
			status: 'draft',
			request_key: crypto.randomUUID(),
		}
		config.value = newConfig()
		variants.value = []
		taskAssets.value = []
		step.value = 'basics'
		drawer.value = true
		contentBaseline = JSON.stringify(variants.value)
		await loadModels()
	})
}
async function openPlan(id: number) {
	await loadReferences()
	const result = await social('detail', { id })
	draft.value = { ...result.plan }
	config.value = { ...newConfig(), ...valueObject(result.plan.config) }
	for (const key of [
		'platforms',
		'accounts',
		'products',
		'assets',
		'task_assets',
		'row_ids',
		'markets',
	])
		config.value[key] = values(config.value[key])
	config.value.markets.forEach((market: any) => {
		market.local_time = publishingTime(market.local_time)
	})
	variants.value = (result.variants || []).map((r: any) => ({ ...r, images: values(r.images) }))
	taskAssets.value = result.assets || []
	selectedVariant.value = variants.value[0]?.id || null
	step.value = 'basics'
	drawer.value = true
	contentBaseline = JSON.stringify(variants.value)
	await loadModels()
	void loadLibraryImages()
}
const savePlan = singleFlight(async () => {
	if (!can('social:plan:manage')) return draft.value.id || 0

	if (!draft.value.name || !draft.value.company_id) {
		Message.warning(t('social.name_company_required'))
		return 0
	}
	saving.value = true
	try {
		const result = await social('save_plan', {
			id: draft.value.id || 0,
			data: {
				name: draft.value.name,
				company_id: draft.value.company_id,
				config: config.value,
				request_key: draft.value.request_key,
			},
		})
		draft.value.id = result.id
		Message.success(t('social.saved'))
		await refreshDetail(false)
		void run(refresh)
		return result.id
	} finally {
		saving.value = false
	}
})
async function refreshDetail(replaceContent = true) {
	if (!draft.value.id) return
	const call = ++detailSerial
	const result = await social('detail', { id: draft.value.id })
	if (!alive || call !== detailSerial || draft.value.id !== result.plan.id) return
	draft.value.status = result.plan.status
	draft.value.version = result.plan.version
	draft.value.error = result.plan.error || ''
	working.value = result.plan.status === 'generating'
	taskAssets.value = result.assets || []
	if (replaceContent) {
		variants.value = (result.variants || []).map((r: any) => ({ ...r, images: values(r.images) }))
		if (!variants.value.some(v => v.id === selectedVariant.value))
			selectedVariant.value = variants.value[0]?.id || null
		contentBaseline = JSON.stringify(variants.value)
	}
	void loadLibraryImages()
}
async function generate() {
	await run(async () => {
		if (!(await savePlan())) return
		await social('plan_control', { id: draft.value.id, data: { operation: 'generate' } })
		draft.value.status = 'generating'
		draft.value.error = ''
		step.value = 'content'
		working.value = true
		await refreshDetail()
	})
}
async function manualVariants() {
	await run(async () => {
		if (!(await savePlan())) return
		await social('manual_variants', { id: draft.value.id })
		await refreshDetail()
		step.value = 'content'
	})
}
async function saveVariant(item: any) {
	await social('save_variant', {
		id: item.id,
		data: { title: item.title, body: item.body, link: item.link, images: item.images, video:item.video || '', media_type:item.media_type || 'images', public_reels_confirmed:!!item.public_reels_confirmed },
	})
	const result = await social('detail', { id: draft.value.id })
	const updated = (result.variants || []).find((v: any) => v.id === item.id)
	if (updated) {
		Object.assign(item, { ...updated, images: values(updated.images) })
		const baseline = JSON.parse(contentBaseline || '[]')
		const index = baseline.findIndex((v: any) => v.id === item.id)
		if (index >= 0) baseline[index] = { ...item }
		else baseline.push({ ...item })
		contentBaseline = JSON.stringify(baseline)
	}
	Message.success(t('social.saved'))
}
function mediaTypeOptions(platform: string) {
	const kinds = platform === 'facebook' ? ['images', 'video', 'reels'] : platform === 'instagram' ? ['images', 'reels'] : platform === 'threads' ? ['images', 'video'] : ['images']
	return kinds.map(value => ({ value, label: t('social.' + (value === 'images' ? 'images' : value === 'reels' ? 'video_reels' : 'video')) }))
}
function changeMediaType(item: any, value: string) {
	item.media_type = value
	item.video = ''
	item.public_reels_confirmed = false
	if (value !== 'images') item.images = []
}
async function approveOne(item: any) {
	if (JSON.stringify(variants.value) !== contentBaseline) {
		Message.warning(t('social.edit_review_hint'))
		return
	}
	await social('approve_variant', { id: item.id })
	await refreshDetail()
	Message.success(t('social.approved'))
}
async function approveAll() {
	if (JSON.stringify(variants.value) !== contentBaseline) {
		Message.warning(t('social.edit_review_hint'))
		return
	}
	await run(async () => {
		for (const item of variants.value) await social('approve_variant', { id: item.id })
		await refreshDetail()
	})
}
async function schedulePlan() {
	await run(async () => {
		if (!(await savePlan())) return
		await social('schedule', { id: draft.value.id })
		await refreshDetail()
		await refresh()
		Message.success(t('social.scheduled'))
	})
}
async function planAction(row: any, operation: string) {
	await social('plan_control', { id: row.id, data: { operation } })
	await refresh()
}
const deletePermission = (kind: string) =>
	kind === 'accounts' || kind === 'reports'
		? 'social:account:manage'
		: kind === 'interactions'
			? 'social:interaction:manage'
			: 'social:plan:manage'
const withDelete = (row: any, content: any) =>
	h('div', { class: 'row-actions' }, [
		content,
		can(deletePermission(tab.value))
			? h(
					NButton,
					{
						size: 'small',
						type: 'error',
						secondary: true,
						onClick: () => run(() => deleteSocialRow(row)),
					},
					{ default: () => t('social.delete') }
				)
			: null,
	])
async function deleteSocialRow(row: any) {
	if (!(await confirmRemoval(t('social.delete'), t('social.delete_confirm')))) return
	await social('delete_record', { kind: tab.value, id: row.id })
	if (rows.value.length === 1 && page.value > 1) page.value--
	await refresh()
}
async function syncReports() {
	if (syncing.value) return
	syncing.value = true
	try {
		const result = await social('sync_reports', currentFilters())
		Message.success(t('social.sync_queued', { count: result.queued }))
		await loadReports()
	} finally {
		syncing.value = false
	}
}
async function runNow(row: any) {
	if (runningPlans.value.includes(row.id)) return
	runningPlans.value = [...runningPlans.value, row.id]
	try {
		await social('plan_control', { id: row.id, data: { operation: 'run_now' } })
		Message.success(t('social.run_now_queued'))
		await refresh()
	} finally {
		runningPlans.value = runningPlans.value.filter(id => id !== row.id)
	}
}
async function uploadFiles(event: Event) {
	const input = event.target as HTMLInputElement,
		files = [...(input.files || [])]
	input.value = ''
	if (!files.length) return
	await run(async () => {
		if (!draft.value.id && !(await savePlan())) return
		uploading.value = true
		try {
			for (const file of files) await uploadSocial(file, draft.value.id, uploadAI.value)
			await refreshDetail(false)
		} finally {
			uploading.value = false
		}
	})
}
async function materialAction(row: any, operation: string) {
	await social('material_control', {
		asset_id: row.id,
		data: { operation, purpose: row.category === 'product_sheet' ? 'products' : 'company' },
	})
	await refreshDetail(false)
}
async function openMaterial(row: any) {
	materialID.value = row.id
	materialPage.value = 1
	materialKeyword.value = ''
	await loadMaterial()
	materialModal.value = true
}
async function loadMaterial() {
	material.value = await social('material_detail', {
		asset_id: materialID.value,
		page: materialPage.value,
		keyword: materialKeyword.value,
	})
}
function searchMaterial() {
	materialPage.value = 1
	void run(loadMaterial)
}
async function downloadAsset(row: any) {
	const blob = await socialFile(row.id)
	const url = URL.createObjectURL(blob)
	const a = document.createElement('a')
	a.href = url
	a.download = row.filename.split('/').pop()
	a.click()
	setTimeout(() => URL.revokeObjectURL(url), 1000)
}
const imagePage = ref(1),
	moreImages = ref(false)
async function loadLibraryImages(page = 1) {
	if (!draft.value.company_id || !can('material:list')) return
	const company = draft.value.company_id
	const result = await social('library_options', {
		kind: 'assets',
		page,
		data: { company_id: company },
	})
	if (!alive || company !== draft.value.company_id) return
	imagePage.value = page
	moreImages.value = page * 20 < result.total
	libraryImages.value = mergeOptions(
		page === 1
			? libraryImages.value.filter(o => variants.value.some(v => v.images?.includes(o.value)))
			: libraryImages.value,
		(result.list || [])
			.filter((a: any) => ['image/jpeg', 'image/png'].includes(a.mime))
			.map((a: any) => ({ value: 'library:' + a.id, label: a.filename }))
	) as any
}
async function openSettings() {
	await run(async () => {
		const result = await social('settings')
		settings.value = result.config
		savedKeys.value = result.saved
		settingsModal.value = true
		await loadModels()
	})
}
async function saveSettings() {
	saving.value = true
	try {
		await social('save_settings', { data: settings.value })
		settingsModal.value = false
		availability.value = await social('availability')
		Message.success(t('social.saved'))
	} finally {
		saving.value = false
	}
}
async function accountAction(row: any, operation: string) {
	await social('account_control', { id: row.id, data: { operation } })
	await refresh()
}
async function disconnect(row: any) {
	confirm({
		title: t('social.disconnect'),
		content: t('social.disconnect_hint'),
		onConfirm: () => run(() => accountAction(row, 'disconnect')),
	})
}
async function openGrant(row: any) {
	departments.value = await social('departments')
	grantAccount.value = row.id
	grant.value = { dept_id: null, can_read: true, can_publish: false, eligible_page_team: false }
	grantModal.value = true
}
async function saveGrant() {
	await run(async () => {
		await social('account_control', {
			id: grantAccount.value,
			data: { ...grant.value, operation: 'grant' },
		})
		grantModal.value = false
		await refresh()
	})
}
async function revokeGrant() {
	await run(async () => {
		await social('account_control', {
			id: grantAccount.value,
			data: { dept_id: grant.value.dept_id, operation: 'revoke_grant' },
		})
		grantModal.value = false
	})
}
async function analyze(row: any) {
	await social('interaction_control', { id: row.id, data: { operation: 'analyze' } })
	Message.success(t('social.analysis_queued'))
}
async function openHandling(row: any) {
	handling.value = { ...row }
	assignees.value = await social('assignees', { id: row.account_id })
	interactionModal.value = true
}
async function saveHandling() {
	await run(async () => {
		await social('interaction_control', {
			id: handling.value.id,
			data: {
				operation: 'state',
				state: handling.value.state,
				assigned_to: handling.value.assigned_to,
			},
		})
		interactionModal.value = false
		await refresh()
	})
}
async function retryJob(row: any) {
	await social('retry_job', { id: row.id })
	await refresh()
}
async function openDelivery(row: any) {
	deliveryDetail.value = await social('delivery_detail', { id: row.id })
	verifyID.value = ''
	deliveryModal.value = true
}
async function manualDeliveryChanged() {
	deliveryModal.value = false
	await run(async () => {
		await refresh()
		await manualReminders.value?.refresh()
	})
}
async function verifyDelivery() {
	await run(async () => {
		await social('delivery_control', {
			id: deliveryDetail.value.id,
			data: { operation: 'verify', remote_id: verifyID.value },
		})
		deliveryModal.value = false
		await refresh()
	})
}
async function retryDelivery() {
	await social('delivery_control', {
		id: deliveryDetail.value.id,
		data: { operation: 'retry', confirmed_absent: true },
	})
	deliveryModal.value = false
	await refresh()
}
async function reconcileVideo() {
	await run(async () => {
		await social('delivery_control', { id: deliveryDetail.value.id, data: { operation: 'reconcile' } })
		deliveryModal.value = false
		await refresh()
	})
}
async function loadReports(silent = false) {
	const call = ++serial
	if (!silent) loading.value = true
	try {
		const result = await social('reports', currentFilters())
		if (alive && call === serial) reportRows.value = result.rows || []
	} finally {
		if (alive && call === serial) loading.value = false
	}
}
async function loadCalendar(silent = false) {
	const call = ++serial
	if (!silent) loading.value = true
	try {
		const [from, to] = calendarRange.value
		const result = await social('calendar', {
			...currentFilters(),
			data: { from: new Date(from).toISOString(), to: new Date(to + 86400000).toISOString() },
		})
		if (alive && call === serial) calendarEvents.value = values(result)
	} finally {
		if (alive && call === serial) loading.value = false
	}
}
function dayEvents(year: number, month: number, date: number) {
	return calendarEvents.value.filter(event => {
		const d = socialDate(event.due_at)
		return d.getFullYear() === year && d.getMonth() + 1 === month && d.getDate() === date
	})
}
let polling = false
const poll = setInterval(() => {
	if (!workbenchActive.value) return
	if (!alive) return
	if (drawer.value && draft.value.id && JSON.stringify(variants.value) === contentBaseline) {
		void run(() => refreshDetail(draft.value.status === 'generating'))
	} else if (!drawer.value && !settingsModal.value && !loading.value && !polling) {
		polling = true
		void run(() => refresh(true)).finally(() => {
			polling = false
		})
	}
}, 15000)
watch(
	() => variants.value.flatMap(v => v.images || []),
	async refs => {
		for (const ref of new Set(refs as string[])) {
			if (imageURLs.value[ref]) continue
			const [source, id] = ref.split(':')
			try {
				const blob = await socialFile(id, source)
				if (alive) imageURLs.value[ref] = URL.createObjectURL(blob)
			} catch {
				/* API reports unavailable media */
			}
		}
	},
	{ deep: true }
)

onMounted(() => {
	void run(async () => {
		availability.value = await social('availability')
		await loadReferences()
		await refresh()
		if (route.query.oauth) {
			Message.info(t('social.auth_callback_hint'))
			router.replace('/social')
		}
	})
})
onBeforeUnmount(() => {
	alive = false
	serial++
	detailSerial++
	clearInterval(poll)
	Object.values(imageURLs.value).forEach(url => URL.revokeObjectURL(url))
})
</script>

<style scoped>
.variant-panels :deep(.n-tabs-nav) {
	display: none;
}

.legal-links {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 8px 16px;
	margin: 12px 0 18px;
	font-size: 12px;
	color: var(--color-text-3);
}
.legal-links a {
	color: var(--primary-color, #187151);
	text-decoration: underline;
	text-underline-offset: 3px;
}
.social-page {
	padding: 24px;
	overflow: auto;
	height: 100%;
}
.page-heading {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 20px;
	margin-bottom: 20px;
}
.page-heading h1 {
	font-size: 24px;
	margin: 0 0 8px;
}
.page-heading p,
.hint {
	color: var(--color-text-3);
	line-height: 1.65;
	font-size: 12px;
}
.toolbar {
	margin: 16px 0;
}
.pagination {
	margin-top: 20px;
	justify-content: flex-end;
}
.row-actions {
	display: flex;
	gap: 10px;
	flex-wrap: wrap;
	padding: 6px 0;
}
.metric-grid {
	display: grid;
	grid-template-columns: repeat(5, minmax(0, 1fr));
	gap: 16px;
	margin: 20px 0;
}
.market-row {
	display: grid;
	grid-template-columns: 1fr 1fr 1.4fr 140px 70px;
	gap: 12px;
	margin: 12px 0;
}
.section-bar {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin: 20px 0 12px;
}
.field-row {
	display: flex;
	gap: 10px;
	width: 100%;
}
.field-row :deep(.n-select) {
	flex: 1;
	min-width: 0;
}
.content-grid {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 24px;
}
.post-preview {
	background: var(--color-bg-1);
	border: 1px solid var(--color-border-1);
	border-radius: 12px;
	padding: 24px;
	align-self: start;
	min-width: 0;
}
.post-brand {
	display: flex;
	align-items: center;
	gap: 12px;
	margin-bottom: 20px;
}
.brand-avatar {
	background: #178842;
	color: white;
	width: 44px;
	height: 44px;
	border-radius: 50%;
	display: grid;
	place-items: center;
	font-weight: 700;
}
.post-brand p {
	margin: 4px 0;
	font-size: 12px;
	color: var(--color-text-3);
}
.post-body,
.comment-body {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	line-height: 1.75;
}
.post-link {
	overflow-wrap: anywhere;
	color: #1763ac;
	margin: 16px 0;
}
.post-images {
	display: grid;
	grid-template-columns: repeat(2, minmax(0, 1fr));
	gap: 8px;
}
.post-images img {
	display: block;
	width: 100%;
	max-height: 300px;
	object-fit: contain;
}
.post-images > div {
	padding: 20px;
	background: var(--color-bg-2);
	border-radius: 8px;
	overflow-wrap: anywhere;
}
.calendar-event {
	border-left: 3px solid #198b49;
	padding: 5px;
	background: var(--color-bg-2);
	font-size: 11px;
	line-height: 1.5;
	cursor: pointer;
	margin: 4px 0;
}
.large-textarea :deep(textarea) {
	min-height: 150px;
	line-height: 1.6;
}
.body-textarea :deep(textarea) {
	min-height: 330px;
	line-height: 1.6;
}
@media (max-width: 900px) {
	.metric-grid {
		grid-template-columns: repeat(2, minmax(0, 1fr));
	}
	.content-grid {
		grid-template-columns: 1fr;
	}
	.market-row {
		grid-template-columns: 1fr 1fr;
	}
	.page-heading {
		align-items: flex-start;
		flex-direction: column;
	}
}
</style>
