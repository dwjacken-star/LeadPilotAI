export interface Market {country:string;language:string;timezone:string;city?:string}
export interface MarketCatalog {countries:{code:string;language:string;timezone:string}[];languages:string[];timezones:string[]}
export interface Choice {value:string;label:string}

export function normalizeDeliveryOptions(config:Record<string,unknown>,existing=false) {
 const flag=(key:string,fallback:boolean)=>typeof config[key]==='boolean'?config[key] as boolean:fallback
 return {warmup:flag('warmup',false),unsubscribe:flag('unsubscribe',true),track_open:flag('track_open',!existing),track_click:flag('track_click',!existing)}
}

function displayNames(locale:string,type:'region'|'language') {
 try{return new Intl.DisplayNames([locale.replace('_','-'),'en'],{type})}catch{return new Intl.DisplayNames('en',{type})}
}
// Include saved choices even when a provider or locale catalog has changed.
export function retainChoice(options:Choice[],selected?:string):Choice[] {
 return selected&&!options.some(option=>option.value===selected)?[{value:selected,label:selected},...options]:options
}
export function marketChoices(catalog:MarketCatalog,locale:string) {
 const regions=displayNames(locale,'region'),languages=displayNames(locale,'language')
 const label=(names:Intl.DisplayNames,code:string)=>{try{return names.of(code)||code}catch{return code}}
 const order=(a:Choice,b:Choice)=>a.label.localeCompare(b.label,locale)
 return {
  countries:catalog.countries.map(({code})=>({value:code,label:`${label(regions,code)} (${code})`})).sort(order),
  languages:catalog.languages.map(code=>({value:code,label:`${label(languages,code)} (${code})`})).sort(order),
  timezones:catalog.timezones.map(code=>({value:code,label:code.replaceAll('_',' ')}))
 }
}
export function countryChanged(market:Market,code:string,catalog:MarketCatalog) {
	if(market.country!==code&&'city' in market)market.city=''
 market.country=code
 const defaults=catalog.countries.find(country=>country.code===code)
 if(defaults){market.language=defaults.language;market.timezone=defaults.timezone}
}
