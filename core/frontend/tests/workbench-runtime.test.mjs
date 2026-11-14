import test from 'node:test'
import assert from 'node:assert/strict'
import {createRenderer,h,ref,nextTick,watch,onBeforeUnmount} from 'vue'
import {createRouter,createMemoryHistory,RouterView,useRoute} from 'vue-router'
import {createPinia,setActivePinia} from 'pinia'
import AppMain from '../src/layout/components/AppMain.vue'
import useStore from '../src/store/modules/workbench.ts'
import {useEmailEditorStore} from '../src/features/EmailEditor/store/index.ts'
import {tabFromRoute,upsertTab,restoreTabs} from '../src/features/workbench/model.ts'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'

function host(){
 const node=(type,text='')=>({type,text,children:[],parent:null,scrollTop:0})
 function remove(el){if(el.parent){el.parent.children=el.parent.children.filter(x=>x!==el);el.parent=null}}
 const renderer=createRenderer({createElement:node,createText:text=>node('#text',text),createComment:text=>node('#comment',text),insert(el,parent,anchor){remove(el);el.parent=parent;const index=parent.children.indexOf(anchor);if(index>=0)parent.children.splice(index,0,el);else parent.children.push(el)},remove,setText(el,text){el.text=text},setElementText(el,text){el.text=text;el.children=[]},parentNode:el=>el.parent,nextSibling(el){return el.parent?.children[el.parent.children.indexOf(el)+1]||null},patchProp(el,key,old,value){el[key]=value}})
 return {renderer,container:node('root')}
}

test('actual workspace components retain independent forms, frozen route params and editor stores; closing and account changes dispose them',async()=>{
 const storage=new Map();globalThis.sessionStorage={getItem:k=>storage.get(k)||null,setItem:(k,v)=>storage.set(k,v),removeItem:k=>storage.delete(k)}
 globalThis.document={addEventListener(){},removeEventListener(){}}
 const {renderer,container}=host(),instances=[],unmounted=[],routeChanges=[]
 const Page={setup(){const route=useRoute(),count=ref(0),editor=useEmailEditorStore(),record={id:route.params.id||'home',count,editor,route};instances.push(record);watch(()=>route.params.id,id=>routeChanges.push([record.id,id]));onBeforeUnmount(()=>unmounted.push(record.id));return ()=>h('div',String(count.value))}}
 const router=createRouter({history:createMemoryHistory(),routes:[{path:'/',component:{render:()=>h(AppMain)},children:[{path:'workspace',name:'Home',meta:{key:'workspace',title:'Home'},component:Page},{path:'task/:id',name:'Task',meta:{key:'tasks',title:'Task'},component:Page}]}]})
 const pinia=createPinia();setActivePinia(pinia);const store=useStore()
 router.afterEach(to=>store.visit(to));await router.push('/workspace');await router.isReady();store.initialize(1,p=>router.resolve(p),()=>true);store.visit(router.currentRoute.value)
 const app=renderer.createApp({render:()=>h(RouterView)});app.use(pinia);app.use(router);app.mount(container);await nextTick()
 await router.push('/task/1');await nextTick();const one=instances.find(x=>x.id==='1');one.count.value=11;one.editor.selectedBlockKey='one'
 await router.push('/task/2');await nextTick();const two=instances.find(x=>x.id==='2');two.count.value=22;two.editor.selectedBlockKey='two'
 assert.notEqual(one.editor,two.editor);assert.equal(one.route.params.id,'1');assert.equal(two.route.params.id,'2');assert.deepEqual(routeChanges,[])
 await router.push('/task/1');await nextTick();assert.equal(instances.filter(x=>x.id==='1').length,1);assert.equal(one.count.value,11);assert.equal(one.editor.selectedBlockKey,'one');assert.equal(two.editor.selectedBlockKey,'two')
 store.remove(store.tabs.find(t=>t.fullPath==='/task/2').key);await nextTick();assert.ok(unmounted.includes('2'))
 store.refresh(store.active);await nextTick();assert.equal(instances.filter(x=>x.id==='1').length,2);assert.ok(unmounted.includes('1'))
 const mountedBefore=instances.length;store.initialize(2,p=>router.resolve(p),()=>true);store.visit(router.currentRoute.value);await nextTick();assert.ok(instances.length>mountedBefore);assert.equal(Object.keys(store.dirty).length,0)
 app.unmount()
})

test('tab restore removes forbidden resources, OAuth secrets and duplicates',()=>{
 const resolve=path=>({name:'Social',path:path.split('?')[0],query:Object.fromEntries(new URL('https://local'+path).searchParams),params:{},meta:{key:'social',titleKey:'social.title'},matched:[{meta:{key:'social',titleKey:'social.title'}}]})
 const tab=tabFromRoute(resolve('/social?code=secret&state=oauth&tab=reports'))
 assert.equal(tab.fullPath,'/social?tab=reports');assert.equal(upsertTab([tab],tab).length,1)
 assert.equal(tabFromRoute(resolve('/social?oauth=success&code=secret')).fullPath,'/social?oauth=success')
 assert.equal(tabFromRoute(resolve('/social?oauth=secret')).fullPath,'/social')
 const restored=restoreTabs(JSON.stringify([{fullPath:'/social?access_token=secret'},{fullPath:'/social'},{fullPath:'https://evil.test'},{fullPath:'/restricted'}]),resolve,p=>p!=='/restricted')
 assert.equal(restored.length,1);assert.equal(restored[0].fullPath,'/social')
})

test('mail tabs reject external URLs, strip authentication values and detect independent unsaved drafts',()=>{
 const context={URL,module:{exports:{}},globalThis:{}};vm.runInNewContext(readFileSync(new URL('../../../webmail/plugins/bestmaple_tracking/workbench-model.js',import.meta.url),'utf8'),context)
 const M=context.module.exports,base='https://mail.example/roundcube/?_task=mail'
 assert.equal(M.safeURL('https://evil.test/roundcube/?_task=mail',base),null);assert.equal(M.safeURL('/api/settings',base),null)
 assert.equal(M.safeURL('/roundcube/?_task=mail&_action=compose&_pass=secret&_token=secret&_id=unique',base),'https://mail.example/roundcube/?_task=mail&_action=compose&_id=unique')
 const reply=M.safeURL('/roundcube/?_task=mail&_action=compose&_reply_uid=9&_all=all&_attachment=1&_to=buyer%40example.com',base)
 assert.equal(new URL(reply).searchParams.get('_all'),'all');assert.equal(new URL(reply).searchParams.get('_to'),'buyer@example.com')
 const saved=M.savedTab({url:reply,key:'draft',label:'Draft',kind:'compose',draftUID:'42',draftMailbox:'Drafts'},base)
 assert.equal(new URL(saved.url).searchParams.get('_draft_uid'),'42');assert.equal(new URL(saved.url).searchParams.get('_mbox'),'Drafts');assert.equal(new URL(saved.url).searchParams.get('_to'),null);assert.equal(new URL(saved.url).searchParams.get('_reply_uid'),null)
 assert.equal(M.savedTab({url:reply,kind:'compose'},base),null)
 const first={env:{action:'compose'},cmp_hash:'old',compose_field_hash:()=> 'new'},second={env:{action:'compose'},cmp_hash:'saved',compose_field_hash:()=> 'saved'}
 assert.equal(M.dirty(first),true);assert.equal(M.dirty(second),false);first.env.is_sent=true;assert.equal(M.dirty(first),false)
})
