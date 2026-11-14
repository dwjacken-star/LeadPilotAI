// Keep this formatting contract aligned with public.NormalizeEmailHTML in Go.
const htmlTag = /<(?:!doctype\s+html\b[^>]*|\/?(?:html|head|body|style|table|tbody|thead|tfoot|tr|td|th|p|div|br|ul|ol|li|h[1-6]|span|a|img|strong|b|em|i|u|s|blockquote|pre|hr|font|center|section|article|meta|title|link)(?:\s[^<>]*|\/?)?)>/is
export function escapeEmailText(value:string):string {
 return value.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&#34;').replace(/'/g,'&#39;')
}
export function normalizeEmailHTML(content:string):string {
 content=content.replace(/(Powered\s+by\s*(?:<\/span>\s*)?<a\b[^>]*>)(?:BillionMail|MapleMail)(<\/a>)/gis,(_match,prefix,suffix)=>prefix.replace(/https:\/\/www\.billionmail\.com\/?/g,'https://mail.best-maple.com/')+'LeadPilot AI'+suffix).replace(/(Powered\s+by\s+)(?:BillionMail|MapleMail)\b/gi,'$1LeadPilot AI')
 if(!content.trim())return content
 const actions:string[]=[]
 let masked=content.replace(/{{[\s\S]*?}}/g,value=>{actions.push(value);return `MAPLEMAILACTIONTOKEN${actions.length-1}END`})
 if(htmlTag.test(masked))return content
 masked=masked.replace(/\r\n/g,'\n').replace(/\r/g,'\n')
 let result=`<div style="font-family:Arial,'Microsoft YaHei',sans-serif;font-size:14px;line-height:1.6;color:#222;">`+masked.split(/\n[\t ]*\n+/).map(part=>`<p style="margin:0 0 16px;white-space:pre-wrap;">${escapeEmailText(part).replace(/\n/g,'<br>')}</p>`).join('')+'</div>'
 actions.forEach((action,index)=>{result=result.replaceAll(`MAPLEMAILACTIONTOKEN${index}END`,action)})
 return result
}
