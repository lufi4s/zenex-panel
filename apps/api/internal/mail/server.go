package mail

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// This file holds the text of the mail server configuration (Postfix, Dovecot, rspamd). It
// only builds strings: nothing is written to disk here. The helper writes the files and runs
// the checks (postfix check, doveconf -n, rspamadm configtest) before any service reloads.
// The mail server reads mailbox data from the panel's PostgreSQL database through a read-only
// role; passwords stay hashed.

// hostnameRe is a fully qualified host name such as mail.example.com.
var hostnameRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// ValidHostname reports whether s is a fully qualified host name (the mail server's name).
func ValidHostname(s string) bool { return hostnameRe.MatchString(s) }

// Paths of the generated files on the server.
const (
	PostfixMain        = "/etc/postfix/main.cf"
	PostfixMaster      = "/etc/postfix/master.cf"
	PostfixDomains     = "/etc/postfix/zenex-domains.cf"
	PostfixMailboxes   = "/etc/postfix/zenex-mailboxes.cf"
	DovecotSQL         = "/etc/dovecot/conf.d/zenex-sql.conf.ext"
	DovecotLocal       = "/etc/dovecot/conf.d/99-zenex.conf"
	RspamdDKIM         = "/etc/rspamd/local.d/dkim_signing.conf"
	MailTLSCertificate = "/etc/zenex/mail/tls/fullchain.pem"
	MailTLSKey         = "/etc/zenex/mail/tls/privkey.pem"
)

// Settings are the values the configuration needs.
type Settings struct {
	Host   string // mail server name, for example mail.example.com
	DBHost string // PostgreSQL host, normally 127.0.0.1
	DBName string // panel database name
	DBUser string // read-only mail role
	DBPass string // its password; written only into the root-readable lookup files
	MaxMB  int    // largest accepted message, in megabytes
}

// Validate checks everything Render needs.
func (s Settings) Validate() error {
	if !ValidHostname(s.Host) {
		return errors.New("the mail host must be a full host name such as mail.example.com")
	}
	if s.DBHost == "" || s.DBName == "" || s.DBUser == "" || s.DBPass == "" {
		return errors.New("the mail database settings are incomplete")
	}
	if strings.ContainsAny(s.DBPass, " \t\r\n") {
		return errors.New("the mail database password cannot contain spaces or line breaks")
	}
	if s.MaxMB < 1 || s.MaxMB > 100 {
		return errors.New("the largest message size must be between 1 and 100 MB")
	}
	return nil
}

// File is one generated configuration file.
type File struct {
	Path    string
	Content string
	Mode    uint32 // octal permission bits, for example 0o644
}

// Render returns every file the mail server needs. Files holding the database password are
// marked 0640 (root and the service group only).
func Render(s Settings) ([]File, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// Postfix and Dovecot spell the connection differently: Postfix uses "hosts=", Dovecot "host=".
	postfixDSN := fmt.Sprintf("hosts=%s dbname=%s user=%s password=%s", s.DBHost, s.DBName, s.DBUser, s.DBPass)
	dovecotDSN := fmt.Sprintf("host=%s dbname=%s user=%s password=%s", s.DBHost, s.DBName, s.DBUser, s.DBPass)
	size := s.MaxMB * 1024 * 1024
	return []File{
		{Path: PostfixMain, Mode: 0o644, Content: postfixMain(s, size)},
		{Path: PostfixMaster, Mode: 0o644, Content: postfixMaster()},
		{Path: PostfixDomains, Mode: 0o640, Content: pgsqlMap(postfixDSN,
			"SELECT 'ok' FROM mail_domains md JOIN dns_zones z ON z.id = md.zone_id "+
				"WHERE md.enabled AND z.apex_domain = '%s'")},
		{Path: PostfixMailboxes, Mode: 0o640, Content: pgsqlMap(postfixDSN,
			"SELECT 'ok' FROM mailboxes m JOIN dns_zones z ON z.id = m.zone_id "+
				"JOIN mail_domains md ON md.zone_id = z.id "+
				"WHERE md.enabled AND m.active AND z.apex_domain = '%d' AND m.local_part = '%u'")},
		{Path: DovecotSQL, Mode: 0o640, Content: dovecotSQL(dovecotDSN)},
		{Path: DovecotLocal, Mode: 0o644, Content: dovecotLocal(s)},
		{Path: RspamdDKIM, Mode: 0o644, Content: rspamdDKIM()},
	}, nil
}

