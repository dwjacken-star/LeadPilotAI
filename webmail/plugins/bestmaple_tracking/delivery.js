if (window.rcmail) {
  rcmail.addEventListener('init', function () {
    var panel = document.getElementById('maplemail-delivery');
    if (!panel) return;
    var form = rcmail.gui_objects.messageform;
    if (typeof form === 'string') form = document.getElementById(form) || document.forms[form];
    if (!form || form.tagName !== 'FORM') form = document.forms.form;
    if (!form) return;
    function field(name, value) {
      var input = form.querySelector('input[name="' + name + '"]');
      if (!input) { input = document.createElement('input'); input.type = 'hidden'; input.name = name; form.appendChild(input); }
      input.value = value;
    }
    function sync() {
      field('_maplemail_delivery_present', '1');
      panel.querySelectorAll('input[data-maplemail-option]').forEach(function (input) {
        field('_maplemail_' + input.dataset.maplemailOption, input.checked ? '1' : '0');
      });
    }
    panel.addEventListener('change', function(){sync();if(window.LeadPilotSaveDraftOptions)void window.LeadPilotSaveDraftOptions().catch(function(){})});
    form.addEventListener('submit', sync);
    sync();
  });
}
