// Shapes returned by the Zenex API (apps/api). Keep in sync with the Go structs.

export interface User {
  id: string;
  email: string;
  roles: string[];
}

export interface Metrics {
  cpu_count: number;
  load_1m: number;
  load_5m: number;
  load_15m: number;
  mem_total_bytes: number;
  mem_used_bytes: number;
  disk_total_bytes: number;
  disk_used_bytes: number;
  uptime_seconds: number;
}

export interface Domain {
  id: string;
  apex: string;
  verified: boolean;
  message: string;
  checked_at?: string;
  created_at: string;
}

export type SiteState = "provisioning" | "ready" | "suspended" | "failed" | "deleting" | "deleted";

export interface Site {
  id: string;
  owner_user_id: string;
  node_id: string;
  slug: string;
  domain: string;
  php_version: string;
  state: SiteState;
  health: string;
  auto_update: boolean;
  maintenance: boolean;
  created_at: string;
}

export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled" | "dead";

export interface Job {
  id: string;
  type: string;
  status: JobStatus;
  site_id?: string;
  attempts: number;
  error?: string;
}

export type StepStatus = "pending" | "running" | "succeeded" | "failed" | "skipped";

export interface JobStep {
  name: string;
  status: StepStatus;
  attempts: number;
  error?: string;
}

export interface JobDetail {
  job: Job;
  steps: JobStep[];
}

export interface SiteCredentials {
  url: string;
  username: string;
  password: string;
}

export interface ApiErrorBody {
  error: { code: string; message: string; request_id: string };
}

// ---------------------------------------------------------------------------
// Monitoring and activity
// ---------------------------------------------------------------------------

export type TimeRange = "1h" | "6h" | "24h" | "7d";

export interface MetricPoint {
  t: string;
  load: number;
  cpus: number;
  mem_pct: number;
  disk_pct: number;
}

export interface MetricSeries {
  points: MetricPoint[];
  bucket_seconds: number;
}

export interface SiteHealth {
  site_id: string;
  checks: number;
  ok_checks: number;
  avg_latency_ms: number;
  last_check?: string;
  last_ok: boolean;
  last_status: number;
}

export interface ServiceState {
  name: string;
  state: string;
}

export type ActivityResult = "success" | "failure" | "denied";

export interface ActivityRow {
  id: number;
  time: string;
  action: string;
  target_type?: string;
  target_id?: string;
  result: ActivityResult;
  error_code?: string;
  ip?: string;
  actor_role?: string;
}

export interface ActivityPage {
  items: ActivityRow[];
  next_before?: number;
}

