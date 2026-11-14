import test from 'node:test'
import assert from 'node:assert/strict'
import {emptyContactImport,importableContactCount} from '../src/features/contacts/import-state.ts'

test('pending import is explicit and each new draft starts unchecked',()=>{
 const draft=emptyContactImport()
 const preview={valid:2,unconfirmed:3,queued:100,checking:2,invalid:4,filtered:5,duplicate:6}
 assert.equal(draft.keep_pending,false)
 assert.equal(importableContactCount(preview,draft.keep_pending),2)
 draft.keep_pending=true
 assert.equal(importableContactCount(preview,draft.keep_pending),5)
 draft.keep_pending=false
 assert.equal(importableContactCount(preview,draft.keep_pending),2)
 assert.equal(emptyContactImport().keep_pending,false)
})

test('a pending-only preview is importable only when selected',()=>{
 assert.equal(importableContactCount({valid:0,unconfirmed:3},false),0)
 assert.equal(importableContactCount({valid:0,unconfirmed:3},true),3)
 assert.equal(importableContactCount(null,true),0)
})
