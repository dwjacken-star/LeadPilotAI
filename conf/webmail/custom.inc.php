<?php
    # Product name
    $config['product_name'] = 'LeadPilot AI';
    $config['skin'] = 'bestmaple';
    $config['skins_allowed'] = array('bestmaple');
    $config['display_product_info'] = 0;

    $config['mime_types'] = '/var/roundcube/config/mime.types';
       
    # Plugins
    $config['plugins'] = array('password','userinfo','newmail_notifier','emoticons','zipdownload','bestmaple_tracking');
    $config['bestmaple_tracking_base_url'] = getenv('BESTMAPLE_TRACKING_BASE_URL') ?: '';
    $config['htmleditor'] = 1;

    # Password
    // Keep the SMTP submission credential in sync when a mailbox owner changes it.
    // %p is bound as a parameter by Roundcube's SQL password driver.
    $config['password_query'] = "UPDATE mailbox SET password = %P, password_encode = encode(convert_to(replace(encode(convert_to(%p, 'UTF8'), 'base64'), chr(10), ''), 'UTF8'), 'hex'), update_time = EXTRACT(EPOCH FROM NOW())::int WHERE username = %u AND active = 1";
    $config['password_algorithm'] = 'md5-crypt';
    $config['password_confirm_current'] = true;
    $config['password_minimum_length'] = 8;


# Please create an extra file: extra.php. Used to persist overrides Configuration

$config['skin_logo'] = 'skins/bestmaple/assets/leadpilot-wordmark.png';
