<template>
	<main class="growth-page">
		<header>
			<div>
				<h1>{{ t('growth.materials') }}</h1>
				<p>{{ t('growth.material_intro') }}</p>
			</div>
			<n-button v-if="can('company:manage')" type="primary" @click="editCompany()">{{
				t('growth.add_company')
			}}</n-button>
		</header>
		<n-space class="toolbar" align="center"
			><n-select
				v-model:value="companyID"
				:options="
					companies.map(c => ({ value: c.id, label: c.brand ? c.name + ' · ' + c.brand : c.name }))
				"
				style="width: 320px"
				:placeholder="t('growth.company')"
				@update:value="changeCompany" />
			<n-button :loading="loading" @click="refresh">{{ t('growth.refresh') }}</n-button>
			<n-button v-if="companyID && can('company:manage')" @click="editCompany(currentCompany)">{{
				t('growth.edit_company')
			}}</n-button>
		</n-space>
		<n-alert v-if="!companyID" type="info">{{ t('growth.select_company') }}</n-alert>
		<template v-else>
			<n-tabs v-model:value="tab" type="line" @update:value="changeTab">
				<n-tab-pane v-if="can('product:list')" name="products" :tab="t('growth.products')" />
				<n-tab-pane v-if="can('material:list')" name="assets" :tab="t('growth.material_library')" />
				<n-tab-pane v-if="can('material:list')" name="jobs" :tab="t('growth.jobs')" />
			</n-tabs>
			<n-space class="toolbar" align="center">
				<n-input
					v-model:value="keyword"
					clearable
					:placeholder="t('growth.search')"
					style="width: 250px"
					@keyup.enter="search" />
				<n-select
					v-model:value="status"
					clearable
					:options="statusOptions"
					style="width: 190px"
					:placeholder="t('growth.status')"
					@update:value="search" />
				<n-button @click="search">{{ t('growth.search') }}</n-button>
				<n-button v-if="tab === 'products' && can('product:manage')" @click="editProduct()">{{
					t('growth.add_product')
				}}</n-button>
				<n-button
					v-if="tab !== 'jobs' && can('material:upload')"
					type="primary"
					@click="openUpload"
					>{{
						t(tab === 'products' ? 'growth.import_products' : 'growth.upload_materials')
					}}</n-button
				>
				<n-button
					v-if="
						tab !== 'jobs' &&
						can('material:upload') &&
						(tab !== 'products' || can('product:import'))
					"
					:loading="templateLoading !== ''"
					@click="downloadTemplate(tab === 'products' ? 'products' : 'company')"
					>{{
						t(tab === 'products' ? 'growth.product_template' : 'growth.company_template')
					}}</n-button
				>
				<n-button
					v-if="tab === 'products' && can('product:export')"
					@click="run(()=>workbook('products',companyID!))"
					>{{ t('growth.export') }}</n-button
				>
			</n-space>
			<n-data-table
				:columns="columns"
				:data="rows"
				:loading="loading"
				:row-key="row => row.id"
				:scroll-x="1200" />
			<n-pagination
				v-model:page="page"
				:item-count="total"
				:page-size="20"
				class="pagination"
				@update:page="refresh" />
		</template>
		<n-modal
			:show="workbenchActive && companyModal"
			@update:show="companyModal = $event"
			preset="card"
			:title="t('growth.company')"
			style="width: min(760px, 96vw)">
			<n-form label-placement="top"
				><n-form-item :label="t('growth.company_name')"
					><n-input v-model:value="companyDraft.name"
				/></n-form-item>
				<n-grid :cols="2" :x-gap="16"
					><n-form-item-gi :label="t('growth.brand')"
						><n-input v-model:value="companyDraft.brand" /></n-form-item-gi
					><n-form-item-gi :label="t('growth.domains')"
						><n-dynamic-tags v-model:value="companyDraft.domains" /></n-form-item-gi
				></n-grid>
				<n-form-item :label="t('growth.company_facts')"
					><div style="width: 100%">
						<n-dynamic-input v-model:value="companyFields" :on-create="newField"
							><template #default="{ value }"
								><n-input v-model:value="value.key" :placeholder="t('growth.field')" /><n-input
									v-model:value="value.value"
									type="textarea"
									:autosize="{ minRows: 1, maxRows: 5 }"
									:placeholder="t('growth.value')" /></template
						></n-dynamic-input></div
				></n-form-item>
				<n-alert type="info">{{ t('growth.publish_hint') }}</n-alert>
			</n-form>
			<n-space class="toolbar"
				><n-button type="primary" :loading="saving" @click="saveCompany">{{
					t('growth.save_draft')
				}}</n-button
				><n-popconfirm
					v-if="companyDraft.id && can('company:publish')"
					@positive-click="publishCompany"
					><template #trigger
						><n-button :disabled="saving">{{ t('growth.publish_company') }}</n-button></template
					>{{ t('growth.publish_confirm') }}</n-popconfirm
				>
				<n-button v-if="companyDraft.id && principal?.super" @click="openSharing">{{
					t('growth.sharing')
				}}</n-button
				><n-button
					v-if="companyDraft.id && can('material:manage')"
					@click="history('company', companyDraft.id)"
					>{{ t('growth.versions') }}</n-button
				>
			</n-space>
		</n-modal>
		<n-modal
			:show="workbenchActive && sharing"
			@update:show="sharing = $event"
			preset="card"
			:title="t('growth.sharing')"
			style="width: min(620px, 96vw)"
			><n-alert type="info" class="toolbar">{{ t('growth.share_hint') }}</n-alert
			><n-select v-model:value="shareDepts" multiple :options="departments" /><n-button
				class="toolbar"
				type="primary"
				@click="saveSharing"
				>{{ t('growth.save') }}</n-button
			></n-modal
		>
		<n-modal
			:show="workbenchActive && uploadModal"
			@update:show="uploadModal = $event"
			preset="card"
			:title="t(tab === 'products' ? 'growth.import_products' : 'growth.upload_materials')"
			style="width: min(720px, 96vw)">
			<n-alert type="info" class="toolbar">{{ t('growth.upload_hint') }}</n-alert>
			<n-space class="toolbar"
				><n-button
					v-if="tab !== 'products' || can('product:import')"
					:loading="templateLoading !== ''"
					@click="downloadTemplate(tab === 'products' ? 'products' : 'company')"
					>{{
						t(tab === 'products' ? 'growth.product_template' : 'growth.company_template')
					}}</n-button
				><n-button :loading="templateLoading !== ''" @click="downloadTemplate('images')">{{
					t('growth.image_template')
				}}</n-button></n-space
			>
			<p class="text-desc">{{ t('growth.template_hint') }}</p>
			<n-form-item :label="t('growth.model')"><AIModelSelect v-model:value="recognitionModel" kind="vision" /></n-form-item><n-checkbox v-model:checked="aiAllowed">{{ t('growth.ai_allowed') }}</n-checkbox>
			<input
				ref="fileInput"
				type="file"
				multiple
				:accept="
					tab === 'products'
						? '.xlsx,.zip,.jpg,.jpeg,.png,.webp'
						: '.xlsx,.zip,.pdf,.docx,.pptx,.txt,.md,.jpg,.jpeg,.png,.webp'
				"
				class="file-input"
				@change="selectFiles" />
			<n-space v-for="entry in uploads" :key="entry.name" justify="space-between"
				><span>{{ entry.name }}</span
				><n-tag
					:type="
						entry.state === 'failed' ? 'error' : entry.state === 'completed' ? 'success' : 'info'
					"
					>{{ stateText(entry.state) }}</n-tag
				><span>{{ entry.error }}</span></n-space
			>
			<n-button
				class="toolbar"
				type="primary"
				:loading="uploading"
				:disabled="!files.length"
				@click="upload"
				>{{ t('growth.start_import') }}</n-button
			>
		</n-modal>
		<n-drawer
			:show="workbenchActive && productDrawer"
			@update:show="productDrawer = $event"
			width="min(900px,96vw)"
			><n-drawer-content :title="t('growth.product')" closable>
				<n-form label-placement="top"
					><n-grid :cols="2" :x-gap="16"
						><n-form-item-gi label="SKU"
							><n-input v-model:value="productDraft.sku" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.category')"
							><n-input v-model:value="productDraft.category" /></n-form-item-gi></n-grid
					><n-form-item :label="t('growth.name')"
						><n-input v-model:value="productDraft.name" /></n-form-item
					><n-form-item :label="t('growth.specifications')"
						><n-dynamic-input v-model:value="productFields" :on-create="newField"
							><template #default="{ value }"
								><n-input v-model:value="value.key" :placeholder="t('growth.field')" /><n-input
									v-model:value="value.value"
									:placeholder="t('growth.value')" /></template></n-dynamic-input></n-form-item
				></n-form>
				<n-alert type="info">{{ t('growth.manual_protection') }}</n-alert>
				<n-space class="toolbar"
					><n-button
						v-if="can('product:manage')"
						type="primary"
						:loading="saving"
						@click="saveProduct"
						>{{ t('growth.save_draft') }}</n-button
					><n-popconfirm
						v-if="productDraft.id && can('product:publish')"
						@positive-click="publishProduct"
						><template #trigger
							><n-button>{{
								t(productDraft.published ? 'growth.withdraw' : 'growth.publish')
							}}</n-button></template
						>{{ t('growth.publish_confirm') }}</n-popconfirm
					><n-button
						v-if="productDraft.id && can('material:manage')"
						@click="history('product', productDraft.id)"
						>{{ t('growth.versions') }}</n-button
					></n-space
				>
				<h3>{{ t('growth.product_images') }}</h3>
				<n-image-group
					><n-space
						><n-image
							v-for="image in productImages.filter(i => thumbnails[i.asset_id])"
							:key="image.asset_id"
							:src="thumbnails[image.asset_id]"
							:alt="image.filename"
							:width="140"
							:height="110"
							object-fit="contain" /></n-space></n-image-group
				><n-empty v-if="!productImages.length" :description="t('growth.no_images')" />
				<div v-for="image in productImages" :key="image.asset_id" class="image-row">
					<span>{{ image.filename }}</span
					><n-tag>{{ t('growth.image_roles.' + image.role) }}</n-tag
					><n-button
						v-if="can('material:download')"
						@click="run(() => downloadMaterial(image.asset_id))"
						>{{ t('growth.download') }}</n-button
					><n-select
						v-if="can('product:manage') && can('material:manage')"
						:value="image.role"
						:options="imageRoleOptions"
						style="width: 160px"
						@update:value="role => setImageRole(image.asset_id, role)" />
				</div>
				<n-space
					v-if="productDraft.id && can('product:manage') && can('material:manage')"
					class="toolbar"
					><n-select
						v-model:value="attachAssetID"
						filterable
						remote
						:options="imageOptions"
						:loading="imageSearching"
						:placeholder="t('growth.choose_image')"
						style="width: 350px"
						@search="searchImages"
						@scroll="imageScroll" /><n-button
						:disabled="!attachAssetID"
						@click="setImageRole(attachAssetID!,'side')"
						>{{ t('growth.attach') }}</n-button
					></n-space
				>
			</n-drawer-content></n-drawer
		>
		<n-drawer
			:show="workbenchActive && assetDrawer"
			@update:show="assetDrawer = $event"
			width="min(950px,96vw)"
			><n-drawer-content :title="assetDraft.filename || t('growth.material')" closable>
				<n-form-item v-if="can('material:recognize')" :label="t('growth.model')"><AIModelSelect v-model:value="recognitionModel" kind="vision" /></n-form-item><n-alert v-if="assetDraft.error" type="warning">{{ assetDraft.error }}</n-alert>
				<n-descriptions :column="2" bordered class="toolbar"
					><n-descriptions-item :label="t('growth.status')">{{
						stateText(assetDraft.status)
					}}</n-descriptions-item
					><n-descriptions-item :label="t('growth.category')">{{
						stateText(assetDraft.category)
					}}</n-descriptions-item></n-descriptions
				>
				<n-form v-if="can('material:manage')" label-placement="top"
					><n-grid :cols="2" :x-gap="16"
						><n-form-item-gi :label="t('growth.category')"
							><n-select
								v-model:value="assetDraft.category"
								:options="categoryOptions" /></n-form-item-gi
						><n-form-item-gi :label="t('growth.visibility')"
							><n-select
								v-model:value="assetDraft.visibility"
								:options="
									['internal', 'restricted', 'public'].map(value => ({
										value,
										label: stateText(value),
									}))
								" /></n-form-item-gi></n-grid
					><n-checkbox v-model:checked="assetDraft.ai_allowed">{{
						t('growth.ai_allowed')
					}}</n-checkbox></n-form
				>
				<n-space class="toolbar"
					><n-button
						v-if="can('material:manage')"
						type="primary"
						:loading="saving"
						@click="saveAsset"
						>{{ t('growth.save') }}</n-button
					><n-button v-if="can('material:recognize')" @click="recognizeAsset">{{
						t('growth.recognize')
					}}</n-button
					><n-button
						v-if="can('material:download')"
						@click="run(() => downloadMaterial(assetDraft.id))"
						>{{ t('growth.download') }}</n-button
					><n-popconfirm v-if="can('material:publish')" @positive-click="publishAsset"
						><template #trigger
							><n-button>{{
								t(assetDraft.published ? 'growth.withdraw' : 'growth.publish')
							}}</n-button></template
						>{{ t('growth.publish_confirm') }}</n-popconfirm
					><n-button v-if="can('company:manage') && can('material:manage')" @click="applyCompany">{{
						t('growth.apply_company')
					}}</n-button
					><n-button v-if="can('material:manage')" @click="history('asset', assetDraft.id)">{{
						t('growth.versions')
					}}</n-button></n-space
				>
				<n-image
					v-if="thumbnails[assetDraft.id]"
					:src="thumbnails[assetDraft.id]"
					:alt="assetDraft.filename"
					:width="400"
					object-fit="contain" />
				<h3>{{ t('growth.recognition') }}</h3>
				<p>{{ assetExtracted.summary }}</p>
				<n-data-table :columns="factColumns" :data="list(assetExtracted.facts)" :scroll-x="650" />
				<n-alert v-if="list(assetExtracted.sku_candidates).length" type="info" class="toolbar"
					>{{ t('growth.sku_candidates') }}: {{ assetExtracted.sku_candidates.join(', ') }}</n-alert
				>
				<template v-if="assetDraft.category === 'product_sheet'">
					<h3>{{ t('growth.import_preview') }}</h3>
					<p>{{ t('growth.row_summary', { total: importTotal, review: reviewCount }) }}</p>
					<n-space class="toolbar"
						><n-button @click="loadImportRows">{{ t('growth.refresh') }}</n-button
						><n-button
							v-if="can('material:download')"
							@click="run(()=>workbook('exceptions',companyID!,assetDraft.id))"
							>{{ t('growth.download_exceptions') }}</n-button
						><n-button v-if="can('material:manage')" @click="mappingModal = true">{{
							t('growth.header_mapping')
						}}</n-button></n-space
					>
					<n-data-table :columns="importColumns" :data="importRows" :scroll-x="850" /><n-pagination
						v-model:page="importPage"
						:item-count="importTotal"
						:page-size="20"
						class="pagination"
						@update:page="loadImportRows" />
				</template> </n-drawer-content
		></n-drawer>
		<n-modal
			:show="workbenchActive && rowModal"
			@update:show="rowModal = $event"
			preset="card"
			:title="t('growth.review_row')"
			style="width: min(750px, 96vw)"
			><n-dynamic-input v-model:value="rowFields" :on-create="newField"
				><template #default="{ value }"
					><n-input v-model:value="value.key" /><n-input
						v-model:value="value.value" /></template></n-dynamic-input
			><n-button class="toolbar" type="primary" :loading="saving" @click="resolveRow">{{
				t('growth.confirm_import')
			}}</n-button></n-modal
		>
		<n-modal
			:show="workbenchActive && mappingModal"
			@update:show="mappingModal = $event"
			preset="card"
			:title="t('growth.header_mapping')"
			style="width: min(650px, 96vw)"
			><p>{{ t('growth.mapping_hint') }}</p>
			<n-dynamic-input v-model:value="mappingFields" :on-create="newField"
				><template #default="{ value }"
					><n-input v-model:value="value.key" :placeholder="t('growth.column_index')" /><n-input
						v-model:value="value.value"
						:placeholder="$t('ui.p6ccd21613bea')" /></template></n-dynamic-input
			><n-button class="toolbar" type="primary" @click="saveMapping">{{
				t('growth.save_retry')
			}}</n-button></n-modal
		>
		<n-modal
			:show="workbenchActive && historyModal"
			@update:show="historyModal = $event"
			preset="card"
			:title="t('growth.versions')"
			style="width: min(850px, 96vw)"
			><n-collapse
				><n-collapse-item
					v-for="v in versions"
					:key="v.id"
					:title="'v' + v.version + ' · ' + new Date(v.created_at).toLocaleString()">
					<pre>{{ JSON.stringify(object(v.snapshot), null, 2) }}</pre>
				</n-collapse-item></n-collapse
			></n-modal
		>
	</main>
