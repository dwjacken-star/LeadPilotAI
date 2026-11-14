import { defineStore } from 'pinia'
import {setLanguage} from '@/i18n'
import { isObject } from '@/utils'
import { getLanguages, setLanguage as setLanguageApi } from '@/api/modules/public'

interface LangResponse {
	current_language: string
	available_languages: Array<{ cn: string; name: string }>
}

export default defineStore('GlobalStore', () => {
	const domainSource = ref("")
	const lang = ref('en')

	const langList = ref<Array<{ cn: string; name: string }>>([])

	const isCollapse = ref(false)

	const temp_subject = ref("")

	const setCollapse = () => {
		isCollapse.value = !isCollapse.value
	}

 const languages=['en','zh','ja','de','tr','vi']
 let languageChosen=false
 function storedLanguage(){try{const value=localStorage.getItem('bestmaple.login-language');return languages.includes(value||'')?value:null}catch{return null}}
 function applyLanguage(language:string,persist=true){if(!languages.includes(language))return;lang.value=language;languageChosen=true;setLanguage(language);if(persist){try{localStorage.setItem('bestmaple.login-language',language)}catch{/* Session preference remains in memory. */}}}
 const getLang = async () => {
  const saved=storedLanguage();if(!languageChosen&&saved)applyLanguage(saved,false)
  if(langList.value.length>0){setLanguage(lang.value);return}
  const res=await getLanguages()
  if(isObject<LangResponse>(res)){
   if(!languageChosen)applyLanguage(res.current_language,false)
   langList.value=res.available_languages
  }
 }
 const setLang=async(language:string)=>{applyLanguage(language);await setLanguageApi({language})}

	return {
		domainSource,
		lang,
		langList,
		isCollapse,
		temp_subject,
		getLang,
		setLang,
 applyLanguage,
		setCollapse,
	}
}, {
	persist: [
		{
			pick: ["domainSource"],
			storage: localStorage
		}
	]
})
