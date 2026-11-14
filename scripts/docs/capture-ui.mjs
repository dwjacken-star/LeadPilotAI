import fs from 'node:fs/promises'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { createServer } from 'node:http'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'

const root = process.cwd()
const modulePath = process.env.PLAYWRIGHT_MODULE || path.join(root, '.tools/social-browser-runtime/node_modules/playwright-core/index.mjs')
const { chromium } = await import(pathToFileURL(modulePath).href)
const executablePath = process.env.DOC_BROWSER || 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
const dist = path.join(root, 'core/frontend/dist')
const output = path.join(root, 'docs/handbook/assets/screenshots')
await fs.mkdir(output, { recursive: true })
const dates = '2026-11-14T08:00:00Z'
const permissions = ['campaign:list','campaign:create','campaign:publish','contact:list','contact:create','template:list','template:create','company:list','product:list','material:list','outreach:list','intelligence:view','social:plan:view','crm:list']
const text = (locale, zh, en) => locale === 'zh' ? zh : en
function fixtures(locale) {
 const company = { id: 1, name: text(locale,'示例制造有限公司','Example Manufacturing Ltd.'), brand: 'DemoBrand', domains: ['example.com'], profile: { business: 'Industrial components' }, status: 'published', version: 1, dept_id: 1, owner_id: 1 }
 const product = { id: 11, company_id: 1, sku: 'DEMO-001', name: text(locale,'工业连接组件','Industrial connector'), category: text(locale,'工业零部件','Industrial components'), status: 'published', version: 1, fields: { material: 'steel', specification: 'Demo specification' }, attrs: {}, images: [], created_at: dates, updated_at: dates }
 const task = { id: 21, task_id: 21, name: text(locale,'样例产品介绍邮件','Demo product introduction'), task_name: 'Demo campaign', subject: text(locale,'了解我们的工业组件','Explore our industrial components'), sender: 'sales@example.com', recipient_count: 12, sent_count: 0, success_count: 0, failure_count: 0, pending_count: 12, total: 12, pause: 1, task_process: 0, status: 'draft', task_status: 0, send_count: 0, dept_id: 1, owner_id: 1, created_at: dates, addtime: 1794643200, campaign: { group_ids: [1], senders: ['sales@example.com'], template_id: 1, subject: 'Demo introduction', policy: {} }, policy: {}, full_name: 'DemoBrand' }
 const dept = { id: 1, name: text(locale,'示例销售部','Demo Sales'), parent_id: null, active: true, owner_id: 1, dept_id: 1, created_at: dates }
 const role = { id: 1, role_id: 1, role_name: text(locale,'营销操作员','Marketing operator'), data_scope: 'department', permissions, custom_depts: [], status: 1 }
 const contact = { id: 1, email: 'buyer@example.org', group_id: 1, group_name: text(locale,'示例客户组','Demo customer group'), attribs: { name: text(locale,'示例采购员','Demo buyer'), company: 'Example Buyer Ltd.' }, status: 1, active: 1, dept_id: 1, owner_id: 1, owner_name: text(locale,'文档演示账号','Documentation demo'), dept_name: dept.name, created_at: dates, tags: [] }
 const plan = { id: 31, name: text(locale,'示例工业采购客户计划','Demo industrial buyer plan'), company_id: 1, company_name: company.name, status: 'paused', config: { markets: [{ country: 'US', language: 'en', timezone: 'America/New_York' }], product_ids: [11], senders: ['sales@example.com'], policy: {}, auto_send: false }, created_at: dates, last_run_at: null, next_run_at: null, version: 1 }
 const account = { id: 41, name: text(locale,'示例公司主页','Demo company Page'), platform: 'facebook', target_type: 'page', publish_mode: 'api', status: 'disconnected', external_id: 'demo-page', capabilities: { manual_publish: false }, dept_id: 1, provider_name: 'Demo authorizer', token_expires_at: null }
 const social = { id: 51, name: text(locale,'示例新品社媒草稿','Demo product launch draft'), company_id: 1, company_name: company.name, status: 'draft', platform: 'facebook', platforms: ['facebook', 'linkedin'], config: { platforms: ['facebook', 'linkedin'], markets: [], accounts: [41], account_ids: [41] }, created_at: dates, updated_at: dates, version_count: 2, variants: [], config_version: 1 }
 const workflow = { id: 61, name: text(locale,'示例采购意向工作流','Demo purchase-intent workflow'), code: 'WF02', company_id: 1, company_name: company.name, pack_id: 1, pack_name: text(locale,'工业产品','Industrial products'), version: 1, status: 'paused', enabled: false, config: { markets: [{ country: 'US', language: 'en', timezone: 'America/New_York' }], channels: ['email'], budget: { calls: 20, sends: 0 }, cadence: '6h' }, created_at: dates, last_run_at: null, next_run_at: null }
 const skillNames = ['Global News Monitor','Market Trend Analyst','Product Hotspot Detector','Purchase Intent Detector','B2B Company Finder','Contact Intelligence','Data Cleaner','Lead Scoring','Product Matching','AI Content Creator','Campaign Orchestrator','Reply Intelligence','Growth Analytics','Data Sync & Audit']
 const skillTools = [['serper.search','serper.scrape'],['ai.structured'],['ai.structured'],['serper.search','ai.structured'],['serper.search'],['serper.scrape','ai.structured'],['email.dns_mx_smtp_catchall','suppression.check'],['rules.score'],['ai.structured','product.catalog'],['ai.structured'],['maplemail.queue','social.queue','whatsapp.template'],['inbox.classify','ai.structured','crm.followup'],['db.aggregate'],['crm.upsert','audit.append']]
 const skills = skillNames.map((name,i) => ({ id: 'S'+String(i+1).padStart(2,'0'), name, group: i<4?'intelligence':i<8?'prospecting':i<11?'marketing':'conversion', input: ['industry','markets'], output: ['evidence'], tools: skillTools[i], gate: 'human_review', permission: 'intelligence:run' }))
 const pack = { id: 1, slug: 'industrial_demo', name: text(locale,'工业产品演示包','Industrial products demo pack'), dept_id: 1, version: 1, config: { object_type: 'product', terms: ['industrial components'], buyer_types: ['distributor'], intent_terms: ['procurement'], fields: [{key:'specification',label:'Specification',type:'text'}], description: text(locale,'仅用于文档截图的行业配置','Industry configuration for documentation only') } }
 const customer = { id: 71, name: text(locale,'示例采购客户','Demo procurement customer'), email: 'buyer@example.org', company: 'Example Buyer Ltd.', stage: 'inquiry', next_followup_at: '2026-11-15T08:00:00Z', last_activity_at: dates, dept_id: 1 }
 const mailbox = { username:'sales@example.com', full_name: 'Demo Sales', domain:'example.com', local_part:'sales', active:1, is_admin:0, quota:5368709120, used_quota:0, quota_active:1, create_time:1794643200, mx:1, password:'', password_encode:'' }
 const domain = { domain:'example.com', description:'Documentation example domain', active:1, quota:5368709120, mailbox_count:1, mailboxes:10, used_mailboxes:1, create_time:1794643200, is_default:1, ssl_status:0, mx_record:0, spf_record:0, dkim_record:0, dmarc_record:0, a_record:0, ptr_record:0, dns_records:{}, urls:[], outbound_ip:'' }
 const group = { id:1,name:text(locale,'示例客户组','Demo customer group'), description:text(locale,'虚构地址；未发送邮件','Fictional addresses; no messages sent'), contact_count:1,total:1, confirmed:1,subscribed:1,active:1, dept_id:1, owner_id:1 }
 const template = { id:1,temp_id:1,temp_name:text(locale,'样例产品介绍模板','Demo introduction template'),name:'Demo introduction',subject:'Demo subject', content:'<h2>Demo product introduction</h2><p>Approved facts only.</p>',dept_id:1,owner_id:1,created_at:dates }
 const entities = { departments:[dept], roles:[role], users:[], grants:[{id:1,dept_id:1,dept_name:dept.name,sender:mailbox.username,active:true}], policies:[{id:1,scope:'global',scope_key:'',policy:{per_minute:2,per_hour:20,concurrency:1,min_interval_ms:60000}}], contacts:[contact], groups:[group], tags:[], templates:[template], tasks:[task], schedules:[], automations:[], reports:[], audit:[] }
 const list = rows => ({list:rows,total:rows.length,page_size:20,page:1})
 const models = { enabled:false, model:'', models:[],options:[],all_options:[],providers:[],defaults:{},version:1 }
 return { company, entities, task, skills, domain, mailbox,
  api(p, d) {
   if (p === '/api/languages/get') return {current_language:locale,available_languages:[{name:'en',cn:'English'},{name:'zh',cn:'简体中文'}]}
   if (p === '/api/enterprise/action') {
    if (d.action==='me') return {principal:{id:1,username:text(locale,'文档演示账号','Documentation demo'),dept_id:1,super:true,roles:[{role_id:1,role_name:'admin',permissions:['*'],data_scope:'all'}]},dept_name:dept.name,catalog:permissions,permission_dependencies:{}}
    if (d.action==='list') return list(entities[d.type] || [])
    if (d.action==='options') return list((entities[d.type] || []).map(r=>({...r,value:r.id,label:r.name || r.role_name || r.temp_name || r.email})))
    return list([])
   }
   if (p === '/api/growth/action') {
    if (d.action==='companies') return [company]
    if (['models','image_models','model_catalog'].includes(d.action)) return models
    if (d.action==='model_registry') return {version:1,providers:[{id:'demo',name:'Demo AI provider',enabled:false,format:'chat_completions',base_url:'https://api.example.com/v1',api_key:'',has_key:false,models:[{id:'demo-text-model',name:'Documentation text model',enabled:false,capabilities:['text','chat'],max_output:4000}]}],defaults:{}}
    if (d.action==='settings') return {config:{daily_calls:100,daily_tokens:200000,max_output:4000},usage:{calls:0,tokens:0}}
    if (d.action==='stats') return {plans:1,runs:0,candidates:0,leads:0,messages:0,inquiries:0,jobs:0,products:1}
    if (d.action==='list') return list(({products:[product],assets:[],jobs:[],plans:[plan],leads:[],candidates:[],messages:[],runs:[],inquiries:[],inbox_rules:[],companies:[company]})[d.kind] || [])
    if (d.action==='master') {
     if(d.kind==='catalog') return {companies:[company],packs:[pack],skills,flows:{WF01:['S01','S02','S03','S14'],WF02:['S04','S05','S06','S07','S08','S09','S14'],WF03:['S05','S06','S07','S08','S09','S14'],WF04:['S10','S11','S14'],WF05:['S12','S08','S09','S14'],WF06:['S13','S02','S14']},defaults:Object.fromEntries(['WF01','WF02','WF03','WF04','WF05','WF06'].map(c=>[c,{markets:[],channels:[],product_ids:[],prospect_ids:[],senders:[],social_accounts:[],budget:{},policy:{},reply_rules:[]}]))}
     if(d.kind==='overview') return {stopped:false,metrics:{evidence:0,prospects:0,contents:0,events:0,runs:0,workflows:1}}
     if(d.kind==='list') return list(d.data?.resource==='workflows'?[workflow]:[])
     if(d.kind==='skills') return Object.fromEntries(skills.map(s=>[s.id,{enabled:true,model:'',timeout_seconds:240,retries:2,max_items:20,max_output:2400}]))
     if(d.kind==='options') return list((d.data?.resource==='companies'?[company]:d.data?.resource==='products'?[product]:[]).map(r=>({value:r.id,label:r.name})))
     if(d.kind==='models') return models
     return {}
    }
    return {}
   }
   if(p==='/api/social/action') {
    if(d.action==='availability')return {facebook:{enabled:false,publishing:false},linkedin:{enabled:false,publishing:false}}
    if(d.action==='companies')return [company]
    if(['models','image_models'].includes(d.action))return models
    if(['filter_accounts','account_options'].includes(d.action))return list([account])
    if(d.action==='manual_reminders')return {total:0,rows:[]}
    if(d.action==='list')return list(d.kind==='plans'?[social]:d.kind==='accounts'?[account]:[])
    if(d.action==='reports')return {list:[],total:0,summary:{},coverage:{},source:'demo'}
    if(d.action==='calendar')return {list:[],total:0}
    return {}
   }
   if(p==='/api/crm/action')return d.action==='mailboxes'?[mailbox.username]:d.action==='list'?list([customer]):{}
   if(p==='/api/overview/domains')return [{domain:'example.com'}]
   if(p==='/api/overview') {const dashboard={sends:12,delivered:10,failed:0,delivery_rate:83.3,open_tracked:10,open_rate:30,click_rate:10,bounce_rate:0,delayed_queue:0};return {queue_access:true,dashboard,mail_providers:[],send_mail_chart:{column_type:'hourly',dashboard,data:[{x:9,delivered:4,failed:0},{x:10,delivered:6,failed:0}]},open_rate_chart:{column_type:'hourly',data:[{x:9,open_rate:25},{x:10,open_rate:30}]},click_rate_chart:{column_type:'hourly',data:[{x:9,click_rate:5},{x:10,click_rate:10}]},bounce_rate_chart:{column_type:'hourly',data:[{x:9,bounce_rate:0},{x:10,bounce_rate:0}]}}}
   if(p==='/api/domains/all')return [domain]
   if(p==='/api/domains/list')return list([domain])
   if(p==='/api/mailbox/all')return [mailbox]
   if(p==='/api/mailbox/list')return list([mailbox])
   if(p==='/api/askai/supplier/list')return []
   if(p.includes('languages'))return []
   if(p.includes('system_config'))return {admin_username:'',admin_password:'',hostname:'example.com',timezone:'UTC',public_base_url:'https://example.com'}
   if(p.includes('settings') || p.includes('config'))return {}
   if(p.endsWith('/all'))return []
   return list([])
  }
 }
}
const mime={'.js':'application/javascript','.css':'text/css','.html':'text/html','.png':'image/png','.svg':'image/svg+xml','.ico':'image/x-icon','.woff2':'font/woff2','.ttf':'font/ttf'}
const server=createServer(async(req,res)=>{
 try {
  const url=new URL(req.url,'http://localhost')
  const relative=decodeURIComponent(url.pathname).replace(/^\/+/, '')
  let file=path.resolve(dist,relative)
  if(file!==dist&&!file.startsWith(dist+path.sep)){res.writeHead(403).end();return}
  try{if(!(await fs.stat(file)).isFile())file=path.join(dist,'index.html')}catch{file=path.join(dist,'index.html')}
  let content=await fs.readFile(file)
  if(path.extname(file)==='.html')content=Buffer.from(content.toString().replace(/<meta http-equiv="Content-Security-Policy"[^>]*>/gi,''))
  res.writeHead(200,{'Content-Type':mime[path.extname(file)]||'application/octet-stream'})
  res.end(content)
 }catch{res.writeHead(500).end()}
})
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve))
const origin=`http://127.0.0.1:${server.address().port}`
const browser=await chromium.launch({executablePath,headless:true})
const captures=[]
const errors=[]
const unknown=new Set()
let blockedExternal=0
const languageOnly=process.env.DOC_LOCALES?.split(',') || ['zh','en']
try {
 for(const locale of languageOnly){
  const lang=JSON.parse(await fs.readFile(path.join(root,`core/frontend/src/i18n/lang/${locale}.json`),'utf8'))
  const t=key=>key.split('.').reduce((v,k)=>v?.[k],lang)||key
  const fixture=fixtures(locale)
  const context=await browser.newContext({viewport:{width:1440,height:980},locale:locale==='zh'?'zh-CN':'en-US',deviceScaleFactor:1})
  await context.addInitScript(({locale})=>{
   localStorage.setItem('UserStore',JSON.stringify({login:{token:'documentation-demo-session',refresh_token:'',ttl:86400,expire:Date.now()+86400000}}))
   localStorage.setItem('bestmaple.login-language',locale)
   localStorage.setItem('language',locale)
  },{locale})
  await context.route('**/*',async route=>{
   const request=route.request()
   if(!request.url().startsWith(origin+'/')){blockedExternal++;return route.abort()}
   const url=new URL(request.url())
   if(!url.pathname.startsWith('/api/'))return route.continue()
   let data={};try{data=request.postDataJSON()||{}}catch{}
   const result=fixture.api(url.pathname,data)
   unknown.add(url.pathname+(data.action?':'+data.action:'')+(data.kind?':'+data.kind:''))
   return route.fulfill({contentType:'application/json',body:JSON.stringify({code:0,success:true,msg:'Documentation demo only',data:result})})
  })
  const page=await context.newPage()
  page.on('pageerror',error=>{errors.push({locale,url:new URL(page.url()).pathname,message:error.message});console.error('PAGE_ERROR '+error.message)})
  page.on('requestfailed',request=>console.error('REQUEST_FAILED '+request.url()+' '+request.failure()?.errorText))
  page.on('console',message=>{if(message.type()==='error'){errors.push({locale,url:new URL(page.url()).pathname,message:message.text(),kind:'console'});console.error('BROWSER_CONSOLE '+message.text())}})
  async function capture(slug,routePath,tabKey){
   await page.goto(origin+routePath,{waitUntil:'networkidle',timeout:25000})
   await page.waitForFunction(()=>document.body.innerText.length>180,{},{timeout:15000}).catch(()=>{})
   if(tabKey){const tabs=page.locator('.n-tabs-tab').filter({hasText:tabKey.startsWith('workspaceCopy.') && t(tabKey).includes(':') ? t(tabKey).split(':').slice(1).join(':') : t(tabKey)});if(await tabs.count())await tabs.first().click();else throw new Error('Missing tab '+tabKey);await page.waitForTimeout(600)}
   await page.evaluate(({locale})=>{
    document.getElementById('doc-demo-banner')?.remove()
    const banner=document.createElement('div');banner.id='doc-demo-banner'
    banner.textContent=locale==='zh'?'文档演示界面 · 虚构数据 · 未连接生产服务':'Documentation demo · Fictional data · No production connection'
    Object.assign(banner.style,{position:'fixed',bottom:'0',left:'0',right:'0',zIndex:'99999',padding:'8px 16px',background:'#172b4d',color:'white',font:'13px sans-serif',textAlign:'center'})
    document.body.append(banner)
   },{locale})
   const body=await page.locator('body').innerText()
   if(body.length<180){await fs.writeFile(path.join(root,'.tools/doc-capture-failure.html'),await page.content());await page.screenshot({path:path.join(root,'.tools/doc-capture-failure.png')});console.error('BODY '+body);console.error(JSON.stringify({url:page.url(),api:[...unknown],errors,blockedExternal}));throw new Error('Empty screen '+slug)}
   const filename=`${locale}-${slug}.png`
   await page.screenshot({path:path.join(output,filename),fullPage:false,animations:'disabled'})
   captures.push({locale,slug,route:routePath,filename,viewport:'1440x980',characters:body.length})
   console.log('CAPTURE '+filename)
  }
  await capture('overview','/overview')
  await capture('organization','/workspace','workspaceCopy.w404')
  await capture('permissions','/workspace','workspaceCopy.w74')
  await capture('contacts','/workspace','workspaceCopy.w70')
  await capture('email-campaign','/workspace')
  await capture('materials','/materials')
  await capture('outreach','/outreach')
  await capture('intelligence','/intelligence')
  await capture('skills','/intelligence','intel.tabs.skills')
  await capture('social','/social')
  await capture('social-accounts','/social','social.tabs.accounts')
  await capture('crm','/crm')
  await capture('domain','/domain')
  await capture('mailbox','/mailbox')
  await capture('ai-models','/settings/ai-model')
  await context.close()
  const loginContext=await browser.newContext({viewport:{width:1440,height:980},locale:locale==='zh'?'zh-CN':'en-US'})
  await loginContext.addInitScript(locale=>localStorage.setItem('bestmaple.login-language',locale),locale)
  await loginContext.route('**/*',async route=>{const url=new URL(route.request().url());if(url.origin!==origin)return route.abort();if(!url.pathname.startsWith('/api/'))return route.continue();return route.fulfill({contentType:'application/json',body:JSON.stringify({code:0,success:true,data:fixture.api(url.pathname,{})})})})
  const login=await loginContext.newPage();await login.goto(origin+'/login',{waitUntil:'networkidle'});await login.waitForTimeout(400)
  await login.evaluate(locale=>{const x=document.createElement('div');x.textContent=locale==='zh'?'文档演示 · 账号与密码留空':'Documentation demo · Credentials left blank';Object.assign(x.style,{position:'fixed',bottom:'0',left:'0',right:'0',background:'#172b4d',color:'#fff',padding:'8px',textAlign:'center',zIndex:'99999'});document.body.append(x)},locale)
  const filename=`${locale}-login.png`;await login.screenshot({path:path.join(output,filename),fullPage:false});captures.push({locale,slug:'login',route:'/login',filename,viewport:'1440x980'});console.log('CAPTURE '+filename);await loginContext.close()
 }
 const provenance={source_commit:execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),frontend_index_sha256:createHash('sha256').update(await fs.readFile(path.join(dist,'index.html'))).digest('hex'),frontend_build:'existing local build; not freshly rebuilt for this documentation',mode:'real application frontend with intercepted fictional API data',production_access:false,external_requests_allowed:false,blocked_external_requests:blockedExternal,captures,errors,api_shapes:[...unknown].sort()}
 await fs.writeFile(path.join(root,'docs/handbook/assets/screenshot-provenance.json'),JSON.stringify(provenance,null,2)+'\n')
 if(errors.length){console.error(JSON.stringify(errors,null,2));process.exitCode=1}
} finally {await browser.close();await new Promise(resolve=>server.close(resolve))}