export interface JobLogLine {
  id: number;
  time: string;
  level: "info" | "warn" | "error";
  message: string;
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

export type NotificationLevel = "info" | "success" | "warning" | "error";

export interface Notification {
  id: number;
  level: NotificationLevel;
  title: string;
  body: string;
  created_at: string;
  read_at?: string;
}

export interface NotificationsPage {
  items: Notification[];
  unread: number;
}

// ---------------------------------------------------------------------------
// Website file manager
// ---------------------------------------------------------------------------

export interface FileEntry {
  name: string;
  type: "dir" | "file";
  size: number;
  modified: string;
  editable: boolean;
}

export interface FileContent {
  path: string;
  content: string;
}

// ---------------------------------------------------------------------------
// Branding
// ---------------------------------------------------------------------------

export interface Branding {
  name: string;
  tagline: string;
  primary_color: string;
  has_logo: boolean;
  has_favicon: boolean;
}

/** The fields an administrator edits. The has_* flags are read-only. */
export type BrandingInput = Pick<Branding, "name" | "tagline" | "primary_color">;

/** A logo or favicon as the API expects it: base64 data with its MIME type. */
export interface BrandImageInput {
  mime: string;
  data: string;
}

// ---------------------------------------------------------------------------
// Settings (administrators)
// ---------------------------------------------------------------------------

export interface SiteDefaults {
  php_version: string;
}

export type SftpAuth = "key" | "password";

export interface SftpSettings {
  host: string;
  port: number;
  username: string;
  path: string;
  auth: SftpAuth;
  /** True when a password is saved on the server. The saved password is never returned. */
  password_set: boolean;
  /** Only sent on save, and only when a new password was typed. Empty means keep the saved one. */
  password?: string;
}

export interface BackupDestination {
  type: "local" | "sftp";
  sftp: SftpSettings;
}

export type BackupFrequency = "hourly" | "daily" | "weekly";

export interface BackupSettings {
  frequency: BackupFrequency;
  /** 0 to 23. Ignored for hourly backups. */
  schedule_hour: number;
  /** 0 to 6, Sunday = 0. Only used for weekly backups. */
  weekday: number;
  retention_days: number;
  destination: BackupDestination;
}

export interface EmailAlertSettings {
  enabled: boolean;
  host: string;
  port: number;
  username: string;
  from: string;
  to: string;
}

export interface TelegramAlertSettings {
  enabled: boolean;
  chat_id: string;
}

export interface AlertThresholds {
  cpu: number;
  memory: number;
  disk: number;
}

/** What the API returns. Secrets are never sent back, only whether they are stored. */
export interface AlertSettings {
  email: EmailAlertSettings & { password_set: boolean };
  telegram: TelegramAlertSettings & { token_set: boolean };
  thresholds: AlertThresholds;
}

/** What the API accepts. An empty secret keeps the stored one. */
export interface AlertSettingsInput {
  email: EmailAlertSettings & { password: string };
  telegram: TelegramAlertSettings & { bot_token: string };
  thresholds: AlertThresholds;
}

// ---------------------------------------------------------------------------
// Website backups
// ---------------------------------------------------------------------------

export interface SiteBackup {
  id: number;
  created_at: string;
  size_bytes: number;
}

export type BackupStepName = "archive" | "upload" | "record";
export type BackupStepStatus = "pending" | "running" | "succeeded" | "failed";

export interface BackupStep {
  name: BackupStepName;
  status: BackupStepStatus;
}

/** What a website is doing now, or just finished doing (a build, backup, restore, migration or deletion). */
export interface SiteActivity {
  site_id: string;
  job_id: string;
  type: string;
  status: "queued" | "running" | "succeeded" | "failed" | "cancelled" | "dead";
  /** 0 to 100. */
  percent: number;
  /** The step that is running, or the next one. Empty when finished. */
  step: string;
}

// ---------------------------------------------------------------------------
// Migration from cPanel
// ---------------------------------------------------------------------------

/** The SSH sign-in to a cPanel account. Sent with each request and never stored. */
export interface CpanelConn {
  host: string;
  port: number;
  username: string;
  password: string;
}

/** One WordPress website found on the cPanel account. */
export interface CpanelInstall {
  /** The website folder on the cPanel account. */
  path: string;
  site_url: string;
  /** The website's host name, or "" when it could not be read. */
  domain: string;
  db_name: string;
  table_prefix: string;
  size_kb: number;
  /** The connected domain that covers `domain`, or "". */
  matched_domain: string;
  /** The website here that already uses `domain`, or "". */
  existing_site_id: string;
}

export interface CpanelScanResult {
  installs: CpanelInstall[];
  server_ip: string;
}

export interface CpanelMigrateResult {
  site: Site;
  job_id: string;
  provision_job_id: string;
  server_ip: string;
}

/** A backup on the SFTP server. `path` is the remote file path used to restore it. */
export interface RemoteBackup {
  path: string;
  size_bytes: number;
  domain: string;
  site_id: string;
  created_at: string;
}

/** Progress of the latest backup job. `job_id` is "" when the website has never been backed up. */
export interface BackupJobProgress {
  job_id: string;
  status?: "queued" | "running" | "succeeded" | "failed";
  percent?: number;
  /** The "upload" step is absent for local storage. */
  steps?: BackupStep[];
}
