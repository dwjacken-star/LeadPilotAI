<?php
/** MapleMail compose controls and server-validated delivery preparation. */
require_once __DIR__ . '/workbench.php';
require_once __DIR__ . '/login-localization.php';
class bestmaple_tracking extends rcube_plugin
{
    use LeadPilotWorkbench;
    use LeadPilotLoginLocalization;
    public $task = 'mail|settings|addressbook|login';

    public function init()
    {
        $this->init_login_localization();
        $this->add_hook('login_after', array($this, 'login_language'));
        $rcmail = rcmail::get_instance();
        $native = json_decode(file_get_contents(__DIR__ . '/native-labels.json'), true);
        $code = substr(strtolower((string) ($_SESSION['language'] ?? 'en')), 0, 2);
        if (!empty($native[$code])) $rcmail->load_language(null, array(), $native[$code]);
        $this->workbench_init();
        $this->add_hook('template_container', array($this, 'compose'));
        $this->add_hook('message_compose_body', array($this, 'reply_preview'));
        $this->add_hook('message_before_send', array($this, 'prepare'));
        if (rcmail::get_instance()->action === 'compose') {
            $this->include_script('reply-preview.js');
            $this->include_stylesheet('reply-preview.css');
        }
    }

    public function login_language($args)
    {
        $language = (string) rcube_utils::get_input_value('_language', rcube_utils::INPUT_POST);
        if (in_array($language, array('en_US', 'zh_CN', 'ja_JP', 'de_DE', 'tr_TR', 'vi_VN'), true)) {
            $rcmail = rcmail::get_instance();
            if ($rcmail->user && $rcmail->user->ID) {
                // Roundcube saves users.language from the session, not the prefs argument.
                $rcmail->load_language($language);
                $_SESSION['language'] = $language;
                $rcmail->user->language = $language;
                $rcmail->config->set('language', $language);
                $rcmail->user->save_prefs(array('language' => $language));
            }
        }
        return $args;
    }

    public function reply_preview($args)
    {
        // Roundcube has already washed the received HTML and resolved inline
        // attachments here. Keep it in the submitted body for drafts/sending,
        // but render it outside the editable reply area in the browser.
        if (empty($args['html']) || ($args['mode'] ?? null) !== rcmail_sendmail::MODE_REPLY || trim((string) ($args['body'] ?? '')) === '') return $args;
        $args['body'] = '<div class="maplemail-reply-text"><p><br></p></div>'
            . '<div class="maplemail-quoted-message mceNonEditable" data-maplemail-original="1" contenteditable="false">'
            . $args['body'] . '</div>';
        return $args;
    }

    private function defaults()
    {
        return array('unsubscribe' => true, 'track_click' => true, 'track_open' => true, 'warmup' => false);
    }

    private function labels()
    {
        $language = substr(strtolower((string) ($_SESSION['language'] ?? 'en')),0,2);
        $packs = json_decode(file_get_contents(__DIR__ . '/labels.json'),true);
        return $packs[$language] ?? $packs['en'];
    }

    protected function backend($request)
    {
        $rcmail = rcmail::get_instance();
        $base = rtrim((string) $rcmail->config->get('bestmaple_tracking_base_url', ''), '/');
        $url = parse_url($base);
        if (!$url || ($url['scheme'] ?? '') !== 'https' || empty($url['host']) || !empty($url['user'])) throw new RuntimeException('Configure a valid HTTPS public mailbox URL');
        $username = $rcmail->user->get_username();
        $password = $rcmail->decrypt($_SESSION['password'] ?? '');
        // PHP's associative JSON decoder turns {} into []; preserve the Go object contract.
        if (isset($request['data']) && is_array($request['data']) && count($request['data']) === 0) $request['data'] = (object) array();
        $curl = curl_init($base . '/webmail/delivery');
        curl_setopt_array($curl, array(CURLOPT_POST => true, CURLOPT_POSTFIELDS => json_encode($request), CURLOPT_HTTPHEADER => array('Content-Type: application/json'), CURLOPT_USERPWD => $username . ':' . $password, CURLOPT_HTTPAUTH => CURLAUTH_BASIC, CURLOPT_RETURNTRANSFER => true, CURLOPT_CONNECTTIMEOUT => 8, CURLOPT_TIMEOUT => (($request['action'] ?? '') === 'workbench_ai' ? 100 : 25), CURLOPT_FOLLOWLOCATION => false, CURLOPT_SSL_VERIFYPEER => true, CURLOPT_SSL_VERIFYHOST => 2));
        $raw = curl_exec($curl);
        $status = curl_getinfo($curl, CURLINFO_HTTP_CODE);
        curl_close($curl);
        if ($raw === false || $status !== 200) throw new RuntimeException('LeadPilot AI delivery service is unavailable; message was not sent');
        $result = json_decode($raw, true);
        if (!is_array($result) || empty($result['success'])) throw new RuntimeException((string) ($result['msg'] ?? 'Unable to prepare delivery'));
        return $result['data'];
    }

