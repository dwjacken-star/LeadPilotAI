(function (root) {
 'use strict';
 var safeKeys = ['_task','_action','_mbox','_uid','_draft_uid','_reply_uid','_forward_uid','_id','_safe','_lp_open','_to','_cc','_bcc','_subject','_body','_all','_attachment'];
 function safeURL(value, base) {
  try {var u = new URL(value, base), b = new URL(base);if(u.origin !== b.origin || u.pathname.replace(/index\.php$/,'') !== b.pathname.replace(/index\.php$/,''))return null;u.hash='';Array.from(u.searchParams.keys()).forEach(function(k){if(safeKeys.indexOf(k)<0)u.searchParams.delete(k)});if(u.searchParams.get('_action')==='plugin.leadpilot-workbench')return null;return u.href}catch(e){return null}
 }
 function dirty(rc) {return !!(rc && rc.env && rc.env.action==='compose' && !rc.env.is_sent && !rc.compose_skip_unsavedcheck && rc.compose_field_hash && rc.cmp_hash!==rc.compose_field_hash())}
 function savedTab(tab,base){
  var url=safeURL(tab.url,base);if(!url)return null;var u=new URL(url),uid=tab.draftUID||u.searchParams.get('_draft_uid');
  if(tab.kind==='compose'){
   if(!uid)return null;var mbox=tab.draftMailbox||u.searchParams.get('_mbox');u.search='';u.searchParams.set('_task','mail');u.searchParams.set('_action','compose');u.searchParams.set('_draft_uid',uid);if(mbox)u.searchParams.set('_mbox',mbox)
  }else ['_to','_cc','_bcc','_subject','_body','_lp_open'].forEach(function(key){u.searchParams.delete(key)});
  return {url:u.href,label:tab.label,kind:tab.kind,key:tab.key}
 }
 root.LeadPilotMailModel={safeURL:safeURL,dirty:dirty,savedTab:savedTab};
 if(typeof module==='object')module.exports=root.LeadPilotMailModel;
})(typeof window==='object'?window:globalThis);