</template>
<script setup lang="tsx">
import AIModelSelect from '@/features/ai/AIModelSelect.vue'
import { useWorkbenchActivity } from '@/features/workbench/context'
const workbenchActive = useWorkbenchActivity()
import { NButton, NTag } from 'naive-ui'
import { can, principal, workspace } from '@/features/workspace/api'
import {
	growth,
	object,
	list,
	cleanFields,
	fieldEntries,
	uploadMaterial,
	workbook,
	downloadMaterial,
	downloadUploadTemplate,
} from '@/features/growth/api'
import { Message, confirm } from '@/utils'
const { t, te, locale } = useI18n(),
	stateText = (v: string) => (te('growth.states.' + v) ? t('growth.states.' + v) : v || '—')
const companies = ref<any[]>([]),
	companyID = ref<number | null>(null),
	tab = ref('products'),
	rows = ref<any[]>([]),
	total = ref(0),
	page = ref(1),
	keyword = ref(''),
	status = ref<string | null>(null),
	loading = ref(false),
	saving = ref(false)
const currentCompany = computed(() => companies.value.find(c => c.id === companyID.value)),
	companyModal = ref(false),
	companyDraft = ref<any>({}),
	companyFields = ref<any[]>([]),
	sharing = ref(false),
	shareDepts = ref<number[]>([]),
	departments = ref<any[]>([])
