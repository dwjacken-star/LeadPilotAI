<?php
/** Server-side localization also covers failed login and uncached first visits. */
trait LeadPilotLoginLocalization
{
    private $login_locale_code = 'zh';

    private function init_login_localization()
    {
        $rcmail = rcmail::get_instance();
        if ($rcmail->task !== 'login' || ($rcmail->user && $rcmail->user->ID)) return;
        $locales = array('en'=>'en_US','zh'=>'zh_CN','ja'=>'ja_JP','de'=>'de_DE','tr'=>'tr_TR','vi'=>'vi_VN');
        $requested = (string) rcube_utils::get_input_value('_lp_lang', rcube_utils::INPUT_GET);
        $posted = (string) rcube_utils::get_input_value('_language', rcube_utils::INPUT_POST);
        $code = $requested ?: (array_search($posted, $locales, true) ?: ($_COOKIE['leadpilot_login_language'] ?? substr((string) ($_SESSION['language'] ?? 'zh'),0,2)));
        if (!isset($locales[$code])) $code = 'zh';
        $this->login_locale_code = $code;
        $rcmail->config->set('language', $locales[$code]);
        $rcmail->load_language($locales[$code]);
        setcookie('leadpilot_login_language', $code, array('expires'=>time()+31536000,'path'=>'/roundcube/','secure'=>true,'httponly'=>true,'samesite'=>'Lax'));
        $this->add_hook('render_page', array($this,'localize_login_page'));
    }

    public function localize_login_page($args)
    {
        if (($args['template'] ?? '') !== 'login') return $args;
        $packs = json_decode(file_get_contents(__DIR__ . '/../../skins/bestmaple/login-labels.json'),true);
        $code = $this->login_locale_code;
        $text = $packs[$code] ?? $packs['zh'];
        $html = preg_replace_callback('~<([a-z][a-z0-9]*)([^>]*\bdata-bm=["\x27]([^"\x27]+)["\x27][^>]*)>[\s\S]*?</\1>~i',function($m)use($text){
            return isset($text[$m[3]]) ? '<'.$m[1].$m[2].'>'.htmlspecialchars($text[$m[3]],ENT_QUOTES,'UTF-8').'</'.$m[1].'>' : $m[0];
        },$args['content']);
        $html = preg_replace_callback('~<option\s+value="(en|zh|ja|de|tr|vi)"[^>]*>~',function($m)use($code){return '<option value="'.$m[1].'"'.($m[1]===$code?' selected':'').'>';},$html);
        $html = preg_replace_callback('~<html\b([^>]*)>~i',function($m)use($code){
            $attrs=preg_replace('~\s+(?:lang|data-leadpilot-language)="[^"]*"~i','',$m[1]);
            return '<html'.$attrs.' lang="'.$code.'" data-leadpilot-language="'.$code.'">';
        },$html,1);
        if (!preg_match('~<input\b[^>]*name="_language"~i',$html)) {
            $locales=array('en'=>'en_US','zh'=>'zh_CN','ja'=>'ja_JP','de'=>'de_DE','tr'=>'tr_TR','vi'=>'vi_VN');
            $html=preg_replace('~(<form\b[^>]*id="login-form"[^>]*>)~i','$1<input type="hidden" name="_language" value="'.$locales[$code].'">',$html,1);
        }
        foreach(array('rcmloginuser'=>'user','rcmloginpwd'=>'password') as $id=>$key){
            $html = preg_replace('~(<label\b[^>]*for="'.$id.'"[^>]*>)[\s\S]*?(</label>)~','$1'.htmlspecialchars($text[$key],ENT_QUOTES,'UTF-8').'$2',$html);
        }
        $html = preg_replace('~(<button\b[^>]*id="rcmloginsubmit"[^>]*>)[\s\S]*?(</button>)~','$1'.htmlspecialchars($text['submit'],ENT_QUOTES,'UTF-8').'$2',$html);
        $args['content']=$html;
        return $args;
    }
}
