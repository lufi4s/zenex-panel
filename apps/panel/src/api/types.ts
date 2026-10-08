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
