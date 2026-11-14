(function () {
  'use strict';
  window.LeadPilotPlaceLanguage = function (select) {
    var existing = document.getElementById('leadpilot-public-language');
    if (existing) return;
    var style = document.createElement('style');
    style.textContent = '.brand{flex-wrap:wrap;align-items:center}.brand>div:not(.leadpilot-public-language){flex:1;min-width:0}.leadpilot-public-language{display:flex;justify-content:flex-end;margin-left:auto;flex:0 0 auto;max-width:100%}.leadpilot-public-language-select{font:inherit;font-size:14px;line-height:1.4;min-height:44px;width:140px;max-width:100%;padding:8px 12px;border:1px solid var(--line,#dce7e1);border-radius:10px;background:#fff;color:var(--ink,#18372d)}@media(max-width:600px){.leadpilot-public-language{flex-basis:100%;margin:4px 0 0}.leadpilot-public-language-select{width:150px}}';
    document.head.appendChild(style);
    var holder = document.createElement('div');
    holder.id = 'leadpilot-public-language';
    holder.className = 'leadpilot-public-language';
    select.className = 'leadpilot-public-language-select';
    holder.appendChild(select);
    var brand = document.querySelector('.brand');
    var main = document.querySelector('.container,.unsubscribe-container,main');
    if (brand) brand.appendChild(holder);
    else if (main) { holder.style.marginBottom = '20px'; main.insertBefore(holder, main.firstChild); }
    else { document.body.style.flexDirection = 'column'; document.body.insertBefore(holder, document.body.firstChild); }
    var packs = window.LeadPilotPublicI18n || {};
    // Keep old text aliases for already-saved public templates, while displaying
    // the current localized product brand. Do not touch addresses or form values.
    var originals = {};
    ['en','zh','ja','de','tr','vi'].forEach(function (language) {
      if (!packs[language]) return;
      originals['_source_' + language] = Object.assign({}, packs[language]);
      Object.keys(packs[language]).forEach(function (key) {
        var value = packs[language][key];
        if (typeof value !== 'string') return;
        value = value.replace(/\{'@'\}/g, '@').replace(/BESTMAPLE Mail/g, language === 'zh' ? '领客AI' : 'LeadPilot AI').replace(/BESTMAPLE/g, language === 'zh' ? '领客AI' : 'LeadPilot AI');
        if (key === 'p68ca6006c88c') value = language === 'zh' ? '领客智邮' : 'LeadPilot Smart Mail';
        packs[language][key] = value;
      });
    });
    Object.keys(originals).forEach(function (key) { packs[key] = originals[key]; });
    var labels = {en:'Choose your subscription',zh:'选择订阅的邮件列表',ja:'購読するリストを選択',de:'Abonnement auswählen',tr:'Aboneliğinizi seçin',vi:'Chọn danh sách đăng ký'};
    var originalTitle = document.title;
    function updateExtras() {
      var language = select.value;
      document.querySelectorAll('[data-public-groups-label]').forEach(function (node) {
        var value = labels[language] || labels.en;
        if (node.textContent !== value) node.textContent = value;
      });
      if (packs.en && packs[language]) Object.keys(packs.en).some(function (key) {
        if (packs.en[key] !== originalTitle) return false;
        document.title = packs[language][key] || originalTitle;
        return true;
      });
    }
    select.addEventListener('change', updateExtras);
    updateExtras();
  };
})();
