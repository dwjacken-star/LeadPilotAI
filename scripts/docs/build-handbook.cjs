'use strict'
const fs = require('node:fs')
const path = require('node:path')
const { createRequire } = require('node:module')
const { pathToFileURL } = require('node:url')
const frontendRequire = createRequire(path.resolve('core/frontend/package.json'))
const MarkdownIt = frontendRequire('markdown-it')
const root = process.cwd()
const directory = path.join(root, 'docs/handbook')
const chapters = ['technical-architecture', 'functional-architecture', 'user-manual', 'operations-guide']
const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const css = `
@page {size:A4; margin:17mm 16mm 18mm}
@page interface {size:A4 landscape; margin:12mm 13mm 16mm}
*{box-sizing:border-box}html{font-family:"Microsoft YaHei","Segoe UI",Arial,sans-serif;color:#23384e;background:#fff;font-size:12px;line-height:1.72}body{margin:0}h1{font-size:28px;line-height:1.35;color:#15334c}h2{font-size:19px;color:#087d87;border-bottom:1px solid #d5e6ea;padding-bottom:5px;margin-top:25px;break-after:avoid}h3{font-size:15px;color:#21445c;break-after:avoid}p,li{orphans:3;widows:3}a{color:#047b85;text-decoration:none}strong{color:#173c53}table{border-collapse:collapse;width:100%;font-size:10.5px;line-height:1.55;margin:14px 0;table-layout:auto}thead{display:table-header-group}th,td{border:1px solid #d5e0e9;padding:7px 8px;vertical-align:top;word-break:normal;overflow-wrap:anywhere}th{background:#edf5f8;color:#15354f}tr{break-inside:avoid}code{font-family:Consolas,"Microsoft YaHei",monospace;font-size:.91em;background:#edf3f7;padding:1px 3px;overflow-wrap:anywhere}pre{background:#edf3f7;border-left:3px solid #1395a0;padding:12px;white-space:pre-wrap;overflow-wrap:anywhere;font-size:10px;break-inside:avoid}pre code{padding:0;background:none}blockquote{border-left:3px solid #1a9ca5;padding:8px 15px;background:#eef8f8;margin:15px 0}.cover{min-height:240mm;display:flex;flex-direction:column;justify-content:center;break-after:page}.cover .brand{font-size:45px;font-weight:800;color:#086c76;margin-bottom:14px}.cover p{font-size:16px}.cover .note{font-size:12px;background:#eef6f9;padding:20px;border-radius:8px}.contents{break-after:page}.contents ul{padding-left:20px}.contents>ul>li{margin-bottom:10px}.chapter{break-before:page}.interface{page:interface;break-before:page;break-after:page;margin:0;width:100%;text-align:center;break-inside:avoid}.interface figcaption{text-align:left;font-size:15px;font-weight:700;line-height:1.5;color:#15354f;margin:0 0 8px}.interface img{display:block;width:auto;height:auto;max-width:100%;max-height:158mm;object-fit:contain;margin:0 auto;border:1px solid #d5e0e9}.interface .image-note{text-align:left;font-size:10px;color:#536d82;margin-top:7px}.screen-only{background:#ecf7f7;padding:12px;border-radius:8px}.chapter-title-note{color:#61798a;font-size:11px}@media screen{body{max-width:1040px;margin:30px auto;padding:30px;box-shadow:0 1px 20px #dfe6ed}.cover{min-height:550px}.interface{margin:28px 0}.interface img{max-height:none;width:100%}.chapter{margin-top:50px}.screen-only{margin-bottom:25px}}@media print{.screen-only{display:none}}
`
function standalone(tokens,index){const inline=tokens[index];return inline?.type==='inline'&&inline.children?.length===1&&inline.children[0].type==='image'}
function renderLocale(locale){
 const zh=locale==='zh-CN',toc=[],body=[]
 for(let n=0;n<chapters.length;n++){
  const file=path.join(directory,locale,chapters[n]+'.md')
  const md=new MarkdownIt({html:true,linkify:true,typographer:false})
  const tokens=md.parse(fs.readFileSync(file,'utf8'),{})
  const title=tokens.find(t=>t.type==='inline')?.content||chapters[n]
  const sectionToc=[];let i=0
  tokens.forEach((token,index)=>{if(token.type==='heading_open'){const id='chapter-'+n+'-section-'+i++;token.attrSet('id',id);if(token.tag==='h2')sectionToc.push({id,title:tokens[index+1].content})}})
  const normalOpen=md.renderer.rules.paragraph_open
  md.renderer.rules.paragraph_open=(t,i,o,e,self)=>standalone(t,i+1)?'':normalOpen?normalOpen(t,i,o,e,self):self.renderToken(t,i,o)
  const normalClose=md.renderer.rules.paragraph_close
  md.renderer.rules.paragraph_close=(t,i,o,e,self)=>standalone(t,i-1)?'':normalClose?normalClose(t,i,o,e,self):self.renderToken(t,i,o)
  md.renderer.rules.image=(tokens,index)=>{
   const token=tokens[index],source=token.attrGet('src'),alt=token.content
   const note=source.includes('/screenshots/')?(zh?'真实前端组件；虚构数据；非生产环境截图。':'Real frontend components; fictional data; not a production capture.'):(zh?'架构图基于源码与默认编排分析。':'Diagram based on source and default orchestration.')
   return `<figure class="interface"><figcaption>${esc(alt)}</figcaption><img src="${esc(source)}" alt="${esc(alt)}"><div class="image-note">${esc(note)}</div></figure>`
  }
  body.push(`<section class="chapter">${md.renderer.render(tokens,md.options,{})}</section>`)
  toc.push(`<li><a href="#chapter-${n}-section-0">${esc(title)}</a><ul>${sectionToc.map(s=>`<li><a href="#${s.id}">${esc(s.title)}</a></li>`).join('')}</ul></li>`)
 }
 const title=zh?'领客AI · 架构与操作指南':'LeadPilot AI · Architecture and Operations Handbook'
 const subtitle=zh?'技术架构 · 功能架构 · 用户操作 · 部署运维':'Technical architecture · Functional architecture · User manual · Operations'
 const note=zh?'本文件使用真实应用前端与虚构演示数据。未连接生产、未向客户发信、未发布社媒。SQL 和凭据通过私有流程配套；截图不代表生产验收。':'This handbook uses the real application frontend with fictional demonstration data. No production connection, customer mail or social publication. SQL and credentials remain private; screenshots are not production acceptance.'
 return `<!doctype html><html lang="${locale}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${esc(title)}</title><style>${css}</style></head><body><div class="screen-only">${zh?'可打印 HTML；对应 PDF 和 Markdown 位于同一文档目录。':'Printable HTML; companion PDF and Markdown are in the handbook directory.'}</div><section class="cover"><div class="brand">LeadPilot AI</div><h1>${esc(title)}</h1><p>${esc(subtitle)}</p><p>${zh?'版本 1.0 · 2026-11-14 · 源码基线 cf57fb3':'Version 1.0 · 2026-11-14 · Source baseline cf57fb3'}</p><p class="note">${esc(note)}</p><p><a href="https://github.com/dwjacken-star/LeadPilotAI">github.com/dwjacken-star/LeadPilotAI</a></p></section><nav class="contents"><h1>${zh?'目录':'Contents'}</h1><ul>${toc.join('')}</ul></nav>${body.join('\n')}</body></html>`
}
async function main(){
 fs.mkdirSync(path.join(directory,'html'),{recursive:true});fs.mkdirSync(path.join(directory,'pdf'),{recursive:true})
 const moduleFile=process.env.PLAYWRIGHT_MODULE||path.join(root,'.tools/social-browser-runtime/node_modules/playwright-core/index.mjs')
 const {chromium}=await import(pathToFileURL(moduleFile).href)
 const browser=await chromium.launch({headless:true,executablePath:process.env.DOC_BROWSER||'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'})
 const result=[]
 try{
  for(const locale of ['zh-CN','en']){
   const base=`LeadPilotAI-User-and-Operations-Guide-${locale}`
   const htmlFile=path.join(directory,'html',base+'.html'),pdfFile=path.join(directory,'pdf',base+'.pdf')
   fs.writeFileSync(htmlFile,renderLocale(locale))
   const page=await browser.newPage()
   await page.route('**/*',route=>route.request().url().startsWith('file:///')?route.continue():route.abort())
   await page.goto(pathToFileURL(htmlFile).href,{waitUntil:'networkidle'})
   await page.evaluate(async()=>{await document.fonts.ready;await Promise.all([...document.images].map(image=>image.complete?Promise.resolve():new Promise(resolve=>{image.onload=resolve;image.onerror=resolve})))})
   const broken=await page.evaluate(()=>[...document.images].filter(i=>!i.complete||!i.naturalWidth).map(i=>i.getAttribute('src')))
   if(broken.length)throw new Error('Missing handbook images: '+broken.join(', '))
   await page.pdf({path:pdfFile,preferCSSPageSize:true,printBackground:true,displayHeaderFooter:true,headerTemplate:'<div></div>',footerTemplate:`<div style="width:100%;font:9px Arial;text-align:center;color:#5b7184">LeadPilot AI · 2026-11-14 · cf57fb3 · <span class="pageNumber"></span> / <span class="totalPages"></span></div>`,outline:true,tagged:true})
   result.push({locale,html:path.relative(root,htmlFile).replaceAll('\\','/'),pdf:path.relative(root,pdfFile).replaceAll('\\','/'),bytes:fs.statSync(pdfFile).size,images:await page.locator('img').count()})
   await page.close();console.log('GENERATED '+base+'.pdf')
  }
 }finally{await browser.close()}
 fs.writeFileSync(path.join(directory,'build-manifest.json'),JSON.stringify({version:'1.0',date:'2026-11-14',source_baseline:'cf57fb3',production_access:false,outputs:result},null,2)+'\n')
 console.log(JSON.stringify(result,null,2))
}
main().catch(error=>{console.error(error);process.exitCode=1})
