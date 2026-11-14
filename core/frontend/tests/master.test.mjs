import test from 'node:test'
import assert from 'node:assert/strict'
import {channels,flows,canReview,canResume,offeringFields,previewDocument,mergeOptions,replyNoticeKey} from '../src/features/master/display.ts'
import {routeAllowed} from '../src/features/growth/access.ts'

test('only native authorized channels are offered and all six workflows remain available',()=>{
 assert.deepEqual([...channels],['email','facebook','linkedin','whatsapp'])
 assert.equal(flows.length,6)
 assert.equal(routeAllowed({roles:[{permissions:['intelligence:view']}]},'/intelligence'),true)
 assert.equal(routeAllowed({roles:[{permissions:['outreach:list']}]},'/intelligence'),false)
})
test('queued and uncertain deliveries cannot re-enter content approval',()=>{
 for(const state of ['draft','approved','blocked','rejected'])assert.equal(canReview(state),true)
 for(const state of ['queued','accepted','published','unknown','sending'])assert.equal(canReview(state),false)
 for(const state of ['waiting_approval','configuration_required','budget_exhausted','paused'])assert.equal(canResume(state),true)
 assert.equal(canResume('completed'),false)
})
test('different industry data types retain values and stable selections',()=>{
 const pack={config:{fields:[{key:'oe',type:'text'},{key:'coverage',type:'list'},{key:'days',type:'number'},{key:'available',type:'boolean'}]}}
 const fields=offeringFields(pack,{oe:'0000123',days:0,available:false})
 assert.equal(fields[0].value,'0000123');assert.deepEqual(fields[1].value,[]);assert.equal(fields[2].value,0);assert.equal(fields[3].value,false)
 assert.deepEqual(mergeOptions([{value:1,label:'Selected'}],[{value:2,label:'Next page'}]),[{value:1,label:'Selected'},{value:2,label:'Next page'}])
})
test('rich text previews preserve paragraphs with a script-blocking document policy',()=>{
 const html='<p>Dear team,</p><p>Product details<br>Line two</p>'
 const doc=previewDocument(html)
 assert.ok(doc.includes(html));assert.ok(doc.includes("default-src 'none'"));assert.ok(doc.includes('width=device-width'))
})
test('uncertain automatic replies are visibly distinct from sent replies and unsent drafts',()=>{
 assert.equal(replyNoticeKey({automatic_reply_id:1,automatic_reply_status:'unknown'}),'intel.auto_reply_check')
 assert.equal(replyNoticeKey({automatic_reply_id:1,automatic_reply_status:'sent'}),'intel.auto_reply_sent')
 assert.equal(replyNoticeKey({}),'intel.reply_review_hint')
})
