const menuKeys=['workspace','overview','crm','materials','social','intelligence','outreach','market','template','api','contacts','domain','mailbox','smtp','logs','settings','automation']
export function sortMenuRoutes<T extends {path:string;meta?:{key?:unknown}}>(routes:T[]):T[] {
 const rank=(route:T)=>{const index=menuKeys.indexOf(String(route.meta?.key||''));return index<0?menuKeys.length:index}
 return routes.filter(route=>route && typeof route.path==='string').sort((a,b)=>rank(a)-rank(b))
}
