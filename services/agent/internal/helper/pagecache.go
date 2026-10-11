package helper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Page cache. WordPress builds every page with PHP and the database on each visit. With the
// cache on, a small must-use plugin saves the finished HTML of a page for anonymous visitors, and
// the web server answers the next visit from that file without starting PHP. The plugin
// deletes the saved pages when content changes. A logged-in visitor, a request with a query
// string, a form post or a shopping cart always goes to WordPress.

const (
	pageCachePlugin = "zenex-page-cache.php"
	pageCacheDir    = "wp-content/cache/zenex"
)

// pageCacheHandler is the Caddy block that serves a saved page. It sits before the handler that
// runs PHP, and only matches what the plugin would have saved: a plain GET with no query
// string, a path ending in a slash, and none of the cookies of a signed-in or shopping visitor.
func pageCacheHandler() string {
	return `@zxcached {
        method GET
        expression {http.request.uri.query} == ''
        not path *..*
        not header_regexp Cookie (wordpress_logged_in|wordpress_sec|comment_author|wp-postpass|woocommerce_items_in_cart|woocommerce_cart_hash)
        file {
            try_files /` + pageCacheDir + `/{host}{path}index.html
        }
    }
    handle @zxcached {
        rewrite * {http.matchers.file.relative}
        header X-Zenex-Cache HIT
        file_server
    }`
}

// pageCachePluginSource is the must-use plugin. It runs as the website's own account.
const pageCachePluginSource = `<?php
/**
 * Plugin Name: Zenex Page Cache
 * Description: Saves finished pages for anonymous visitors. Managed by Zenex; remove it from the panel.
 */
if (!defined('ABSPATH')) {
    exit;
}

function zenex_cache_dir() {
    return WP_CONTENT_DIR . '/cache/zenex';
}

function zenex_cache_flush() {
    static $done = false;
    if ($done) {
        return;
    }
    $done = true;
    $root = zenex_cache_dir();
    if (!is_dir($root)) {
        return;
    }
    $items = new RecursiveIteratorIterator(
        new RecursiveDirectoryIterator($root, FilesystemIterator::SKIP_DOTS),
        RecursiveIteratorIterator::CHILD_FIRST
    );
    foreach ($items as $item) {
        if ($item->isLink() || $item->isFile()) {
            @unlink($item->getPathname());
        } else {
            @rmdir($item->getPathname());
        }
    }
}

function zenex_cache_wanted() {
    if (($_SERVER['REQUEST_METHOD'] ?? '') !== 'GET' || !empty($_SERVER['QUERY_STRING'])) {
        return false;
    }
    if (is_user_logged_in() || is_admin() || is_search() || is_404() || is_feed() || is_preview()
        || is_customize_preview() || is_robots() || post_password_required()) {
        return false;
    }
    foreach (array('DONOTCACHEPAGE', 'DOING_AJAX', 'DOING_CRON', 'REST_REQUEST', 'XMLRPC_REQUEST', 'WP_INSTALLING') as $flag) {
        if (defined($flag) && constant($flag)) {
            return false;
        }
    }
    if ((function_exists('is_cart') && is_cart()) || (function_exists('is_checkout') && is_checkout())
        || (function_exists('is_account_page') && is_account_page())) {
        return false;
    }
    foreach ($_COOKIE as $name => $value) {
        if (preg_match('/^(wordpress_logged_in|wordpress_sec|comment_author|wp-postpass|woocommerce_items_in_cart|woocommerce_cart_hash)/', $name)) {
            return false;
        }
    }
    return true;
}

add_action('template_redirect', function () {
    if (!zenex_cache_wanted()) {
        return;
    }
    $path = parse_url($_SERVER['REQUEST_URI'] ?? '/', PHP_URL_PATH);
    $host = strtolower(preg_replace('/:\d+$/', '', $_SERVER['HTTP_HOST'] ?? ''));
    if (!is_string($path) || substr($path, -1) !== '/' || !preg_match('#^/[A-Za-z0-9/_.~%-]*$#', $path)
        || strpos($path, '..') !== false || !preg_match('/^[a-z0-9.-]+$/', $host)) {
        return;
    }
    $target = zenex_cache_dir() . '/' . $host . $path;
    ob_start(function ($html) use ($target) {
        if (http_response_code() !== 200 || strlen($html) < 256 || stripos($html, '<html') === false) {
            return $html;
        }
        foreach (headers_list() as $header) {
            if (stripos($header, 'Set-Cookie:') === 0) {
                return $html;
            }
        }
        if (!is_dir($target) && !@mkdir($target, 0755, true)) {
            return $html;
        }
        $tmp = $target . 'index.html.' . getmypid() . '.tmp';
        if (@file_put_contents($tmp, $html . "\n<!-- Cached by Zenex -->") !== false) {
            @chmod($tmp, 0644);
            @rename($tmp, $target . 'index.html');
        }
        return $html;
    });
}, 0);

foreach (array(
    'save_post', 'deleted_post', 'trashed_post', 'untrashed_post', 'comment_post',
    'wp_set_comment_status', 'edit_comment', 'switch_theme', 'customize_save_after',
    'activated_plugin', 'deactivated_plugin', 'upgrader_process_complete', 'wp_update_nav_menu',
    'created_term', 'edited_term', 'delete_term', 'update_option_sidebars_widgets',
    'update_option_blogname', 'update_option_permalink_structure', 'woocommerce_update_product',
) as $hook) {
    add_action($hook, 'zenex_cache_flush');
}
`

// pageCacheEnable puts the plugin in the website's must-use folder, owned by the website account.
func (o *Ops) pageCacheEnable(args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(o.docRoot(name), "wp-load.php")); err != nil {
		return errors.New("WordPress is not installed for this website")
	}
	siteUser, err := o.lookup(name)
	if err != nil {
		return fmt.Errorf("site account missing: %w", err)
	}
	uid, _ := strconv.Atoi(siteUser.Uid)
	gid, _ := strconv.Atoi(siteUser.Gid)
	dir := filepath.Join(o.docRoot(name), "wp-content", "mu-plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the must-use folder: %w", err)
	}
	_ = os.Chown(dir, uid, gid)
	path := filepath.Join(dir, pageCachePlugin)
	if err := writeFileAtomic(path, []byte(pageCachePluginSource), 0o644); err != nil {
		return fmt.Errorf("write the page cache plugin: %w", err)
	}
	return os.Chown(path, uid, gid)
}

// pageCacheDisable removes the plugin and every saved page.
func (o *Ops) pageCacheDisable(args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(o.docRoot(name), "wp-content", "mu-plugins", pageCachePlugin)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return o.pageCacheClear(args)
}

// pageCacheClear deletes every saved page of the website.
func (o *Ops) pageCacheClear(args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(o.docRoot(name), filepath.FromSlash(pageCacheDir)))
}
