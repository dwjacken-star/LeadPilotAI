import {readFile,stat} from 'node:fs/promises'
import {fileURLToPath,pathToFileURL} from 'node:url'
import path from 'node:path'
import {createRequire} from 'node:module'
const require=createRequire(import.meta.url),compiler=require('vue/compiler-sfc'),ts=require('typescript')
const root=path.resolve(fileURLToPath(new URL('../src',import.meta.url)))
export async function resolve(specifier,context,next){
 if(specifier.startsWith('@/'))specifier=pathToFileURL(path.join(root,specifier.slice(2))).href
 if(specifier.startsWith('file:')||specifier.startsWith('.')){
  const url=new URL(specifier,context.parentURL||import.meta.url)
  if(url.pathname.includes('/src/'))for(const suffix of ['','.ts','.vue','/index.ts']){const candidate=new URL(url.href+suffix);try{if((await stat(candidate)).isFile())return {url:candidate.href,shortCircuit:true}}catch{}}
 }
 return next(specifier,context)
}
export async function load(url,context,next){
 if(url.endsWith('.vue')){const {descriptor}=compiler.parse(await readFile(new URL(url),'utf8')),compiled=compiler.compileScript(descriptor,{id:'workbench-test',inlineTemplate:true});return {format:'module',source:ts.transpileModule(compiled.content,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText,shortCircuit:true}}
 if(url.endsWith('.ts')&&url.includes('/src/'))return {format:'module',source:ts.transpileModule(await readFile(new URL(url),'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText,shortCircuit:true}
 return next(url,context)
}
