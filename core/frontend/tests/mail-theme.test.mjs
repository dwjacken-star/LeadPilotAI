import test from 'node:test'
import assert from 'node:assert/strict'
import vm from 'node:vm'
import fs from 'node:fs'

test('native compose theme follows parent theme without touching MIME body',()=>{
 let dark=false,observer,style=null
 const events={}
 const original='<p style="color:#123456;background:#fff">Customer draft</p>'
 const doc={head:{appendChild:s=>style=s},getElementById:()=>style,createElement:()=>({textContent:''})}
 Object.defineProperty(doc,'body',{get(){throw Error('The theme must not rewrite the outgoing body')}})
 const rc={env:{action:'compose'},addEventListener:(name,fn)=>events[name]=fn,editor:{editor:null}}
 const context={window:{rcmail:rc},rcmail:rc,document:{documentElement:{classList:{contains:()=>dark}}},MutationObserver:class{constructor(fn){observer=fn}observe(){}}}
 vm.runInNewContext(fs.readFileSync('../../webmail/plugins/bestmaple_tracking/compose-theme.js','utf8'),context)
 events.init()
 assert.equal(style,null)
 rc.editor.editor={getDoc:()=>doc,getContent:()=>original}
 events['editor-load']()
 assert.match(style.textContent,/#ffffff/)
 dark=true;observer()
 assert.match(style.textContent,/#172336/)
 assert.equal(rc.editor.editor.getContent(),original)
 dark=false;observer()
 assert.match(style.textContent,/#ffffff/)
 assert.doesNotMatch(style.textContent,/#172336/)
})
