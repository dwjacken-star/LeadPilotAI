const fs=require('fs'),path=require('path');
const {createRequire}=require('module');
const vueRequire=createRequire(require.resolve('vue-i18n'));
const compilerRequire=createRequire(vueRequire.resolve('@intlify/core-base'));
const {baseCompile}=compilerRequire('@intlify/message-compiler');
const flatten=(value,prefix='')=>Object.fromEntries(Object.entries(value).flatMap(([key,item])=>item&&typeof item==='object'&&!Array.isArray(item)?Object.entries(flatten(item,prefix+key+'.')):[[prefix+key,item]]));
const langs=Object.fromEntries(['en','zh','ja','de','tr','vi'].map(lang=>[lang,flatten(JSON.parse(fs.readFileSync(path.join(__dirname,'../src/i18n/lang',lang+'.json'),'utf8')))]));
const keys=[...new Set(Object.values(langs).flatMap(value=>Object.keys(value)))];let failed=false;
for(const key of keys){for(const [lang,values] of Object.entries(langs)){if(!(key in values)){console.error('Missing',lang,key);failed=true}}}
const variables=value=>[...new Set([...String(value).matchAll(/\{([a-zA-Z_]\w*)\}/g)].map(match=>match[1]))].sort().join(',');
for(const key of keys){const expected=variables(langs.en[key]);for(const lang of ['zh','ja','de','tr','vi'])if(variables(langs[lang][key])!==expected){console.error('Placeholder mismatch',lang,key);failed=true}}
function compileMessages(value,lang,key=''){
 if(value && typeof value==='object'){for(const [name,message]of Object.entries(value))compileMessages(message,lang,key?key+'.'+name:name);return}
 if(typeof value!=='string')return
 baseCompile(value,{onError:error=>{console.error('Invalid message syntax',lang,key,error.code,error.message);failed=true}})
}
for(const lang of ['en','zh','ja','de','tr','vi'])compileMessages(JSON.parse(fs.readFileSync(path.join(__dirname,'../src/i18n/lang',lang+'.json'),'utf8')),lang)
if(failed)process.exit(1);console.log(`i18n: ${keys.length} keys aligned across en/zh/ja/de/tr/vi; interpolation variables match; all message syntax compiles.`);
