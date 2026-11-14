/* Theme only the editor document's head; never rewrite outgoing message HTML. */
(function () {
 'use strict';
 function apply() {
  var rc=window.rcmail;if(!rc||rc.env.action!=='compose')return;
  var editor=rc.editor&&rc.editor.editor;if(!editor||!editor.getDoc)return;
  var doc=editor.getDoc();if(!doc||!doc.head)return;
  var style=doc.getElementById('leadpilot-editor-theme');
  if(!style){style=doc.createElement('style');style.id='leadpilot-editor-theme';doc.head.appendChild(style)}
  var dark=document.documentElement.classList.contains('dark-mode');
  style.textContent='html{color-scheme:'+(dark?'dark':'light')+'}body.mce-content-body{background:'+(dark?'#172336':'#ffffff')+'!important;color:'+(dark?'#e3edf8':'#172c47')+';min-height:100%;}'+(dark?'body.mce-content-body a:not([style*="color"]){color:#71e1d3}':'');
 }
 window.LeadPilotApplyEditorTheme=apply;
 if(window.rcmail){rcmail.addEventListener('init',apply);rcmail.addEventListener('editor-load',apply)}
 new MutationObserver(apply).observe(document.documentElement,{attributes:true,attributeFilter:['class']});
})();
