export interface ContactImportDraft {file_data:string;file_type:string;filter_keywords:string[];preview_token:string;skip_invalid:boolean;keep_pending:boolean;validation_sender:string}
export const emptyContactImport=():ContactImportDraft=>({file_data:'',file_type:'',filter_keywords:[],preview_token:'',skip_invalid:true,keep_pending:false,validation_sender:''})
export function parseImportKeywords(text:string):string[]{return [...new Set(text.split(/[\n,，;；]+/).map(s=>s.trim()).filter(Boolean))]}
export function importableContactCount(preview:{valid?:number;unconfirmed?:number}|null,keepPending:boolean):number{return Number(preview?.valid||0)+(keepPending?Number(preview?.unconfirmed||0):0)}