const recognitionModel=ref('')
const uploadModal = ref(false),
	files = ref<File[]>([]),
	uploads = ref<any[]>([]),
	uploading = ref(false),
	aiAllowed = ref(true),
	fileInput = ref<HTMLInputElement>()
const productDrawer = ref(false),
	productDraft = ref<any>({}),
	productFields = ref<any[]>([]),
	productImages = ref<any[]>([]),
	attachAssetID = ref<string | null>(null),
	imageOptions = ref<any[]>([]),
	imageSearching = ref(false)
const assetDrawer = ref(false),
	assetDraft = ref<any>({}),
	assetExtracted = computed(() => object(assetDraft.value.extracted)),
	importRows = ref<any[]>([]),
	importTotal = ref(0),
	importPage = ref(1),
	reviewCount = ref(0),
	rowModal = ref(false),
	rowID = ref(0),
	rowFields = ref<any[]>([])
const mappingModal = ref(false),
	mappingFields = ref<any[]>([]),
	historyModal = ref(false),
	versions = ref<any[]>([])
const newField = () => ({ key: '', value: '' }),
	categoryOptions = computed(() =>
		[
			'unclassified',
			'product_sheet',
			'product_image',
			'company_profile',
			'brand',
			'catalog',
			'certificate',
			'sales_policy',
			'technical',
			'faq',
			'quotation',
			'internal',
		].map(value => ({ value, label: stateText(value) }))
	),
	imageRoleOptions = computed(() =>
		['main', 'side', 'label', 'packaging', 'dimensions', 'candidate'].map(value => ({
			value,
			label: t('growth.image_roles.' + value),
		}))
	)
