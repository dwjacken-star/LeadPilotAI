import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import {growthValue,growthDetailFields,growthEmails,growthError,growthVerificationFields,growthFactsFields} from '../src/features/growth/display.ts'
const messages=JSON.parse(fs.readFileSync(new URL('../src/i18n/lang/zh.json',import.meta.url),'utf8'))
const get=k=>k.split('.').reduce((a,b)=>a?.[b],messages)
const ctx={locale:'zh',te:key=>typeof get(key)==='string',t:(key,p={})=>String(get(key)??key).replace(/\{(\w+)\}/g,(_,name)=>p[name]??'')}
test('business relations use supplied names and never raw foreign keys',()=>{
 const row={company_id:7,company_name:'杭州佳枫',plan_id:9,plan_name:'东南亚计划',run_id:44,run_name:'东南亚计划 · 2026-11-07 22:30',task_id:22,task_name:'开发信主题'}
 for(const key of ['company_id','plan_id','run_id','task_id']){const value=growthValue('candidates',key,row,ctx);assert.ok(!/^\d+$/.test(value));assert.ok(!value.includes('growth.'))}
 assert.equal(growthValue('leads','company_id',{company_id:7},ctx),'关联资料暂不可用')
})
test('countries language flags errors and verification display readable labels',()=>{
 assert.equal(growthValue('leads','country',{country:'PH'},ctx),'菲律宾');assert.equal(growthValue('leads','language',{language:'th'},ctx),'泰语')
 assert.equal(growthValue('leads','suppressed',{suppressed:'false'},ctx),'否');assert.equal(growthValue('jobs','kind',{kind:'draft'},ctx),'生成开发信')
 assert.match(growthError('daily AI budget reached',ctx),/每日 AI 预算/)
 assert.match(growthError('Serper request failed (HTTP 404)',ctx),/HTTP 404/)
 assert.match(growthError('catch_all',ctx),/Catch-all/)
 const rows=growthVerificationFields(JSON.stringify({status:'unknown',code:'catch_all',stage:'smtp',checked_at:'2026-11-07T10:00:00Z'}),ctx)
 assert.equal(rows.find(v=>v.key==='status').value,'无法确认');assert.ok(rows.every(v=>!v.label.startsWith('growth.')))
})
test('collected emails are readable before validation and machine fields stay out of primary details',()=>{
 assert.deepEqual(growthEmails('["sales@example.com","support@example.com","sales@example.com"]'),['sales@example.com','support@example.com'])
 const fields=growthDetailFields('candidates',{company_id:1,company_name:'佳枫',company:'客户公司',emails:'["sales@example.com"]',identity_key:'internal',facts:'{}',state:'failed',created_at:'2026-11-07T10:00:00Z'},ctx)
 assert.ok(!fields.some(v=>['identity_key','facts','emails'].includes(v.key)));assert.ok(fields.every(v=>!v.label.startsWith('growth.')))
 const facts=growthFactsFields({summary:'Automotive distributor',relevant:false,confidence:0,evidence:'Published text'},ctx)
 assert.equal(facts.find(v=>v.key==='relevant').value,'否');assert.equal(facts.find(v=>v.key==='confidence').value,'0%')
})
