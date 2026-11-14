type Translator = (key: string, params?: Record<string, any>) => string
const scopes: Record<string, string> = { self: 'w79', department: 'w80', descendants: 'w81', custom: 'w82', all: 'w83' }
const policyScopes: Record<string, string> = { global: 'w84', department: 'w347', sender: 'w85', ip: 'w86', provider: 'w87' }
const cadence: Record<string, string> = { once: 'w88', daily: 'w89', weekly: 'w90', monthly: 'w91' }
const states: Record<string, string> = { pending: 'w96', retry: 'w97', claimed: 'w98', sending: 'w99', accepted: 'w100', failed: 'w101', skipped: 'w102', unknown: 'w103', completed: 'w104', stopped: 'w105', waiting: 'w385' }
const resources: Record<string, string> = { departments: 'w347', users: 'w119', roles: 'w391', grants: 'w123', policies: 'w125', groups: 'w129', contacts: 'w106', tags: 'w138', templates: 'w107', tasks: 'w108', schedules: 'w393', automations: 'w389', reports: 'w375' }
const actions: Record<string, string> = { save: 'w65', delete: 'w112', create_task: 'w110', save_schedule: 'w65', save_automation: 'w65', create_group_import: 'w113', import: 'w113', assign_tags: 'w171', task_control: 'w386', adopt: 'w161', reconcile: 'w186' }
function parsed(value: any, fallback: any) { if (value == null) return fallback; if (typeof value === 'string') { try { return JSON.parse(value) } catch { return fallback } } return value }
function array(value: any): any[] { const result = parsed(value, []); return Array.isArray(result) ? result : [] }
export function workspacePermissionLabel(code: string, t: Translator) {
 if (code === '*') return t('workspaceDisplay.allPermissions')
 if (code === 'overview:view') return t('layout.menu.overview')+' · '+t('workspaceCopy.w109')
 const [area, action,subAction] = code.split(':')
 if(area==='social'){const areas:Record<string,string>={account:'accounts',plan:'plans',content:'content',interaction:'interactions',report:'reports',publish:'confirm_schedule',settings:'settings'};const verbs:Record<string,string>={view:'workspaceCopy.w109',manage:'workspaceCopy.w386',edit:'workspaceCopy.w111',review:'social.approve',analyze:'social.analyze'};return t('social.'+(areas[action]&&['accounts','plans','interactions','reports'].includes(areas[action])?'tabs.':'')+(areas[action]||'title'))+(subAction?' · '+t(verbs[subAction]||'workspaceDisplay.unknown'):'')}
 if(area==='intelligence'){const verbs:Record<string,string>={view:'workspaceCopy.w109',manage:'workspaceCopy.w386',run:'intel.run_now',review:'intel.approve',publish:'workspaceCopy.w352',settings:'layout.menu.settings',export:'workspaceCopy.w114'};return t('intel.title')+' · '+t(verbs[action]||'workspaceDisplay.unknown')}
 const growthAreas:Record<string,string>={company:'company',material:'materials',product:'products',outreach:'outreach',inquiry:'inquiries',growth:'ai_settings'}
 if(growthAreas[area] || code==='crm:ai_draft'){
  if(code==='crm:ai_draft')return t('growth.ai_reply')
  const labels:Record<string,string>={list:'view',manage:'edit',publish:'publish',upload:'upload_materials',recognize:'recognize',download:'download',delete:'archive',import:'start_import',export:'export',run:'run_now',settings:'ai_settings'}
  if(code==='growth:settings')return t('growth.ai_settings')
  return t('growth.'+growthAreas[area])+' · '+t('growth.'+(labels[action]||'view'))
 }
 const areas: Record<string,string> = { contact:'w106',template:'w107',campaign:'w108',schedule:'w393',automation:'w389',report:'w375' }
 const verbs: Record<string,string> = { list:'w109',create:'w110',update:'w111',delete:'w112',import:'w113',export:'w114',publish:'w352',pause:'w115',cancel:'w116',manage:'w386' }
 return `${area === 'crm' ? t('crm.title') : areas[area] ? t('workspaceCopy.'+areas[area]) : t('workspaceDisplay.unknown')}：${['reply','sync'].includes(action) ? t('crm.'+action) : verbs[action] ? t('workspaceCopy.'+verbs[action]) : t('workspaceDisplay.unknown')}`
}
export function formatWorkspaceCell(kind: string, key: string, row: Record<string,any>, t: Translator): string {
 const value = row[key], copy = (id: string) => t('workspaceCopy.'+id)
 const name = (value: any) => typeof value === 'string' && value.trim() ? value : t('workspaceDisplay.unavailable')
 const names = (value: any) => array(value).map(name).join('、') || '—'
 const relation: Record<string,string> = { dept_id:'dept_name',parent_id:'parent_name',group_id:'group_name',account_id:'account_name',task_id:'task_name',contact_id:'contact_email',resource_id:'resource_name' }
 if(key==='verification_status'){if(value==='valid'&&['mx_valid','implicit_mx'].includes(row.verification_code))return t('emailVerification.domainOnly');return t('emailVerification.'+(['valid','invalid','unknown'].includes(value)?value:'pending'))}
 if(key==='verification_code')return value?t('emailVerification.codes.'+value):'—'
 if (relation[key]) {
  if (key==='parent_id' && !Number(value)) return t('workspaceDisplay.rootDepartment')
  if (!Number(value)) return '—'
  return name(row[relation[key]])
 }
 if (['group_names','role_names','custom_dept_names'].includes(key)) return names(value)
 if (key==='template_name') return name(value)
 if (key==='scope_key') return row.scope==='global' ? copy('w84') : row.scope==='department' ? name(row.scope_name) : String(value || '—')
 if (key==='data_scope') return scopes[value] ? copy(scopes[value]) : t('workspaceDisplay.unknown')
 if (key==='scope') return policyScopes[value] ? copy(policyScopes[value]) : t('workspaceDisplay.unknown')
 if (key==='cadence') return cadence[value] ? copy(cadence[value]) : t('workspaceDisplay.unknown')
 if (key==='state' || key==='delivery_state') return value==='validation_hold'?t('emailVerification.held'):states[value] ? copy(states[value]) : t('workspaceDisplay.unknown')
 if (key==='active' || key==='status') {
  const enabled = value===true || value===1 || value==='1' || value==='true'
  if (kind==='contacts') return t(`workspaceDisplay.${key==='active' ? enabled?'subscribed':'unsubscribed' : enabled?'confirmed':'unconfirmed'}`)
  return t('workspaceDisplay.'+(enabled?'enabled':'disabled'))
 }
 if (key==='permissions') return array(value).map(code=>workspacePermissionLabel(String(code),t)).join('；') || '—'
 if (key==='delivery_counts') return Object.entries(parsed(value,{})||{}).map(([state,count])=>`${state==='validation_hold'?t('emailVerification.held'):states[state] ? copy(states[state]) : t('workspaceDisplay.unknown')}：${count}`).join('；') || '—'
 if (key==='action') {
  const [action,type]=String(value||'').split(':')
  return [actions[action] ? copy(actions[action]) : t('workspaceDisplay.unknown'),resources[type] ? copy(resources[type]) : ''].filter(Boolean).join(' · ')
 }
 if (key==='wait_reason' || key==='last_error') {
  const validation=String(value||'').match(/^Recipient validation \w+: (.+) \((\w+)\)$/)
  if(validation)return `${t('emailVerification.codes.'+validation[2])} · ${validation[1]}`
  const [reason,target]=String(value||'').split('：')
  const reasons:Record<string,string>={'等待发送时间窗':'waitWindow','等待批间休息':'waitRest','等待发送间隔':'waitInterval','等待并发额度':'waitConcurrency','等待滚动配额':'waitQuota','等待收件人触达间隔':'waitRecipient'}
  if(reasons[reason]) {
   const match=target?.match(/^(global|department|task|sender|ip|provider):(.*)$/)
   const scope=match?.[1],identifier=match?.[2]
   const label=scope==='global' ? copy('w84') : scope==='department' ? name(row.dept_name) : scope==='task' ? name(row.task_name) : scope ? `${copy(policyScopes[scope])}：${identifier}` : ''
   return [t('workspaceDisplay.'+reasons[reason]),label].filter(Boolean).join(' · ')
  }
 }
 if (key==='policy') {
  const policy=parsed(value,{}),fields:Record<string,string>={per_minute:'w381',per_hour:'w382',per_day:'w383',min_interval_ms:'w198',concurrency:'w355',recipient_cooldown_hours:'w354',batch_size:'w384',batch_rest_seconds:'w374',timezone:'w192',window_start:'w193',window_end:'w195'}
  const parts=Object.entries(fields).filter(([field])=>policy[field]!=null && policy[field]!=='').map(([field,label])=>`${copy(label)}：${policy[field]===0 && ['per_minute','per_hour','per_day','concurrency','batch_size'].includes(field) ? t('workspaceDisplay.unlimited') : policy[field]}`)
  const days=array(policy.weekdays).map(day=>copy('w'+(357+Number(day)))).join('、')
  if(days) parts.push(`${copy('w377')}：${days}`)
  return parts.join('；') || '—'
 }
 if (['created_at','next_attempt_at','next_run_at','due_at','next_at'].includes(key)) {
  if(!value || value==='0') return '—'
  const date=new Date(typeof value==='number' ? value*1000 : value)
  return Number.isNaN(date.getTime()) ? '—' : `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')} ${String(date.getHours()).padStart(2,'0')}:${String(date.getMinutes()).padStart(2,'0')}:${String(date.getSeconds()).padStart(2,'0')}`
 }
 if (key==='step') return t('workspaceCopy.step',{index:Number(value)+1})
 return value==null || value==='' ? '—' : String(value)
}
