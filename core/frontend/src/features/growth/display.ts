type Row=Record<string,any>
export interface GrowthText {t:(key:string,params?:Record<string,any>)=>string;te:(key:string)=>boolean;locale:string}
const empty='—'
export function growthObject(value:any):Row{if(typeof value==='string'){try{value=JSON.parse(value)}catch{return {}}}return value&&typeof value==='object'&&!Array.isArray(value)?value:{}}
export function growthArray(value:any):any[]{if(typeof value==='string'){try{value=JSON.parse(value)}catch{return []}}return Array.isArray(value)?value:[]}
export function growthEmails(value:any):string[]{return [...new Set(growthArray(value).filter(v=>typeof v==='string'&&v.trim()).map(v=>v.trim()))]}
export function growthEnum(value:any,c:GrowthText):string{const key='growth.states.'+value;return value==null||value===''?empty:c.te(key)?c.t(key):String(value)}
export function growthError(value:any,c:GrowthText):string{
 if(!value)return empty
 const raw=String(value),codes='emailVerification.codes.'+raw
 if(c.te(codes))return c.t(codes)
 const messages:Record<string,string>={'daily AI budget reached':'ai_budget_reached','plan configuration changed; start a new run':'plan_changed','Some jobs failed or require service configuration; inspect background tasks':'jobs_require_attention','Some company pages could not be collected; inspect candidates':'pages_require_attention','Search returned no websites; no email collection or sending occurred':'no_search_sources','Daily lead limit reached; remaining addresses retained in candidate':'daily_lead_limit','Serper daily budget exhausted; configuration required':'serper_budget_reached','Serper authentication or credits unavailable; configuration required':'serper_auth_or_credits','Serper credits exhausted; configuration required':'serper_credits_exhausted','Serper configuration required: enable search and save an API key':'serper_not_configured','Serper connection failed':'serper_connection_failed','Serper returned an empty website':'empty_website','Serper target website denied or unavailable':'website_unavailable','plan paused':'plan_paused','AI connection failed':'ai_connection_failed'}
 if(messages[raw])return c.t('growth.reason_'+messages[raw])
 const http=raw.match(/^Serper request failed \(HTTP (\d+)\)$/)
 if(http)return c.t('growth.reason_serper_http',{status:http[1]})
 const partial='Some contact pages could not be collected: '
 if(raw.startsWith(partial))return c.t('growth.reason_partial_pages')+' '+raw.slice(partial.length).split('; ').map(v=>growthError(v,c)).join('；')
 return raw
}
function localName(value:any,type:'region'|'language',c:GrowthText):string{
 if(!value)return empty
 try{return new Intl.DisplayNames([c.locale.replace('_','-'),'en'],{type}).of(type==='region'?String(value).toUpperCase():String(value))||String(value)}catch{return String(value)}
}
export function growthValue(kind:string,key:string,row:Row,c:GrowthText):string{
 const value=row[key]
 const related:Record<string,string>={company_id:'company_name',plan_id:'plan_name',run_id:'run_name',lead_id:'lead_email',task_id:'task_name'}
 if(related[key])return value==null||!Number(value)?empty:row[related[key]]||c.t('growth.reference_unavailable')
 if(key==='emails')return growthEmails(value).join('\n')||c.t('growth.no_email_yet')
 if(key==='country'||key==='target_country')return localName(value,'region',c)
 if(key==='language')return localName(value,'language',c)
 if(kind==='jobs'&&key==='kind'&&value==='draft')return c.t('growth.job_draft')
 if(['status','state','kind'].includes(key))return kind==='runs'&&value==='completed'?c.t('growth.run_finished'):growthEnum(value,c)
 if(['error','reason'].includes(key))return growthError(value,c)
 if(['active','suppressed','reply_pending','relevant'].includes(key))return c.t('growth.'+((value===true||value===1||value==='1'||value==='true')?'yes':'no'))
 if(key==='confidence')return value==null?empty:Math.round(Number(value)*100)+'%'
 if(['created_at','updated_at','due_at','next_at','queued_at','verified_until','last_contact_at','replied_at'].includes(key)){
  if(!value)return empty;const date=new Date(value);if(Number.isNaN(date.getTime()))return String(value)
  try{return new Intl.DateTimeFormat(c.locale,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).format(date)}catch{return String(value)}
 }
 if(value==null||value==='')return empty
 if(typeof value==='object')return empty
 return String(value)
}
const detailKeys:Record<string,string[]>={
 candidates:['company_id','plan_id','run_id','company','website','target_country','country','language','address','phone','state','filtered_count','imported_count','error','created_at','updated_at'],
 leads:['company_id','plan_id','email','company','website','country','language','state','assigned_sender','verified_until','last_contact_at','replied_at','suppressed','reply_pending','error','created_at'],
 messages:['plan_id','run_id','lead_id','subject','language','status','task_id','error','created_at','queued_at'],
 runs:['plan_id','due_at','status','error','created_at'],
 inquiries:['company_id','kind','confidence','summary','state','created_at']
}
export function growthDetailFields(kind:string,row:Row,c:GrowthText){return(detailKeys[kind]||[]).filter(key=>key in row).map(key=>({key,label:c.t('growth.'+(key==='company'&&['candidates','leads'].includes(kind)?'customer_company':key)),value:growthValue(kind,key,row,c)}))}
export function growthVerificationFields(value:any,c:GrowthText){
 const v=growthObject(value);if(!Object.keys(v).length)return []
 const row={...v,status:v.status==='valid'?'verified':v.status==='unknown'?'unconfirmed':v.status}
 return ['status','code','stage','checked_at','expires_at','probe_sender'].filter(k=>k in v).map(key=>({key,label:c.t('growth.verification_'+key),value:key==='status'&&v.status==='valid'&&v.stage!=='smtp'?c.t('emailVerification.domainOnly'):key==='code'?growthError(v[key],c):key==='stage'?(c.te('growth.verification_stage_'+v[key])?c.t('growth.verification_stage_'+v[key]):String(v[key])):key.endsWith('_at')?growthValue('', 'created_at',{created_at:v[key]},c):growthValue('',key,row,c)}))
}
export function growthFactsFields(value:any,c:GrowthText){const f=growthObject(value);return ['summary','relevant','confidence','evidence','country_evidence','address','address_evidence','phone','phone_evidence'].filter(k=>f[k]!==undefined&&f[k]!==null&&f[k]!=='').map(key=>({key,label:c.t('growth.fact_'+key),value:growthValue('',key,f,c)}))}