    public function compose($args)
    {
        if (($args['name'] ?? '') !== 'composeoptions') return $args;
        $rcmail = rcmail::get_instance();
        $preferences = $rcmail->user->get_prefs();
        $options = array_merge($this->defaults(), (array) ($preferences['bestmaple_delivery_options'] ?? array()));
        $settings = null;
        $settingsError = '';
        try {
            $draft = (string) rcube_utils::get_input_value('_draft_uid', rcube_utils::INPUT_GET);
            $compose = (string) rcube_utils::get_input_value('_id', rcube_utils::INPUT_GET);
            $settings = $this->backend(array('action' => 'config', 'data' => array('draft_key' => $draft !== '' ? 'd:' . $draft : 'c:' . $compose)));
            if (!empty($settings['options'])) $options = array_merge($options, $settings['options']);
        } catch (Throwable $e) { $settingsError = $e->getMessage(); }
        $labels = $this->labels();
        $html = '<fieldset id="maplemail-delivery" class="maplemail-delivery"><legend>' . htmlspecialchars($labels['title'], ENT_QUOTES, 'UTF-8') . '</legend>';
        foreach ($this->defaults() as $key => $unused) {
            $html .= '<label><input type="checkbox" data-maplemail-option="' . $key . '"' . (!empty($options[$key]) ? ' checked' : '') . '> ' . htmlspecialchars($labels[$key], ENT_QUOTES, 'UTF-8') . '</label>';
        }
        $html .= '<p>' . htmlspecialchars($labels['hint'], ENT_QUOTES, 'UTF-8') . '</p>';
        if ($settings) {
            $identity = $settings['identity'];
            $html .= '<p class="lp-effective-identity">' . htmlspecialchars($labels['identity'] . ': ' . $identity['from'] . ' / Reply-To: ' . ($identity['reply_to'] ?: $identity['from']), ENT_QUOTES, 'UTF-8') . '</p>';
        } elseif ($settingsError !== '') {
            $html .= '<p class="warning">' . htmlspecialchars($settingsError, ENT_QUOTES, 'UTF-8') . '</p>';
        }
        $html .= '</fieldset>';
        $args['content'] = ($args['content'] ?? '') . $html;
        $this->include_script('delivery.js');
        $this->include_stylesheet('delivery.css');
        return $args;
    }

    public function prepare($args)
    {
        $rcmail = rcmail::get_instance();
        $preferences = $rcmail->user->get_prefs();
        $options = array_merge($this->defaults(), (array) ($preferences['bestmaple_delivery_options'] ?? array()));
        $interactive = ($rcmail->action ?? '') === 'send';
        if (!$interactive) {
            try {
                $result = $this->backend(array('action' => 'config'));
                $extra = array('From' => $result['identity']['from']);
                if (!empty($result['identity']['reply_to'])) $extra['Reply-To'] = $result['identity']['reply_to'];
                $args['message']->headers($extra, true);
            } catch (Throwable $e) { $args['abort'] = true; $args['result'] = false; $args['error'] = $e->getMessage(); }
            return $args;
        }
        if (!empty($_POST['_maplemail_delivery_present'])) {
            foreach ($this->defaults() as $key => $unused) $options[$key] = ($_POST['_maplemail_' . $key] ?? '') === '1';
        }
        if (!$interactive) $options = array_fill_keys(array_keys($this->defaults()), false);
        try {
            $recipients = array();
            foreach ((array) $args['mailto'] as $line) {
                foreach (rcube_mime::decode_address_list($line, null, false) as $address) {
                    $email = strtolower(trim($address['mailto'] ?? ''));
                    if (filter_var($email, FILTER_VALIDATE_EMAIL)) $recipients[$email] = true;
                }
            }
            $message = $args['message'];
            $headers = $message->headers();
            $plain = $message->getTXTBody();
            $html = $message->getHTMLBody();
            if (!$html) $html = '<div style="white-space:pre-wrap">' . htmlspecialchars((string) $plain, ENT_QUOTES, 'UTF-8') . '</div>';
            $result = $this->backend(array('action' => 'prepare', 'message_id' => $headers['Message-ID'] ?? $headers['Message-Id'] ?? '', 'recipients' => array_keys($recipients), 'html' => $html, 'options' => $options));
            $extra = (array) $result['headers'];
            $extra['From'] = $result['identity']['from'];
            if (!empty($result['identity']['reply_to'])) $extra['Reply-To'] = $result['identity']['reply_to'];
            $extra['Message-ID'] = $result['message_id'];
            $message->headers($extra, true);
            $message->setHTMLBody($result['html']);
            if ($plain !== false && $plain !== null) $message->setTXTBody((string) $plain . $result['plain_append']);
            if ($interactive) $rcmail->user->save_prefs(array('bestmaple_delivery_options' => $options));
        } catch (Throwable $e) {
            $args['abort'] = true;
            $args['result'] = false;
            $args['error'] = $e->getMessage();
        }
        return $args;
    }
}
