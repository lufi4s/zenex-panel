package helper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// autoLoginLoginRe is the WordPress user name the sign-in link may log in as.
var autoLoginLoginRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,60}$`)

// autoLoginPlugin is a must-use plugin that signs the administrator in when a valid,
// unused, short-lived link is opened on the website itself. The link is made by the panel
// with the same key: "<expiry>.<nonce>.<hmac>". Browsers keep the login only when the
// sign-in starts on the website, which is why it is a link and not a form posted from the panel.
const autoLoginPlugin = `<?php
/**
 * Zenex one-time sign-in for the panel's "Open admin" button. Managed by Zenex: do not edit.
 */
if (!defined('ABSPATH')) {
    exit;
}

add_action('init', function () {
    if (!isset($_GET['zenex_autologin'])) {
        return;
    }
    $key = '__KEY__';
    $login = '__LOGIN__';
    $fail = function ($message) {
        wp_die(esc_html($message), 'Sign-in link', array('response' => 403));
    };

    $parts = explode('.', (string) wp_unslash($_GET['zenex_autologin']));
    if (count($parts) !== 3) {
        $fail('This sign-in link is not valid.');
    }
    list($expiry, $nonce, $signature) = $parts;
    if (!ctype_digit($expiry) || (int) $expiry < time() || (int) $expiry > time() + 120) {
        $fail('This sign-in link has expired. Open admin again from the panel.');
    }
    if (!preg_match('/^[a-f0-9]{16,64}$/', $nonce)) {
        $fail('This sign-in link is not valid.');
    }
    $expected = hash_hmac('sha256', $expiry . '.' . $nonce, $key);
    if (!hash_equals($expected, $signature)) {
        $fail('This sign-in link is not valid.');
    }

    // A link works once: the nonce is remembered until the link would have expired anyway.
    $seen = 'zenex_autologin_' . md5($nonce);
    if (get_transient($seen)) {
        $fail('This sign-in link has already been used. Open admin again from the panel.');
    }
    set_transient($seen, 1, 300);

    $user = get_user_by('login', $login);
    if (!$user || !user_can($user, 'manage_options')) {
        $fail('The administrator account for this website was not found.');
    }
    wp_set_current_user($user->ID);
    wp_set_auth_cookie($user->ID, false, is_ssl());
    wp_safe_redirect(admin_url());
    exit;
}, 0);
`

// wpAutoLogin installs or refreshes the sign-in plugin of a website. The file holds the key,
// so it is owned by the site account and readable by that account only.
func (o *Ops) wpAutoLogin(ctx context.Context, args map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	key := args["key"]
	if err := requireHex(key, "sign-in key"); err != nil {
		return err
	}
	login := args["login"]
	if !autoLoginLoginRe.MatchString(login) {
		return errors.New("invalid administrator name")
	}
	docroot := o.docRoot(name)
	if _, err := os.Stat(filepath.Join(docroot, "wp-config.php")); err != nil {
		return errors.New("WordPress is not installed on this website")
	}

	dir := filepath.Join(docroot, "wp-content", "mu-plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.New("the sign-in plugin folder could not be created")
	}
	target := filepath.Join(dir, "zenex-autologin.php")
	code := strings.NewReplacer("__KEY__", key, "__LOGIN__", login).Replace(autoLoginPlugin)
	if err := writeFileAtomic(target, []byte(code), 0o600); err != nil {
		return errors.New("the sign-in plugin could not be written")
	}
	return o.ownBySite(name, target)
}
