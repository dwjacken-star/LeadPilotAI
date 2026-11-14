import test from 'node:test'
import assert from 'node:assert/strict'
import {routeAllowed} from '../src/features/growth/access.ts'
import {workspacePermissionLabel} from '../src/features/workspace/display.ts'
import {sectionTabs,sectionEntry} from '../src/features/workspace/sections.ts'
import fs from 'node:fs'
import {marketChoices,retainChoice,countryChanged,normalizeDeliveryOptions} from '../src/features/growth/options.ts'
import {parseImportKeywords} from '../src/features/contacts/import-state.ts'
const markets=JSON.parse(fs.readFileSync(new URL('../src/features/growth/market-data.json',import.meta.url),'utf8'))
const catalog=JSON.parse(fs.readFileSync(new URL('../src/i18n/lang/zh.json',import.meta.url),'utf8'))
const translate=key=>key.split('.').reduce((value,key)=>value?.[key],catalog)??key

test('new plans get pictured delivery defaults while old plans keep their actual behavior',()=>{
 assert.deepEqual(normalizeDeliveryOptions({}),{warmup:false,unsubscribe:true,track_open:true,track_click:true})
 assert.deepEqual(normalizeDeliveryOptions({},true),{warmup:false,unsubscribe:true,track_open:false,track_click:false})
 const saved={warmup:true,unsubscribe:false,track_open:false,track_click:true}
 assert.deepEqual(normalizeDeliveryOptions(saved,true),saved)
 assert.deepEqual(normalizeDeliveryOptions({unsubscribe:null},true),{warmup:false,unsubscribe:true,track_open:false,track_click:false})
 assert.deepEqual(parseImportKeywords('test,info；noreply，sales;test\n support\r\n'),['test','info','noreply','sales','support'])
 assert.deepEqual(parseImportKeywords(' \n，;'),[])
})

test('market selectors cover ISO countries, valid timezones and preserve saved selections',()=>{
 assert.equal(markets.countries.length,249)
 assert.equal(new Set(markets.countries.map(c=>c.code)).size,249)
 for(const zone of markets.timezones)new Intl.DateTimeFormat('en',{timeZone:zone})
 const zh=marketChoices(markets,'zh'),en=marketChoices(markets,'en')
 assert.match(zh.countries.find(c=>c.value==='US').label,/美国/)
 assert.match(en.countries.find(c=>c.value==='DE').label,/Germany/)
 const market={country:'',language:'',timezone:''};countryChanged(market,'DE',markets)
 assert.deepEqual(market,{country:'DE',language:'de',timezone:'Europe/Berlin'})
 countryChanged(market,'US',markets);assert.equal(market.timezone,'America/New_York')
 for(const country of markets.countries){assert.ok(markets.languages.includes(country.language));new Intl.DateTimeFormat('en',{timeZone:country.timezone})}
 assert.equal(retainChoice([],'custom-model')[0].value,'custom-model')
 assert.equal(retainChoice([{value:'en',label:'English'}],'en').length,1)
})
test('existing role grants control the integrated menus without exposing legacy administration',()=>{
 const account={super:false,roles:[{permissions:['product:list','company:list','outreach:list','crm:list']}]}
 for(const path of ['/materials','/outreach','/crm','/workspace'])assert.equal(routeAllowed(account,path),true,path)
 for(const path of ['/settings','/domain','/mailbox','/unknown'])assert.equal(routeAllowed(account,path),false,path)
 assert.equal(routeAllowed({super:false,roles:[{permissions:['inquiry:list']}]},'/outreach'),true)
 assert.equal(routeAllowed({super:false,roles:[]},'/materials'),false)
 assert.equal(routeAllowed(null,'/materials'),false)
 assert.equal(routeAllowed({super:true},'/settings'),true)
})
test('new permission names are translated into business descriptions',()=>{
 for(const code of ['company:list','company:publish','material:upload','material:recognize','material:delete','product:export','outreach:run','inquiry:manage','crm:ai_draft','growth:settings']){
  const label=workspacePermissionLabel(code,translate);assert.ok(!label.includes('unknown'),label);assert.ok(!label.includes('growth.'),label);assert.ok(!label.includes('未知'),label)
 }
})

test('business menu grants expose scoped entries but never legacy administration',()=>{
 const account=codes=>({super:false,roles:[{permissions:codes}]})
 const paths=['/market','/market/task','/template','/contacts','/contacts/group','/contacts/subscribers','/contacts/tags']
 const granted=account(['campaign:list','template:list','contact:list'])
 for(const path of paths)assert.equal(routeAllowed(granted,path),true,path)
 for(const path of ['/market/task/edit','/market/template','/template/ai-template/1','/contacts/suspend','/contacts/settings/1','/automation','/settings','/mailbox','/domain']){
  assert.equal(routeAllowed(granted,path),false,path)
  assert.equal(routeAllowed({super:true},path),true,path)
 }
 for(const path of paths){
  assert.equal(routeAllowed(account(['campaign:create']),path),false,'mailbox writing role: '+path)
  assert.equal(routeAllowed(account([]),path),false,path)
 }
 assert.equal(routeAllowed(account(['report:list']),'/market'),false)
 assert.equal(routeAllowed(account(['campaign:list']),'/market/task/analytics/1'),true)
 assert.equal(routeAllowed(account(['campaign:list','campaign:create']),'/market/task/edit'),true)
 assert.equal(routeAllowed(account(['campaign:list','campaign:update']),'/market/task/edit'),true)
 assert.equal(routeAllowed(account(['campaign:list']),'/market/task/edit'),false)
 assert.equal(routeAllowed(account(['overview:view']),'/overview'),true)
 assert.equal(routeAllowed(account(['campaign:list']),'/overview'),false)
 assert.equal(routeAllowed(account(['contact:list']),'/template'),false)
 assert.equal(routeAllowed(account(['template:list']),'/contacts'),false)
 assert.deepEqual(sectionTabs.marketing,['tasks','schedules','automations','reports'])
 assert.deepEqual(sectionTabs.templates,['templates'])
 assert.deepEqual(sectionTabs.contacts,['groups','contacts','tags'])
 assert.equal(sectionEntry('contacts','/contacts/subscribers'),'contacts')
 assert.equal(sectionEntry('contacts','/contacts/tags'),'tags')
 assert.equal(sectionEntry('templates','/template'),'templates')
})

test('intelligence and social action labels are complete and distinct in every language',()=>{
 for(const language of ['zh','en','ja','de','tr','vi']){
  const messages=JSON.parse(fs.readFileSync(new URL(`../src/i18n/lang/${language}.json`,import.meta.url),'utf8'))
  const translate=key=>{
   const value=key.split('.').reduce((value,key)=>value?.[key],messages)
   assert.equal(typeof value,'string',`${language}: ${key}`)
   return value
  }
  for(const code of ['overview:view','campaign:update','campaign:delete']){
   const label=workspacePermissionLabel(code,translate)
   assert.ok(!label.includes(translate('workspaceDisplay.unknown')),language+': '+code)
  }
  const labels=['view','manage','run','review','publish','settings','export'].map(action=>workspacePermissionLabel('intelligence:'+action,translate))
  assert.equal(new Set(labels).size,7,language)
  for(const area of ['account','plan','interaction'])
   assert.notEqual(workspacePermissionLabel(`social:${area}:view`,translate),workspacePermissionLabel(`social:${area}:manage`,translate),language+': '+area)
 }
})
