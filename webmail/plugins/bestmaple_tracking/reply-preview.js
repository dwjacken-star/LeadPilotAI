/* Separate received HTML from the reply editor without dropping its MIME body. */
(function () {
  'use strict';
  if (!window.rcmail) return;
  var panel, previewHTML = '';
  var editorStyle = 'body > .maplemail-quoted-message{display:none!important}'
    + '.maplemail-reply-text{min-height:160px}';

  function labels(){var lang=String(document.documentElement.lang||'en').toLowerCase().split(/[-_]/)[0],text=(window.LeadPilotMailI18n.ui[lang]||window.LeadPilotMailI18n.ui.en);return {title:text.original,hint:text.quoteHint}}

  function removePreview() {
    if (panel) panel.remove();
    panel = null;
    previewHTML = '';
  }

  function refresh(editor) {
    var body = editor.getBody();
    // Only the outer quote belongs to this reply; older thread quotes can be
    // nested inside it. Ordinary new messages and forwards are left alone.
    var quote = body && Array.prototype.find.call(body.children, function (node) {
      return node.classList.contains('maplemail-quoted-message') && node.getAttribute('data-maplemail-original') === '1';
    });
    if (!quote) { removePreview(); return; }
    var container = editor.getContainer();
    if (!container || !container.parentNode) return;
    var text = labels();
    if (!panel) {
      panel = document.createElement('section');
      panel.id = 'maplemail-reply-preview';
      panel.className = 'maplemail-reply-preview';
      var heading = document.createElement('h3');
      heading.textContent = text.title;
      var hint = document.createElement('p');
      hint.textContent = text.hint;
      var frame = document.createElement('iframe');
      frame.title = text.title;
      // The original cannot edit the reply, execute scripts, submit forms or
      // navigate its parent. Links may be deliberately opened in a new tab.
      // Preserve authenticated inline-image URLs. Scripts remain disabled even
      // with same-origin access, and every preview element is read-only.
      frame.setAttribute('sandbox', 'allow-same-origin allow-popups allow-popups-to-escape-sandbox');
      frame.setAttribute('referrerpolicy', 'no-referrer');
      panel.appendChild(heading);
      panel.appendChild(hint);
      panel.appendChild(frame);
      container.parentNode.insertBefore(panel, container.nextSibling);
    }
    if (previewHTML !== quote.innerHTML) {
      previewHTML = quote.innerHTML;
      var doc = new DOMParser().parseFromString(previewHTML, 'text/html');
      doc.documentElement.setAttribute('contenteditable', 'false');
      doc.body.setAttribute('contenteditable', 'false');
      doc.querySelectorAll('[contenteditable]').forEach(function (node) { node.setAttribute('contenteditable', 'false'); });
      doc.querySelectorAll('script,form,iframe,object,embed,base').forEach(function (node) { node.remove(); });
      doc.querySelectorAll('a').forEach(function (node) { node.target = '_blank'; node.rel = 'noopener noreferrer'; });
      var style = doc.createElement('style');
      style.textContent = 'html{color-scheme:light}body{margin:0;padding:16px;background:#fff;color:#252d30;font:14px/1.6 Arial,sans-serif;overflow-wrap:anywhere}img{max-width:100%;height:auto}table{max-width:100%}blockquote{margin:12px 0;padding-left:12px;border-left:2px solid #d5dfda}';
      doc.head.appendChild(style);
      panel.querySelector('iframe').srcdoc = '<!doctype html>' + doc.documentElement.outerHTML;
    }
  }

  rcmail.addEventListener('editor-init', function (event) {
    if (rcmail.env.action !== 'compose') return;
    var config = event.config;
    var plugins = String(config.plugins || '').split(/\s+/);
    if (plugins.indexOf('noneditable') < 0) plugins.push('noneditable');
    config.plugins = plugins.join(' ');
    config.content_style = (config.content_style || '') + editorStyle;
  });

  rcmail.addEventListener('editor-load', function (event) {
    if (rcmail.env.action !== 'compose' || !event.ref.editor) return;
    var editor = event.ref.editor;
    refresh(editor);
    editor.on('SetContent Undo Redo', function () { refresh(editor); });
    editor.on('remove', removePreview);
  });
})();