const thumbnails = ref<Record<string, string>>({})
async function loadThumbnails(ids: string[]) {
	for (const id of ids) {
		if (thumbnails.value[id] || !alive) continue
		try {
			thumbnails.value[id] = await growth('image_preview', { asset_id: id })
		} catch {
			/* Unavailable images keep their filename and metadata. */
		}
	}
}
const statusOptions = computed(() =>
	(tab.value === 'products'
		? ['published', 'draft']
		: tab.value === 'assets'
			? ['uploaded', 'queued', 'processing', 'ready', 'failed']
			: ['queued', 'running', 'completed', 'failed', 'configuration_required', 'cancelled']
	).map(value => ({ value, label: stateText(value) }))
)
const btn = (label: string, action: () => any) =>
	h(
		NButton,
		{ text: true, type: 'primary', onClick: () => run(action) },
		{ default: () => t('growth.' + label) }
	)
const columns = computed(() => {
	const actions = {
		key: 'actions',
		title: t('growth.actions'),
		fixed: 'right',
		width: 220,
		render: (r: any) =>
			h(
				'div',
				{ class: 'actions' },
				tab.value === 'jobs'
					? [
							can('material:recognize') || can('outreach:run') || can('inquiry:manage')
								? btn(
										['failed', 'configuration_required', 'cancelled'].includes(r.status)
											? 'retry'
											: 'cancel',
										() => jobControl(r)
									)
								: null,
						]
					: [
							btn('view', () => (tab.value === 'products' ? editProduct(r) : openAsset(r.id))),
							(tab.value === 'products' ? can('product:manage') : can('material:delete'))
								? btn('archive', () => archive(r))
								: null,
						]
			),
	}
	if (tab.value === 'products')
		return [
			{ key: 'sku', title: 'SKU', width: 150 },
			{ key: 'name', title: t('growth.name'), minWidth: 230 },
			{ key: 'category', title: t('growth.category'), width: 150 },
			{
				key: 'published',
				title: t('growth.status'),
				width: 140,
				render: (r: any) => stateText(r.published ? 'published' : 'draft'),
			},
			{ key: 'version', title: t('growth.version'), width: 90 },
			actions,
		]
	if (tab.value === 'assets')
		return [
			{ key: 'filename', title: t('growth.filename'), minWidth: 280 },
			{
				key: 'category',
				title: t('growth.category'),
				width: 180,
				render: (r: any) => stateText(r.category),
			},
			{
				key: 'status',
				title: t('growth.status'),
				width: 130,
				render: (r: any) => stateText(r.status),
			},
			{
				key: 'visibility',
				title: t('growth.visibility'),
				width: 130,
				render: (r: any) => stateText(r.visibility),
			},
			{
				key: 'published',
				title: t('growth.publication'),
				width: 110,
				render: (r: any) => stateText(r.published ? 'published' : 'draft'),
			},
			actions,
		]
	return [
		{ key: 'kind', title: t('growth.job_type'), width: 180, render: (r: any) => stateText(r.kind) },
		{
			key: 'status',
			title: t('growth.status'),
			width: 180,
			render: (r: any) =>
				h(
					NTag,
					{ type: r.status === 'failed' ? 'error' : 'info' },
					{ default: () => stateText(r.status) }
				),
		},
		{
			key: 'progress',
			title: t('growth.progress'),
			width: 100,
			render: (r: any) => r.progress + '%',
		},
		{ key: 'error', title: t('growth.reason'), minWidth: 350 },
		actions,
	]
})
const factColumns = computed(() =>
	['key', 'value', 'evidence', 'location'].map(key => ({
		key,
		title: t('growth.' + key),
		minWidth: 150,
		ellipsis: { tooltip: true },
	}))
)
const importColumns = computed(() => [
	{ key: 'sheet', title: t('growth.sheet'), width: 150 },
	{ key: 'source_row', title: t('growth.row'), width: 80 },
	{ key: 'sku', title: 'SKU', width: 140 },
	{ key: 'state', title: t('growth.status'), width: 150, render: (r: any) => stateText(r.state) },
	{ key: 'reason', title: t('growth.reason'), minWidth: 220 },
	{
		key: 'actions',
		title: t('growth.actions'),
		width: 120,
		fixed: 'right',
		render: (r: any) =>
			r.state === 'review' && can('product:manage')
				? btn('review', () => {
						rowID.value = r.id
						rowFields.value = fieldEntries(r.proposed)
						rowModal.value = true
					})
				: null,
	},
])
let serial = 0,
	imagePage = 1,
	imageTotal = 0,
	imageKeyword = '',
	imageTimer: ReturnType<typeof setTimeout> | undefined,
	alive = true
