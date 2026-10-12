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
