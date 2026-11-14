import test from 'node:test'
import assert from 'node:assert/strict'
import {createSSEDecoder,downloadFilename,normalizeResponseBody} from '../src/utils/response.ts'
import {normalizeEmailHTML} from '../src/utils/email-content.ts'
import {formatWorkspaceCell} from '../src/features/workspace/display.ts'
import {readFileSync} from 'node:fs'
import {createRequire} from 'node:module'

test('workspace cells render business names and localized descriptions without changing IDs or counts',()=>{
 const {createI18n}=createRequire(import.meta.url)('vue-i18n')
 const messages=JSON.parse(readFileSync(new URL('../src/i18n/lang/zh.json',import.meta.url),'utf8'))
 const {t}=createI18n({legacy:false,locale:'zh',messages:{zh:messages},warnHtmlMessage:false}).global
 const row={id:8,group_id:901,group_name:'欧洲采购商',dept_id:3,dept_name:'外贸部门',parent_id:0,role_names:['销售经理'],group_names:['欧洲采购商','北美客户'],template_name:'报价模板',contact_id:44,contact_email:'buyer@example.com',scope:'department',scope_key:'3',scope_name:'外贸部门',task_id:91,task_name:'新品推广',recipient_count:123}
 for(const [kind,key,expected] of [['tags','group_id','欧洲采购商'],['contacts','group_id','欧洲采购商'],['users','dept_id','外贸部门'],['departments','parent_id','顶级部门'],['users','role_names','销售经理'],['tasks','group_names','欧洲采购商、北美客户'],['tasks','template_name','报价模板'],['enrollments','contact_id','buyer@example.com'],['runs','task_id','新品推广'],['policies','scope_key','外贸部门'],['reports','recipient_count','123']]) assert.equal(formatWorkspaceCell(kind,key,row,t),expected)
 assert.equal(row.group_id,901)
 assert.equal(formatWorkspaceCell('tags','group_id',{group_id:999},t),'已删除或无权查看')
 assert.equal(formatWorkspaceCell('roles','data_scope',{data_scope:'department'},t),'本部门')
 assert.equal(formatWorkspaceCell('schedules','cadence',{cadence:'weekly'},t),'每周')
 assert.equal(formatWorkspaceCell('tags','active',{active:false},t),'已停用')
 assert.equal(formatWorkspaceCell('contacts','status',{status:0},t),'未确认')
 assert.equal(formatWorkspaceCell('tasks','delivery_counts',{delivery_counts:'{"pending":4,"accepted":2}'},t),'待发送：4；SMTP已接受：2')
 assert.equal(formatWorkspaceCell('roles','permissions',{permissions:['contact:list','campaign:publish']},t),'联系人：查看；任务：发布')
 assert.equal(formatWorkspaceCell('audit','action',{action:'save:groups'},t),'保存 · 分组')
 const policy=formatWorkspaceCell('policies','policy',{policy:{per_day:1000,weekdays:[1,5]}},t)
 assert.ok(policy.includes('24小时最多（封）：1000'));assert.ok(policy.includes('周一、周五'));assert.ok(!policy.includes('weekdays'))
 assert.equal(formatWorkspaceCell('tasks','wait_reason',{wait_reason:'等待发送间隔：department:3',dept_name:'外贸部门'},t),'等待发送间隔 · 外贸部门')
 assert.equal(formatWorkspaceCell('tasks','wait_reason',{wait_reason:'等待滚动配额：task:91',task_name:'新品推广'},t),'等待滚动配额 · 新品推广')
 assert.equal(formatWorkspaceCell('deliveries','delivery_state',{delivery_state:'validation_hold'},t),'历史暂缓')
 assert.equal(formatWorkspaceCell('contacts','verification_status',{verification_status:'valid',verification_code:'mx_valid'},t),'DNS/MX 已通过，待 SMTP 验证')
 assert.ok(formatWorkspaceCell('deliveries','last_error',{last_error:'Recipient validation unknown: buyer@example.net (catch_all)'},t).includes('Catch-all'))
})
test('import keyword hints and sample emails compile and display literal characters',()=>{
 const {createI18n}=createRequire(import.meta.url)('vue-i18n')
 for(const locale of ['zh','en','ja']){
  const messages=JSON.parse(readFileSync(new URL('../src/i18n/lang/'+locale+'.json',import.meta.url),'utf8'))
  const i18n=createI18n({legacy:false,locale,messages:{[locale]:messages},warnHtmlMessage:false})
  const hint=i18n.global.t('repair.filter_hint');assert.ok(hint.includes('@example.com'));assert.ok(!hint.includes("{'@'}"))
  assert.ok(i18n.global.t('contacts.subscribers.import.pastePlaceholder.1').includes('@example.com'))
  const sample=i18n.global.t('contacts.subscribers.import.pastePlaceholder.2');assert.ok(sample.includes('@example.com'));assert.ok(sample.includes('{'))
 }
})
import {sortMenuRoutes} from '../src/router/menu-order.ts'
test('renamed and unknown menu titles never create undefined routes',()=>{
 const routes=[{path:'/other',meta:{key:'extension',title:'Unknown'}},{path:'/crm',meta:{key:'crm',title:'客户管理'}},{path:'/overview',meta:{key:'overview',title:'首页'}},{path:'/workspace',meta:{key:'workspace',title:'Team workspace'}}]
 const sorted=sortMenuRoutes(routes)
 assert.deepEqual(sorted.map(route=>route.path),['/workspace','/overview','/crm','/other'])
 assert.equal(sorted.length,routes.length);assert.ok(sorted.every(route=>typeof route.path==='string'))
})
test('preview and SMTP share the email formatting contract',()=>{
 const fixtures=JSON.parse(readFileSync(new URL('../../internal/service/public/testdata/email_content.json',import.meta.url),'utf8'))
 for(const fixture of fixtures){const result=normalizeEmailHTML(fixture.input);assert.equal(result,fixture.expected,fixture.name);assert.equal(normalizeEmailHTML(result),result,fixture.name+' is idempotent')}
})
test('SSE survives every possible frame boundary and CRLF',()=>{
 const events=[{content:'客户你好\n<p>Hello</p>'},{content:'more'},{is_end:true}]
 const input=': connected\r\n\r\n'+events.map(event=>'data: '+JSON.stringify(event)+'\r\n\r\n').join('')+'data: [DONE]\r\n\r\n'
 for(let split=1;split<input.length;split++){const actual=[],decoder=createSSEDecoder(event=>actual.push(event));decoder.push(input.slice(0,split));decoder.push(input.slice(split));decoder.finish();assert.deepEqual(actual,events)}
 const chars=[],decoder=createSSEDecoder(event=>chars.push(event));for(const char of input)decoder.push(char);decoder.finish();assert.deepEqual(chars,events)
})
test('JSON error blobs are decoded while JSON downloads stay files',async()=>{
 const data={code:500,success:false,msg:'Not yet achieved'},blob=new Blob([JSON.stringify(data)],{type:'application/json'})
 assert.deepEqual(await normalizeResponseBody(blob),data)
 assert.equal(await normalizeResponseBody(blob,'attachment; filename="export.json"'),blob)
 assert.equal(downloadFilename("attachment; filename*=UTF-8''%E6%A8%A1%E6%9D%BF.xlsx"),'模板.xlsx')
 assert.equal(downloadFilename('attachment; filename="contacts.xlsx"'),'contacts.xlsx')
})

test('intelligence menu is between social marketing and AI outreach',()=>{
 const routes=['outreach','intelligence','social'].map(key=>({path:'/'+key,meta:{key}}))
 assert.deepEqual(sortMenuRoutes(routes).map(r=>r.meta.key),['social','intelligence','outreach'])
})