async function run(fn: () => any) {
	try {
		return await fn()
	} catch {
		/* Shared API presents errors. */
	}
}
async function loadCompanies() {
	companies.value = list(await growth('companies'))
	if (!companyID.value || !companies.value.some(c => c.id === companyID.value))
		companyID.value = companies.value[0]?.id || null
}
async function refresh() {
	if (!companyID.value) return
	const current = ++serial
	loading.value = true
	try {
		const result = await growth('list', {
			kind: tab.value,
			company_id: companyID.value,
			page: page.value,
			keyword: keyword.value,
			status: status.value || '',
		})
		if (current === serial) {
			rows.value = list(result.list)
			total.value = Number(result.total) || 0
		}
	} catch {
		/* The shared API displays errors. */
	} finally {
		if (current === serial) loading.value = false
	}
}
function search() {
	page.value = 1
	return refresh()
}
function changeTab() {
	status.value = null
	keyword.value = ''
	search()
}
function changeCompany() {
	productDrawer.value = false
	assetDrawer.value = false
	search()
}
function editCompany(row?: any) {
	companyDraft.value = { id: 0, name: '', brand: '', domains: [], ...row }
	companyDraft.value.domains = list(object(row).domains)
	companyFields.value = fieldEntries(row?.profile)
	companyModal.value = true
}
async function saveCompany() {
	saving.value = true
	try {
		const id = await growth('save_company', {
			id: companyDraft.value.id,
			data: { ...companyDraft.value, profile: cleanFields(companyFields.value) },
		})
		companyDraft.value.id = id
		companyID.value = id
		await loadCompanies()
		Message.success(t('growth.saved'))
		await refresh()
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function publishCompany() {
	await run(async () => {
		await growth('publish_company', { id: companyDraft.value.id })
		await loadCompanies()
		Message.success(t('growth.published'))
	})
}
async function openSharing() {
	await run(async () => {
		const result = await workspace('list', { type: 'departments', page_size: 200 })
		departments.value = list(result?.list || result).map((r: any) => ({
			value: r.id,
			label: r.name,
		}))
		shareDepts.value = list(await growth('company_grants', { id: companyDraft.value.id })).map(
			(r: any) => r.dept_id
		)
		sharing.value = true
	})
}
async function saveSharing() {
	await run(async () => {
		await growth('share_company', {
			id: companyDraft.value.id,
			data: { dept_ids: shareDepts.value },
		})
		sharing.value = false
	})
}
function openUpload() {
	files.value = []
	uploads.value = []
	uploadModal.value = true
}
const templateLoading = ref('')
async function downloadTemplate(kind: string) {
	if (!companyID.value || templateLoading.value) return
	templateLoading.value = kind
	try {
		await downloadUploadTemplate(kind, companyID.value, locale.value)
	} catch {
		/* Shared request handler displays the download error. */
	} finally {
		templateLoading.value = ''
	}
}
function selectFiles(event: Event) {
	files.value = Array.from((event.target as HTMLInputElement).files || [])
	uploads.value = files.value.map(file => ({ name: file.name, state: 'queued', error: '' }))
}
async function upload() {
	if (!companyID.value || uploading.value) return
	uploading.value = true
	const purpose = tab.value === 'products' ? 'products' : 'company'
	try {
		for (let i = 0; i < files.value.length; i++) {
			uploads.value[i].state = 'running'
			try {
				await uploadMaterial(files.value[i], companyID.value, purpose, aiAllowed.value, recognitionModel.value)
				uploads.value[i].state = 'completed'
			} catch (e: any) {
				uploads.value[i].state = 'failed'
				uploads.value[i].error = e.message
			}
		}
		await refresh()
	} finally {
		uploading.value = false
	}
}
async function editProduct(row?: any) {
	productDraft.value = {
		id: 0,
		company_id: companyID.value,
		sku: '',
		name: '',
		category: '',
		published: false,
	}
	productFields.value = []
	productImages.value = []
	if (row?.id) {
		const result = await growth('product', { id: row.id })
		productDraft.value = result.product
		productFields.value = fieldEntries(result.product.fields)
		productImages.value = list(result.images)
		void loadThumbnails(productImages.value.map(i => i.asset_id))
	}
	productDrawer.value = true
	attachAssetID.value = null
	imageOptions.value = []
	searchImages('')
}
async function saveProduct() {
	saving.value = true
	try {
		productDraft.value.id = await growth('save_product', {
			id: productDraft.value.id,
			data: { ...productDraft.value, fields: cleanFields(productFields.value) },
		})
		await editProduct(productDraft.value)
		await refresh()
		Message.success(t('growth.saved'))
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function publishProduct() {
	await run(async () => {
		await growth('publish_product', {
			id: productDraft.value.id,
			data: { publish: !productDraft.value.published },
		})
		await editProduct(productDraft.value)
		await refresh()
	})
}
async function fetchImages(append = false) {
	if (!can('material:list') || !companyID.value) return
	imageSearching.value = true
	try {
		const result = await growth('list', {
			kind: 'assets',
			company_id: companyID.value,
			page: imagePage,
			keyword: imageKeyword,
		})
		imageTotal = Number(result.total) || 0
		const options = list(result.list)
			.filter(r => r.mime?.startsWith('image/'))
			.map(r => ({ value: r.id, label: r.filename }))
		imageOptions.value = append ? [...imageOptions.value, ...options] : options
	} finally {
		imageSearching.value = false
	}
}
function searchImages(value: string) {
	clearTimeout(imageTimer)
	imageKeyword = value
	imagePage = 1
	imageTimer = setTimeout(() => run(() => fetchImages()), 250)
}
function imageScroll(event: Event) {
	const el = event.target as HTMLElement
	if (
		!imageSearching.value &&
		imagePage * 20 < imageTotal &&
		el.scrollHeight - el.scrollTop - el.clientHeight < 30
	) {
		imagePage++
		run(() => fetchImages(true))
	}
}
async function setImageRole(assetID: string, role: string) {
	await run(async () => {
		await growth('attach_image', { id: productDraft.value.id, asset_id: assetID, data: { role } })
		await editProduct(productDraft.value)
	})
}
async function openAsset(id: string) {
	const result = await growth('asset', { asset_id: id })
	assetDraft.value = result.asset
	if (result.asset.mime?.startsWith('image/')) void loadThumbnails([id])
	assetDrawer.value = true
	importPage.value = 1
	mappingFields.value = fieldEntries(object(result.asset.extracted).header_mapping)
	if (result.asset.category === 'product_sheet') await loadImportRows()
}
async function saveAsset() {
	saving.value = true
	try {
		await growth('update_asset', { asset_id: assetDraft.value.id, data: assetDraft.value })
		await openAsset(assetDraft.value.id)
		await refresh()
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function recognizeAsset() {
	await run(async () => {
		await growth('recognize', { asset_id: assetDraft.value.id, data:{model:recognitionModel.value} })
		Message.success(t('growth.queued'))
		await refresh()
	})
}
async function publishAsset() {
	await run(async () => {
		await growth('publish_asset', {
			asset_id: assetDraft.value.id,
			data: { publish: !assetDraft.value.published },
		})
		await openAsset(assetDraft.value.id)
		await refresh()
	})
}
async function applyCompany() {
	await run(async () => {
		await growth('apply_company', { asset_id: assetDraft.value.id, data:{model:recognitionModel.value} })
		await loadCompanies()
		Message.success(t('growth.saved'))
	})
}
async function loadImportRows() {
	await run(async () => {
		const result = await growth('import_rows', {
			asset_id: assetDraft.value.id,
			page: importPage.value,
		})
		importRows.value = list(result.list)
		importTotal.value = Number(result.total) || 0
		reviewCount.value =
			Number(list(result.counts).find((r: any) => r.state === 'review')?.count) || 0
	})
}
async function resolveRow() {
	saving.value = true
	try {
		await growth('resolve_row', { id: rowID.value, data: cleanFields(rowFields.value) })
		rowModal.value = false
		await loadImportRows()
	} catch {
		/* The shared API displays errors. */
	} finally {
		saving.value = false
	}
}
async function saveMapping() {
	await run(async () => {
		await growth('mapping', {
			asset_id: assetDraft.value.id,
			data: cleanFields(mappingFields.value),
		})
		mappingModal.value = false
	})
}
async function history(kind: string, id: string | number) {
	await run(async () => {
		versions.value = list(
			await growth('versions', {
				company_id: companyID.value,
				kind,
				data: { resource_id: String(id) },
			})
		)
		historyModal.value = true
	})
}
function archive(row: any) {
	confirm({
		title: t('growth.archive'),
		content: t('growth.archive_confirm'),
		confirmType: 'error',
		onConfirm: async () => {
			await growth('archive', {
				kind: tab.value,
				id: tab.value === 'products' ? row.id : 0,
				asset_id: tab.value === 'assets' ? row.id : '',
			})
			await refresh()
		},
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
let poll: ReturnType<typeof setInterval>
onMounted(async () => {
	await run(async () => {
		await loadCompanies()
		tab.value = can('product:list') ? 'products' : 'assets'
		await refresh()
	})
	poll = setInterval(() => {
		if (!workbenchActive.value) return
		if (
			alive &&
			document.visibilityState === 'visible' &&
			(tab.value === 'jobs' || rows.value.some(r => ['queued', 'processing'].includes(r.status)))
		)
			refresh()
	}, 5000)
})
onBeforeUnmount(() => {
	alive = false
	serial++
	clearInterval(poll)
	clearTimeout(imageTimer)
})
</script>
<style scoped>
.growth-page {
	padding: 26px;
	min-width: 0;
}
.growth-page header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	gap: 16px;
	margin-bottom: 24px;
}
.growth-page h1 {
	font-size: 26px;
	margin: 0 0 8px;
}
.growth-page header p {
	opacity: 0.65;
	margin: 0;
	max-width: 850px;
}
.toolbar {
	margin: 18px 0;
}
.pagination {
	margin-top: 20px;
	justify-content: flex-end;
}
.actions {
	display: flex;
	gap: 18px;
	flex-wrap: wrap;
}
.image-row {
	display: flex;
	align-items: center;
	gap: 16px;
	border-bottom: 1px solid #8883;
	padding: 14px 0;
}
.image-row > span {
	flex: 1;
	overflow-wrap: anywhere;
}
.file-input {
	display: block;
	margin: 24px 0;
	width: 100%;
}
pre {
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	font-size: 12px;
}
@media (max-width: 700px) {
	.growth-page {
		padding: 14px;
	}
	.growth-page header {
		align-items: flex-start;
		flex-direction: column;
	}
	.image-row {
		flex-wrap: wrap;
	}
}
</style>