func postfixMain(s Settings, sizeBytes int) string {
	return fmt.Sprintf(`# Managed by Zenex. Do not edit by hand.
smtpd_banner = $myhostname ESMTP
biff = no
append_dot_mydomain = no
readme_directory = no
compatibility_level = 3.6

myhostname = %[1]s
myorigin = $myhostname
mydestination = localhost
mynetworks = 127.0.0.0/8 [::1]/128
inet_interfaces = all
inet_protocols = all

# Mail for hosted domains is looked up in the panel database (see zenex-*.cf).
virtual_mailbox_domains = pgsql:%[2]s
virtual_mailbox_maps = pgsql:%[3]s
virtual_transport = lmtp:unix:private/dovecot-lmtp

# Only signed-in mailboxes may send through this server. Strangers cannot relay.
smtpd_sasl_type = dovecot
smtpd_sasl_path = private/auth
smtpd_sasl_auth_enable = yes
smtpd_sasl_security_options = noanonymous
smtpd_relay_restrictions = permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination
smtpd_recipient_restrictions = permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination
smtpd_helo_required = yes
smtpd_client_connection_count_limit = 20
smtpd_client_message_rate_limit = 300
anvil_rate_time_unit = 60s

# Transport security. Receiving servers may connect without TLS; clients must use it to log in.
smtpd_tls_cert_file = %[4]s
smtpd_tls_key_file = %[5]s
smtpd_tls_security_level = may
smtpd_tls_auth_only = yes
smtpd_tls_protocols = !SSLv2, !SSLv3, !TLSv1, !TLSv1.1
smtp_tls_security_level = may
smtp_tls_protocols = !SSLv2, !SSLv3, !TLSv1, !TLSv1.1

message_size_limit = %[6]d
mailbox_size_limit = 0
`, s.Host, PostfixDomains, PostfixMailboxes, MailTLSCertificate, MailTLSKey, sizeBytes)
}

func postfixMaster() string {
	return `# Managed by Zenex. Do not edit by hand.
# Submission on 587: clients sign in with their mailbox and must use TLS.
submission inet n - y - - smtpd
  -o syslog_name=postfix/submission
  -o smtpd_tls_security_level=encrypt
  -o smtpd_sasl_auth_enable=yes
  -o smtpd_tls_auth_only=yes
  -o smtpd_client_restrictions=permit_sasl_authenticated,reject
`
}

// pgsqlMap builds a Postfix PostgreSQL lookup file. Postfix substitutes %s, %d and %u.
func pgsqlMap(dsn, query string) string {
	return fmt.Sprintf("# Managed by Zenex. Do not edit by hand.\n"+
		"# Postfix lookup against the panel database (read-only role).\n"+
		"hosts = %s\n", pgsqlHosts(dsn)) + pgsqlRest(dsn) + "query = " + query + "\n"
}

// pgsqlHosts, pgsqlRest split the DSN into Postfix lookup keys. The DSN is built by Render from
// validated values, so it has no quotes or line breaks.
func pgsqlHosts(dsn string) string { return fieldValue(dsn, "hosts") }

func pgsqlRest(dsn string) string {
	return "dbname = " + fieldValue(dsn, "dbname") + "\n" +
		"user = " + fieldValue(dsn, "user") + "\n" +
		"password = " + fieldValue(dsn, "password") + "\n"
}

// fieldValue returns the value of key in a space-separated key=value string.
func fieldValue(s, key string) string {
	for _, part := range strings.Fields(s) {
		if k, v, ok := strings.Cut(part, "="); ok && k == key {
			return v
		}
	}
	return ""
}

func dovecotSQL(dsn string) string {
	return fmt.Sprintf(`# Managed by Zenex. Do not edit by hand.
driver = pgsql
connect = %[1]s
default_pass_scheme = ARGON2ID
password_query = SELECT '{ARGON2ID}' || m.password_hash AS password FROM mailboxes m JOIN dns_zones z ON z.id = m.zone_id JOIN mail_domains md ON md.zone_id = z.id WHERE md.enabled AND m.active AND z.apex_domain = '%%d' AND m.local_part = '%%n'
user_query = SELECT 'vmail' AS uid, 'vmail' AS gid, '/var/mail/vhosts/%%d/%%n' AS home, 'maildir:/var/mail/vhosts/%%d/%%n' AS mail FROM mailboxes m JOIN dns_zones z ON z.id = m.zone_id WHERE m.active AND z.apex_domain = '%%d' AND m.local_part = '%%n'
`, dsn)
}

func dovecotLocal(s Settings) string {
	return fmt.Sprintf(`# Managed by Zenex. Do not edit by hand.
protocols = imap lmtp
mail_location = maildir:/var/mail/vhosts/%%d/%%n
mail_uid = vmail
mail_gid = vmail
first_valid_uid = 1000

ssl = required
ssl_server_cert_file = %[1]s
ssl_server_key_file = %[2]s
ssl_min_protocol = TLSv1.2

auth_mechanisms = plain login
passdb {
  driver = sql
  args = %[3]s
}
userdb {
  driver = sql
  args = %[3]s
}

service auth {
  unix_listener /var/spool/postfix/private/auth {
    mode = 0660
    user = postfix
    group = postfix
  }
}

service lmtp {
  unix_listener /var/spool/postfix/private/dovecot-lmtp {
    mode = 0600
    user = postfix
    group = postfix
  }
}
`, MailTLSCertificate, MailTLSKey, DovecotSQL)
}

func rspamdDKIM() string {
	return `# Managed by Zenex. Do not edit by hand.
# Signs outgoing mail with the domain's key. Keys live in /var/lib/zenex/dkim/<domain>/.
enabled = true;
sign_authenticated = true;
sign_local = true;
use_domain = "header";
selector = "default";
path = "/var/lib/zenex/dkim/$domain/$selector.key";
`
}
