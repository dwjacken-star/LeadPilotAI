<?php
/** LeadPilot AI shell. Native Roundcube sessions, compose IDs and MIME remain authoritative. */
trait LeadPilotWorkbench
{
    private function workbench_init()
    {
        $rcmail = rcmail::get_instance();
        if (!$rcmail->user || !$rcmail->user->ID) return;
        $rcmail->output->set_env('leadpilot_mailbox', $rcmail->user->get_username());
        $rcmail->output->set_env('leadpilot_user', (string) $rcmail->user->ID);
        $this->include_stylesheet('workbench.css');
        $this->include_script('workbench-i18n.js');
        $this->include_script('compose-theme.js');
        $this->include_script('workbench-model.js');
        $this->include_script('workbench.js');
        $this->register_action('plugin.leadpilot-workbench', array($this, 'workbench_page'));
        $this->register_action('plugin.leadpilot-api', array($this, 'workbench_api'));
    }

    public function workbench_page()
    {
        $rcmail = rcmail::get_instance();
        $rcmail->output->set_env('leadpilot_shell', true);
        $rcmail->output->set_pagetitle('LeadPilot AI');
        $rcmail->output->send('bestmaple_tracking.workbench');
    }

    public function workbench_api()
    {
        $rcmail = rcmail::get_instance();
        $id = substr((string) rcube_utils::get_input_value('_lp_id', rcube_utils::INPUT_POST), 0, 100);
        $raw = (string) rcube_utils::get_input_value('_lp_payload', rcube_utils::INPUT_POST);
        try {
            if (strlen($raw) > 200000) throw new RuntimeException('Request too large');
            $request = json_decode($raw, true);
            $allowed = array('delivery_preview','draft_options_save', 'workbench_options', 'workbench_ai', 'workbench_templates', 'workbench_template', 'workbench_customer', 'workbench_save_customer', 'workbench_followup');
            if (!is_array($request) || !in_array($request['action'] ?? '', $allowed, true)) throw new RuntimeException('Unsupported workbench action');
            $result = array('id' => $id, 'success' => true, 'data' => $this->backend($request));
        } catch (Throwable $e) {
            $result = array('id' => $id, 'success' => false, 'message' => $e->getMessage());
        }
        $rcmail->output->command('plugin.leadpilot-result', $result);
        $rcmail->output->send();
    }
}
