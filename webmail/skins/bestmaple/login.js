(function () {
	'use strict';
	var copy = window.LeadPilotMailI18n && window.LeadPilotMailI18n.login;
 if(!copy)return;
	function applyLanguage(language) {
		var text = copy[language] || copy.zh;
		document.documentElement.lang = language === 'zh' ? 'zh-CN' : language;
		document.querySelectorAll('[data-bm]').forEach(function (element) { element.textContent = text[element.getAttribute('data-bm')] || ''; });
		var selector=document.getElementById('leadpilot-login-language');if(selector){selector.value=language;selector.setAttribute('aria-label',text.language||'Language')}
		var languageField=document.querySelector('input[name="_language"]');if(!languageField){languageField=document.createElement('input');languageField.type='hidden';languageField.name='_language';var form=document.getElementById('login-form');if(form)form.appendChild(languageField)}languageField.value=({zh:'zh_CN',en:'en_US',ja:'ja_JP',de:'de_DE',tr:'tr_TR',vi:'vi_VN'})[language]||'en_US';
		var username = document.getElementById('rcmloginuser'), password = document.getElementById('rcmloginpwd'), submit = document.getElementById('rcmloginsubmit');
		if (username) { username.placeholder = 'name@best-maple.com'; username.autocomplete = 'username'; username.setAttribute('autocapitalize', 'none'); username.setAttribute('spellcheck', 'false'); username.setAttribute('aria-label', text.user); }
		if (password) { password.placeholder = text.passwordHint; password.autocomplete = 'current-password'; password.setAttribute('aria-label', text.password); }
		var userLabel = document.querySelector('label[for="rcmloginuser"]'), passwordLabel = document.querySelector('label[for="rcmloginpwd"]');
		if (userLabel) userLabel.textContent = text.user;
		if (passwordLabel) passwordLabel.textContent = text.password;
		if (submit) { if (submit.tagName === 'INPUT') submit.value = text.submit; else submit.textContent = text.submit; }
		try { localStorage.setItem('bestmaple.login-language', language); } catch (e) { /* Language switching also works without local storage. */ }
	}
	function start() {
		var language = document.documentElement.getAttribute('data-leadpilot-language') || 'zh';
		try { var saved=localStorage.getItem('bestmaple.login-language');if(!document.documentElement.getAttribute('data-leadpilot-language')&&copy[saved])language=saved; } catch (e) { }
		applyLanguage(language);
		var selector=document.getElementById('leadpilot-login-language');if(selector)selector.addEventListener('change',function(){applyLanguage(selector.value)});
	}
	if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start); else start();
})();
