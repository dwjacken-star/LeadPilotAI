/* Optional enhancement. The full policy text is present in the HTML. No network calls. */
(() => {
  const root = document.documentElement;
  const controls = [...document.querySelectorAll('[data-language-button]')];
  function setLanguage(language) {
    const lang = language === 'zh' ? 'zh' : 'en';
    root.dataset.language = lang;
    root.lang = lang === 'zh' ? 'zh-CN' : 'en';
    controls.forEach(button => button.setAttribute('aria-pressed', String(button.dataset.languageButton === lang)));
    const next = new URL(location.href);
    if (lang === 'en') next.searchParams.delete('lang'); else next.searchParams.set('lang', lang);
    history.replaceState(null, '', next.pathname + next.search + next.hash);
    document.querySelectorAll('a[href^="/legal/"]').forEach(link => {
      const target = new URL(link.getAttribute('href'), location.origin);
      if (lang === 'en') target.searchParams.delete('lang'); else target.searchParams.set('lang', lang);
      link.setAttribute('href', target.pathname + target.search + target.hash);
    });
  }
  controls.forEach(button => button.addEventListener('click', () => setLanguage(button.dataset.languageButton)));
  setLanguage(new URLSearchParams(location.search).get('lang'));
  if (matchMedia('(max-width:960px)').matches) document.querySelectorAll('.toc details').forEach(item => { item.open = false; });
  document.querySelectorAll('[data-print]').forEach(button => button.addEventListener('click', () => window.print()));
  document.querySelectorAll('[data-copy]').forEach(button => button.addEventListener('click', async () => {
    const box = button.closest('.request-box');
    const text = box.querySelector('.request-template').textContent;
    const status = box.querySelector('.copy-status');
    const zh = root.dataset.language === 'zh';
    try {
      if (!navigator.clipboard) throw new Error('Clipboard unavailable');
      await navigator.clipboard.writeText(text);
      status.textContent = zh ? '已复制。请粘贴到邮件并发送；复制不代表已提交。' : 'Copied. Paste into an email and send it; copying does not submit a request.';
    } catch {
      const selection = getSelection();
      const range = document.createRange();
      range.selectNodeContents(box.querySelector('.request-template'));
      selection.removeAllRanges(); selection.addRange(range);
      status.textContent = zh ? '请手动复制已选内容，并发送邮件。' : 'Please copy the selected text manually and email your request.';
    }
  }));
})();
