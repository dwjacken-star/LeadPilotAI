(function () {

 'use strict';

 if(!window.rcmail)return;

 var rc=window.rcmail, M=window.LeadPilotMailModel, pending={}, serial=0;

 var lang=String(document.documentElement.lang||rc.env.locale||'en').toLowerCase();

 var strings=window.LeadPilotMailI18n.ui;

 var L=strings[lang.slice(0,2)]||strings.en;

 var extra=window.LeadPilotMailI18n.ui;Object.assign(L,extra[lang.slice(0,2)]||extra.en);

 function el(tag,cls,text){var e=document.createElement(tag);if(cls)e.className=cls;if(text!==undefined)e.textContent=text;return e}

 function button(text,fn,cls){var b=el('button',cls||'lp-button',text);b.type='button';b.addEventListener('click',fn);return b}

 function api(action,data){return new Promise(function(resolve,reject){var id='lp-'+(++serial),timer=setTimeout(function(){delete pending[id];reject(Error(L.connection))},110000);pending[id]={resolve:resolve,reject:reject,timer:timer};rc.http_post('plugin.leadpilot-api',{_lp_id:id,_lp_payload:JSON.stringify({action:action,data:data||{}})},false)})}

 rc.addEventListener('plugin.leadpilot-result',function(r){var p=pending[r.id];if(!p)return;clearTimeout(p.timer);delete pending[r.id];if(r.success)p.resolve(r.data);else p.reject(Error(/mailbox business account|mailbox_business_access_required/.test(r.message||'')?L.unavailable:(r.message||L.connection)))});

 function taskURL(task){var url=new URL(rc.env.comm_path,window.location.href);url.searchParams.set('_task',task);url.searchParams.delete('_action');url.searchParams.delete('_framed');return url.href}

 function notice(text,error){rc.display_message(text,error?'error':'confirmation')}

 var owner=null;try{if(window.parent!==window&&window.frameElement&&String(window.frameElement.name).indexOf('leadpilot-pane-')===0)owner=window.parent.LeadPilotMail}catch(e){}

 rc.addEventListener('init',function(){

  if(rc.env.leadpilot_shell){shell();return}

  if(owner){document.documentElement.classList.add('lp-mail-embedded');owner.ready(window.frameElement.name,window);enhance();return}

  if(window.top===window&&rc.env.task==='mail'&&!['print','get','viewsource','plugin.leadpilot-api'].includes(rc.env.action)){

   var current=new URL(window.location.href);var dest=new URL(rc.url('mail/plugin.leadpilot-workbench'),window.location.href);

   if(rc.env.action)dest.searchParams.set('_lp_open',btoa(encodeURIComponent(M.safeURL(current.href,current.href)||'')));

   window.location.replace(dest.href)

  }else enhance();

 });

 function shell(){

  document.body.classList.add('lp-mail-shell');var nav=document.getElementById('lp-mail-navigation'),header=document.getElementById('lp-mail-header'),tabs=document.getElementById('lp-mail-tabs'),panes=document.getElementById('lp-mail-panes');if(!nav)return;

  var logo=el('img','lp-mail-logo');logo.src='skins/bestmaple/assets/leadpilot-wordmark.png';logo.alt='LeadPilot AI';nav.appendChild(logo);

  nav.appendChild(button('+ '+L.compose,function(){open(rc.url('mail/compose'),L.draft,'compose')},'lp-button lp-primary lp-compose-main'));

  nav.appendChild(button(L.inbox,function(){activate('inbox')},'lp-nav-item'));

  nav.appendChild(button(L.contacts,function(){open(taskURL('addressbook'),L.contacts,'section','addressbook')},'lp-nav-item'));

  nav.appendChild(button(L.settings,function(){open(taskURL('settings'),L.settings,'section','settings')},'lp-nav-item'));

  nav.appendChild(button(L.logout,function(){if(!hasDirty()||window.confirm(L.unsaved)){sessionStorage.removeItem(storage);allowLeave=true;rc.command('logout')}},'lp-nav-item lp-logout'));

  header.appendChild(el('h1','',L.workspace));header.appendChild(el('span','lp-mail-account',rc.env.leadpilot_mailbox));header.appendChild(button('◐',function(){document.documentElement.classList.toggle('dark-mode');document.cookie='colorMode='+(document.documentElement.classList.contains('dark-mode')?'dark':'light')+';path=/roundcube/;SameSite=Lax';entries.forEach(function(t){if(t.win)t.win.document.documentElement.classList.toggle('dark-mode',document.documentElement.classList.contains('dark-mode'))})},'lp-icon'));

  var strip=el('div','lp-mail-tab-strip'),chooser=el('select','lp-tab-chooser');chooser.setAttribute('aria-label',L.all);chooser.addEventListener('change',function(){activate(chooser.value)});

  tabs.appendChild(button('‹',function(){strip.scrollBy({left:-280,behavior:'smooth'})},'lp-icon'));tabs.appendChild(strip);tabs.appendChild(button('›',function(){strip.scrollBy({left:280,behavior:'smooth'})},'lp-icon'));tabs.appendChild(chooser);

  tabs.appendChild(button('↻',function(){var t=find(active);if(t&&(!isDirty(t)||window.confirm(L.unsaved))&&!isBusy(t))t.frame.contentWindow.location.reload()},'lp-icon'));

  var entries=[],active='',storage='leadpilot.mail.tabs.'+rc.env.leadpilot_user+'@'+rc.env.leadpilot_mailbox,uid=0,closing={},allowLeave=false;

  function find(key){return entries.find(function(t){return t.key===key})}

  function persist(){try{sessionStorage.setItem(storage,JSON.stringify(entries.map(function(t){return M.savedTab(t,window.location.href)}).filter(Boolean)))}catch(e){}}

  function isDirty(t){try{return t.kind==='compose'&&M.dirty(t.win.rcmail)}catch(e){return false}}

  function isBusy(t){try{return !!t.win.LeadPilotMailBusy}catch(e){return false}}

  function hasDirty(){return entries.some(isDirty)||entries.some(isBusy)}

  function render(){strip.replaceChildren();chooser.replaceChildren();entries.forEach(function(t){var item=el('div','lp-mail-tab'+(active===t.key?' active':'')),b=button(t.label+(isDirty(t)?' •':''),function(){activate(t.key)});b.setAttribute('role','tab');b.setAttribute('aria-selected',String(active===t.key));item.appendChild(b);if(t.key!=='inbox')item.appendChild(button('×',function(){close(t.key)},'lp-tab-close'));strip.appendChild(item);var option=el('option','',t.label);option.value=t.key;chooser.appendChild(option)});chooser.value=active;var selected=strip.querySelector('.active');if(selected)selected.scrollIntoView({block:'nearest',inline:'nearest'})}

  function activate(key){var t=find(key);if(!t)return;active=key;entries.forEach(function(item){item.frame.hidden=item.key!==key;try{var r=item.win.rcmail;if(item.key!==key&&r._keepalive){clearInterval(r._keepalive);r._keepalive=null}else if(item.key===key&&r.start_keepalive){r.start_keepalive();item.win.dispatchEvent(new Event('resize'))}}catch(e){}});render()}

  function open(url,label,kind,key){var fixed=kind==='section'?new URL(url,window.location.href).searchParams.get('_task'):null;if(fixed==='settings')label=L.settings;else if(fixed==='addressbook')label=L.contacts;else if(kind==='compose'&&Object.keys(strings).some(function(code){return [strings[code].draft,strings[code].compose].indexOf(label)>=0}))label=L.draft;url=M.safeURL(url,window.location.href);if(!url)return;if(entries.length>=24&&!find(key)){notice(L.close+' '+L.all,true);return}key=key||'mail-'+Date.now()+'-'+(++uid);var t=find(key);if(t){activate(key);return}var frame=el('iframe','lp-mail-pane');frame.name='leadpilot-pane-'+key;frame.title=label;frame.src=url;frame.setAttribute('referrerpolicy','same-origin');frame.addEventListener('load',function(){try{if(frame.contentWindow.rcmail&&frame.contentWindow.rcmail.env.task==='login'){allowLeave=true;sessionStorage.removeItem(storage);window.location.assign(taskURL('login'))}}catch(e){}});t={key:key,label:label,url:url,kind:kind,frame:frame,win:null,draftUID:new URL(url).searchParams.get('_draft_uid'),draftMailbox:new URL(url).searchParams.get('_mbox')};entries.push(t);panes.appendChild(frame);activate(key);persist()}

  function remove(key){var t=find(key);if(!t||key==='inbox')return;var index=entries.indexOf(t);if(t.win&&t.win.rcmail)t.win.rcmail.compose_skip_unsavedcheck=true;t.frame.remove();entries.splice(index,1);if(active===key)activate(entries[Math.max(0,index-1)].key);render();persist()}

  function close(key){var t=find(key);if(!t||key==='inbox')return;if(isBusy(t)){notice(L.busy,true);return}if(!isDirty(t)){remove(key);return}

   var dialog=el('dialog','lp-close-dialog');dialog.appendChild(el('h2','',L.unsaved));dialog.appendChild(button(L.save,function(){if(isBusy(t))return;closing[key]=function(){remove(key);dialog.close();dialog.remove()};notice(L.saving);t.win.rcmail.command('savedraft');setTimeout(function(){if(closing[key]){delete closing[key];notice(L.failed,true)}},45000)},'lp-button lp-primary'));dialog.appendChild(button(L.discard,function(){dialog.close();dialog.remove();remove(key)}));dialog.appendChild(button(L.cancel,function(){dialog.close();dialog.remove()}));document.body.appendChild(dialog);dialog.showModal();

  }

  window.LeadPilotMail={open:open,ready:function(name,win){var t=entries.find(function(t){return t.frame.name===name});if(!t)return;if(win.rcmail.env.leadpilot_mailbox!==rc.env.leadpilot_mailbox){window.location.reload();return}t.win=win;if(t.kind==='compose'&&win.rcmail.env.draft_id){t.draftUID=win.rcmail.env.draft_id;t.draftMailbox=win.rcmail.env.drafts_mailbox;persist()}if(t.kind==='read'){var heading=win.document.querySelector('#message-header .subject');if(heading)t.label=heading.textContent.slice(0,100)}win.document.documentElement.classList.toggle('dark-mode',document.documentElement.classList.contains('dark-mode'));if(t.key!==active&&win.rcmail._keepalive){clearInterval(win.rcmail._keepalive);win.rcmail._keepalive=null}},changed:function(name,subject){var t=entries.find(function(t){return t.frame.name===name});if(t&&t.kind==='compose'&&subject)t.label=String(subject).slice(0,100);render()},draftSaved:function(name,id){var t=entries.find(function(t){return t.frame.name===name});if(!t)return;t.draftUID=id;t.draftMailbox=t.win.rcmail.env.drafts_mailbox;persist();render();if(closing[t.key]){var done=closing[t.key];delete closing[t.key];done()}},submitted:function(name){var t=entries.find(function(t){return t.frame.name===name});if(t)t.label=L.submitted;render()},activate:activate};

  var savedTabs='[]';try{savedTabs=sessionStorage.getItem(storage)||'[]'}catch(e){}

  open(taskURL('mail'),L.inbox,'mail','inbox');

  try{JSON.parse(savedTabs).slice(0,23).forEach(function(t){if(t.key!=='inbox')open(t.url,t.label,t.kind,t.key)})}catch(e){}

  var restore=new URL(window.location.href).searchParams.get('_lp_open');if(restore){try{var url=decodeURIComponent(atob(restore)),u=new URL(url);open(url,L.mail,u.searchParams.get('_action')==='compose'?'compose':'read')}catch(e){}window.history.replaceState({},'',rc.url('mail/plugin.leadpilot-workbench'))}

  activate('inbox');window.addEventListener('beforeunload',function(e){if(!allowLeave&&hasDirty()){e.preventDefault();e.returnValue=''}});

 }

 function enhance(){

  document.body.classList.add('lp-mail-surface');

  if(owner){

   rc.open_compose_step=function(p){var key=p&&p._draft_uid?'draft:'+p._mbox+':'+p._draft_uid:undefined;owner.open(rc.url('mail/compose',p),L.draft,'compose',key)};

   var nativeShow=rc.show_message;rc.show_message=function(id,safe,preview){if(preview)return nativeShow.apply(rc,arguments);var params=rc.params_from_uid(id,{});params._safe=safe?1:0;owner.open(rc.url('mail/show',params),L.read,'read','read:'+params._mbox+':'+params._uid)};

  }

  if(rc.env.action==='compose')composeTools();

  if(['show','preview'].indexOf(rc.env.action)>=0)customerTools();

 }

 function editor(){return rc.editor&&rc.editor.editor?rc.editor.editor:null}

 function bodyHTML(){var e=editor();return e&&e.getContent?e.getContent():String((document.querySelector('[name="_message"]')||{}).value||'').split('\n').map(function(line){return '<p>'+escapeHTML(line)+'</p>'}).join('')}

 function escapeHTML(s){return String(s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}

 function applyHTML(html,replace){var e=editor();if(e){if(replace){var content=document.createElement('div');content.innerHTML=e.getContent();var quote=content.querySelector('.maplemail-quoted-message');e.setContent(html+(quote?quote.outerHTML:''))}else e.insertContent(html)}else{var field=document.querySelector('[name="_message"]'),doc=new DOMParser().parseFromString(html,'text/html');if(field){field.value=(replace?'':field.value+'\n')+doc.body.textContent;field.dispatchEvent(new Event('input',{bubbles:true}))}}if(owner)owner.changed(window.frameElement.name,(document.querySelector('[name="_subject"]')||{}).value||'')}

 function panel(title){var d=el('dialog','lp-tool-dialog');d.appendChild(el('h2','',title));d.appendChild(button('×',function(){d.close();d.remove()},'lp-dialog-close'));document.body.appendChild(d);d.showModal();return d}

 function previewHTML(html){var doc=new DOMParser().parseFromString(html,'text/html');doc.querySelectorAll('script,form,iframe,object,embed,base,meta,link').forEach(function(n){n.remove()});doc.querySelectorAll('a,area').forEach(function(n){n.title=n.getAttribute('href')||'';n.removeAttribute('href');n.removeAttribute('target')});doc.querySelectorAll('img').forEach(function(n){if(/\/pmta\/w\//.test(n.getAttribute('src')||''))n.remove()});doc.querySelectorAll('*').forEach(function(n){n.removeAttribute('contenteditable');Array.from(n.attributes).forEach(function(a){if(/^on/i.test(a.name))n.removeAttribute(a.name)})});var meta=doc.createElement('meta');meta.httpEquiv='Content-Security-Policy';meta.content="default-src 'none'; img-src data: cid: blob: https: http:; style-src 'unsafe-inline'; font-src data:; form-action 'none'; base-uri 'none'";doc.head.prepend(meta);return '<!doctype html>'+doc.documentElement.outerHTML}

 function composeTools(){

  function options(){var values={};document.querySelectorAll('[data-maplemail-option]').forEach(function(input){values[input.dataset.maplemailOption]=input.checked});return values}

  window.LeadPilotSaveDraftOptions=function(){if(!rc.env.compose_id)return Promise.resolve();return api('draft_options_save',{draft_key:'c:'+rc.env.compose_id,draft_uid:rc.env.draft_id||'',options:options()}).catch(function(e){notice(e.message,true);throw e})};

  var form=rc.gui_objects.messageform;if(typeof form==='string')form=document.getElementById(form)||document.forms[form];if(!form)return;

  var toolbar=el('div','lp-compose-tools'),status=el('span','lp-compose-status');toolbar.appendChild(button(L.ai,aiDialog));toolbar.appendChild(button(L.template,templateDialog));toolbar.appendChild(button(L.preview,async function(){var prepared;try{var recipients=[];['_to','_cc','_bcc'].forEach(function(key){var field=document.querySelector('[name="'+key+'"]');if(field){var matches=field.value.match(/[A-Z0-9.!#$%&'*+\/=?^_`{|}~-]+@[A-Z0-9.-]+\.[A-Z]{2,}/gi)||[];recipients.push.apply(recipients,matches)}});prepared=await api('delivery_preview',{html:bodyHTML(),recipients:Array.from(new Set(recipients)),options:options()})}catch(e){notice(e.message,true);return}var d=panel(L.preview),tools=el('div','lp-preview-tools'),frame=el('iframe','lp-preview-frame');frame.setAttribute('sandbox','allow-same-origin');frame.srcdoc=previewHTML(prepared.html);frame.title=L.preview;tools.appendChild(button(L.desktop,function(){frame.style.width='100%'}));tools.appendChild(button(L.mobile,function(){frame.style.width='390px';frame.style.maxWidth='100%'}));d.appendChild(tools);d.appendChild(frame)}));toolbar.appendChild(status);form.prepend(toolbar);

  var delivery=document.getElementById('maplemail-delivery');if(delivery){var identity=delivery.querySelector('.lp-effective-identity');if(identity){var visibleIdentity=identity.cloneNode(true);toolbar.insertAdjacentElement('afterend',visibleIdentity)}var details=el('details','lp-send-details'),summary=el('summary','',L.parameters);details.appendChild(summary);delivery.parentNode.insertBefore(details,delivery);details.appendChild(delivery);details.appendChild(el('p','lp-hint',L.personalHint));details.appendChild(el('p','lp-hint',L.warmupHint))}

  form.addEventListener('input',function(){if(owner)owner.changed(window.frameElement.name,(document.querySelector('[name="_subject"]')||{}).value||'')});form.addEventListener('change',function(){if(owner)owner.changed(window.frameElement.name,(document.querySelector('[name="_subject"]')||{}).value||'')});

  var nativeSubmit=rc.submit_messageform,nativeDraft=rc.set_draft_id,nativeSent=rc.sent_successfully,nativeMessage=rc.display_message,submitTimer,sending=false;function unlock(){clearTimeout(submitTimer);window.LeadPilotMailBusy=false;review.hidden=true}var review=button(L.reviewDelivery,function(){if(!window.confirm(L.unknown))return;unlock();rc.enable_command('send',true);status.textContent=''});review.hidden=true;toolbar.appendChild(review);function uncertain(){clearTimeout(submitTimer);window.LeadPilotMailBusy='unknown';rc.enable_command('send',false);status.textContent=L.unknown;review.hidden=false}

  rc.submit_messageform=function(draft){if(window.LeadPilotMailBusy){notice(L.busy,true);return}window.LeadPilotMailBusy=true;sending=!draft;status.textContent=draft?L.saving:L.busy;submitTimer=setTimeout(function(){if(sending)uncertain();else{unlock();status.textContent=L.failed}},(rc.env.request_timeout||60)*1000+1000);return nativeSubmit.apply(rc,arguments)};

  rc.set_draft_id=function(id){nativeDraft.apply(rc,arguments);unlock();status.textContent=L.saved;window.LeadPilotSaveDraftOptions().then(function(){if(owner)owner.draftSaved(window.frameElement.name,id)}).catch(function(){})};

  rc.sent_successfully=function(type,msg,folders,saveError){unlock();if(saveError)return nativeSent.apply(rc,arguments);rc.env.is_sent=true;rc.compose_skip_unsavedcheck=true;rc.lock_form(form);status.textContent=L.submitted;notice(L.sentHint);if(owner)owner.submitted(window.frameElement.name);else nativeSent.apply(rc,arguments)};

  rc.display_message=function(message,type){if(type==='error'&&window.LeadPilotMailBusy){if(sending&&/requesttimedout|connerror|servererror/i.test(String(message)))uncertain();else{unlock();status.textContent=L.failed}}return nativeMessage.apply(rc,arguments)};

  rc.addEventListener('editor-load',function(event){var e=event.ref&&event.ref.editor;if(e)e.on('change input undo redo',function(){if(owner)owner.changed(window.frameElement.name,(document.querySelector('[name="_subject"]')||{}).value||'')})});

 }

 function select(label,items){var box=el('label','lp-field',label),field=el('select');items.forEach(function(item){var option=el('option','',item.label);option.value=item.value;field.appendChild(option)});box.appendChild(field);return {box:box,field:field}}

 function countries(){var display;try{display=new Intl.DisplayNames([lang],{type:'region'})}catch(e){}var list=[];for(var i=65;i<=90;i++)for(var j=65;j<=90;j++){var code=String.fromCharCode(i,j),name=display?display.of(code):code;if(name&&name!==code)list.push({value:code,label:name})}if(!list.length)list=['US','GB','CN','DE','FR','JP','AU','BR','AE','IN'].map(function(c){return {value:c,label:c}});return list.sort(function(a,b){return a.label.localeCompare(b.label)})}

 function aiDialog(){var d=panel(L.ai),requirement=el('textarea','lp-requirement'),country=select(L.country,countries()),language=select(L.language,[['en','English'],['zh-CN','简体中文'],['ja','日本語'],['de','Deutsch'],['fr','Français'],['es','Español'],['pt','Português'],['it','Italiano'],['ar','العربية'],['ru','Русский'],['ko','한국어'],['tr','Türkçe'],['vi','Tiếng Việt'],['id','Bahasa Indonesia'],['th','ไทย']].map(function(v){return {value:v[0],label:v[1]}})),model=select(L.model,[]),platform=select(L.platform||L.model,[]),grid=el('div','lp-form-grid'),result=null;

  requirement.placeholder=L.requirement;country.field.value='US';grid.append(country.box,language.box,platform.box,model.box);d.append(grid,requirement);var generate=button(L.generate,async function(){generate.disabled=true;try{result=await api('workbench_ai',{requirement:requirement.value,country:country.field.value,language:language.field.value,model:model.field.value,target:'all',subject:(document.querySelector('[name="_subject"]')||{}).value||'',content:bodyHTML()});preview.srcdoc=previewHTML(result.html_content);subject.textContent=result.subject;apply.disabled=false;insert.disabled=false}catch(e){notice(e.message,true)}finally{generate.disabled=false}},'lp-button lp-primary');generate.disabled=true;d.appendChild(generate);d.appendChild(el('p','lp-hint',L.review));var subject=el('p'),preview=el('iframe','lp-preview-frame');preview.setAttribute('sandbox','allow-same-origin');preview.title=L.review;d.append(subject,preview);var apply=button(L.replace,function(){if(!result)return;if(M.dirty(rc)&&!window.confirm(L.unsaved))return;applyHTML(result.html_content,true);var f=document.querySelector('[name="_subject"]');if(f)f.value=result.subject;d.close();d.remove()}),insert=button(L.insert,function(){if(result)applyHTML(result.html_content,false);d.close();d.remove()});apply.disabled=insert.disabled=true;d.append(insert,apply);api('workbench_options').then(function(options){var author=options.authoring||{};var catalog=author.models||[],providers={};catalog.forEach(function(m){var id=m.provider_id||'legacy';providers[id]=m.provider_name||'AI'});Object.keys(providers).forEach(function(id){var o=el('option','',providers[id]);o.value=id;platform.field.appendChild(o)});var preferred=catalog.find(function(m){return m.value===author.model});if(preferred)platform.field.value=preferred.provider_id||'legacy';function updateModels(){model.field.textContent='';catalog.filter(function(m){return (m.provider_id||'legacy')===platform.field.value}).forEach(function(m){var o=el('option','',m.model_name||m.model_id||m.label);o.value=m.value;model.field.appendChild(o)});generate.disabled=!author.enabled||!model.field.value}platform.field.addEventListener('change',updateModels);updateModels();if(preferred)model.field.value=preferred.value;generate.disabled=!author.enabled||!model.field.value;if(!options.ai||!author.enabled)d.appendChild(el('p','lp-hint',author.warning==='mailbox_business_access_required'?L.unavailable:(author.warning||L.unavailable)))}).catch(function(e){d.appendChild(el('p','lp-hint',L.unavailable+' '+e.message))})

 }

 function templateDialog(){var d=panel(L.template),search=el('input'),list=el('div','lp-template-list'),page=0;search.placeholder=L.search;d.appendChild(search);var more=button(L.more,load);d.append(button(L.search,function(){page=0;list.replaceChildren();load()}),list,more);async function load(){more.disabled=true;try{var r=await api('workbench_templates',{keyword:search.value,page:++page});(r.list||[]).forEach(function(item){list.appendChild(button(item.name,async function(){try{var result=await api('workbench_template',{id:item.id});if(M.dirty(rc)&&!window.confirm(L.unsaved))return;applyHTML(result.template.html_content,true);d.close();d.remove()}catch(e){notice(e.message,true)}}))});more.hidden=(r.list||[]).length<20}catch(e){d.appendChild(el('p','lp-hint',e.message))}finally{more.disabled=false}}load()}

 function customerTools(){var holder=document.getElementById('message-header')||document.getElementById('messagebody')||document.body;holder.prepend(button(L.customer,function(){var d=panel(L.customer),email=el('input'),customer=null;email.type='email';email.placeholder=L.email;email.value=rc.env.sender||'';var info=el('div');d.append(email,button(L.lookup,load),info);async function load(){try{var r=await api('workbench_customer',{email:email.value});customer=r.customer&&r.customer.id?r.customer:null;info.replaceChildren();if(!customer)info.appendChild(el('p','',L.none));var name=el('input'),company=el('input'),stage=select(L.stage,['new','inquiry','quoted','negotiating','won','lost'].map(function(s,i){return {value:s,label:L.stages[i]}}));name.placeholder=L.name;company.placeholder=L.company;if(customer){name.value=customer.name;company.value=customer.company;stage.field.value=customer.stage}info.append(name,company,stage.box,button(L.saveCustomer,async function(){try{await api('workbench_save_customer',{id:customer?customer.id:0,email:email.value,name:name.value,company:company.value,stage:stage.field.value});await load()}catch(e){notice(e.message,true)}}));if(customer){var note=el('textarea');note.placeholder=L.note;info.append(note,button(L.saveNote,async function(){try{await api('workbench_followup',{id:customer.id,content:note.value});note.value='';await load()}catch(e){notice(e.message,true)}}));((r.detail||{}).followups||[]).forEach(function(f){info.appendChild(el('p','lp-customer-note',f.content))})}}catch(e){info.textContent=L.unavailable+' '+e.message}}if(email.value)load()}))}

})();

