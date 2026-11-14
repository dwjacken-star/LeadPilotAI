import i18n from './index'
import en from './lang/en.json'
import zh from './lang/zh.json'
const keys=new Map<string,string>()
function scan(value:any,path=''){if(!value||typeof value!=='object')return;for(const [k,v] of Object.entries(value)){const key=path?path+'.'+k:k;if(typeof v==='string'){if(!keys.has(v))keys.set(v,key)}else scan(v,key)}}
scan(en);scan(zh)
export function uiText(value:string):string {const key=i18n.global.te(value)?value:keys.get(value);return key?String(i18n.global.t(key)):value}
