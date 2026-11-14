<template>
 <div class="mail-editor"><div ref="toolbarRef" class="mail-toolbar"/><div ref="editorRef" class="mail-body"/></div>
</template>
<script setup lang="ts">
import '@wangeditor/editor/dist/css/style.css'
import { createEditor, createToolbar, i18nChangeLanguage, type IDomEditor } from '@wangeditor/editor'
const value=defineModel<string>('value',{default:''})
const {locale}=useI18n()
const toolbarRef=ref<HTMLElement>(),editorRef=ref<HTMLElement>()
let editor:IDomEditor|undefined,updating=true,lastHTML=''
let prefix='',suffix=''
// WangEditor treats an outer div as one paragraph. Unwrap only layout shells
// before parsing, and restore their language/direction/style after edits.
function editorBody(source:string){
 prefix='';suffix=''
 let parent:HTMLElement=new DOMParser().parseFromString(source,'text/html').body
 while(parent.children.length===1&&parent.firstElementChild?.tagName==='DIV'&&[...parent.childNodes].every(node=>node.nodeType!==Node.TEXT_NODE||!node.textContent?.trim())){
  const child=parent.firstElementChild as HTMLElement
  const shell=child.cloneNode(false) as HTMLElement
  prefix+=shell.outerHTML.slice(0,-6);suffix='</div>'+suffix;parent=child
 }
 return parent.innerHTML
}
function draftBody(html:string){
 const body=document.createElement('div');body.innerHTML=html
 for(const p of body.querySelectorAll('p')){if(!p.style.margin)p.style.margin='0 0 16px'}
 return prefix+body.innerHTML+suffix
}
onMounted(()=>{
 i18nChangeLanguage(locale.value.startsWith('zh')?'zh-CN':'en')
 editor=createEditor({selector:editorRef.value!,html:editorBody(value.value),mode:'simple',config:{autoFocus:false,onChange(current){const html=current.getHtml();if(!updating&&html!==lastHTML){lastHTML=html;value.value=draftBody(html)}}}})
 lastHTML=editor.getHtml();updating=false
 createToolbar({editor,selector:toolbarRef.value!,config:{toolbarKeys:['bold','italic','underline','color','fontSize','bulletedList','numberedList','insertLink','undo','redo']}})
})
watch(value,html=>{if(editor && draftBody(editor.getHtml())!==html){updating=true;try{editor.setHtml(editorBody(html));lastHTML=editor.getHtml()}finally{updating=false}}})
onBeforeUnmount(()=>{editor?.destroy();editor=undefined})
</script>
<style scoped>
.mail-editor{border:1px solid var(--color-border-1);border-radius:8px;overflow:hidden;background:#fff;color:#243247}.mail-toolbar{border-bottom:1px solid #e5e7eb}.mail-body{height:480px;overflow-y:auto}:deep(.w-e-text-container){color:#243247;background:#fff}
</style>
