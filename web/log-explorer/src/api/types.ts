export interface BackendOperationMix {
  operation: string;
  weight: number;
  uri?: string;
  prompt_name?: string;
  arguments?: Record<string, unknown>;
}

export interface BackendToolTemplate {
  template_id: string;
  tool_name: string;
  weight: number;
  arguments: Record<string, unknown>;
  expects_streaming?: boolean;
}

export interface BackendStopCondition {
  id: string;
  metric: string;
  comparator: string;
  threshold: number;
  window_ms: number;
  sustain_windows: number;
  scope: Record<string, string>;
}

export interface BackendRunConfig {
  schema_version: string;
  scenario_id: string;
  metadata?: {
    name: string;
    description?: string;
    created_by: string;
    tags: Record<string, string>;
  };
  target: {
    kind: string;
    url: string;
    transport: 'streamable_http';
    headers: Record<string, string>;
    auth: {
      type: string;
      tokens?: string[];
    };
    identification: {
      run_id_header: {
        name: string;
        value_template: string;
      };
      user_agent: {
        value: string;
      };
    };
    timeouts: {
      connect_timeout_ms: number;
      request_timeout_ms: number;
      stream_stall_timeout_ms: number;
    };
    tls: {
      verify: boolean;
      ca_bundle_ref: string | null;
    };
    redirect_policy: {
      mode: string;
      max_redirects: number;
    };
  };
  environment: {
    allowlist: {
      mode: string;
      allowed_targets: Array<{ kind: string; value: string }>;
    };
    forbidden_patterns: string[];
  };
  session_policy: {
    mode: string;
    pool_size: number | null;
    ttl_ms: number | null;
    max_idle_ms: number | null;
  };
  workload: {
    in_flight_per_vu: number;
    think_time: {
      mode: string;
      base_ms: number;
      jitter_ms: number;
    };
    operation_mix: BackendOperationMix[];
    tools: {
      selection: {
        mode: string;
        single_template_id?: string | null;
      };
      templates: BackendToolTemplate[];
    };
    payload_profiles: unknown[];
  };
  stages: Array<{
    stage_id: string;
    stage: string;
    enabled: boolean;
    duration_ms: number;
    load: {
      target_vus: number;
      target_rps: number | null;
    };
    stop_conditions: BackendStopCondition[];
  }>;
  safety: {
    ramp_by_default: boolean;
    emergency_stop_enabled: boolean;
    worker_failure_policy: string;
    hard_caps: {
      max_vus: number;
      max_rps: number;
      max_connections: number;
      max_duration_ms: number;
      max_in_flight_per_vu: number;
      max_telemetry_q_depth: number;
    };
    stop_policy: {
      mode: string;
      drain_timeout_ms: number;
    };
    identification_required: boolean;
  };
  reporting: {
    formats: string[];
    retention: {
      raw_logs_days: number;
      metrics_days: number;
      reports_days: number;
    };
    include: {
      store_raw_logs: boolean;
      store_metrics_snapshot: boolean;
      store_event_log: boolean;
    };
    redaction: {
      redact_headers: string[];
    };
  };
  telemetry: {
    structured_logs: {
      enabled: boolean;
      sample_rate: number;
    };
    traces: {
      enabled: boolean;
      propagation: {
        accept_incoming_traceparent: boolean;
      };
    };
  };
  server_telemetry?: {
    enabled: boolean;
    pair_key: string;
  };
}

// Legacy error body format (kept for backward compatibility)
export interface ApiErrorBody {
  error?: string;
  error_message?: string;
  message?: string;
  detail?: string;
}

// Structured error response (matches backend api.ErrorResponse)
export interface ErrorResponse {
  error_type: string;
  error_code: string;
  error_message: string;
  retryable: boolean;
  details?: Record<string, unknown>;
}

export interface AgentInfo {
  agent_id: string;
  pair_key: string;
  hostname: string;
  os: string;
  arch: string;
  version: string;
  online: boolean;
  registered_at: string;
  last_seen: string;
}

export interface DiscoveredTool {
  name: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
  annotations?: {
    readOnlyHint?: boolean;
    destructiveHint?: boolean;
    idempotentHint?: boolean;
    openWorldHint?: boolean;
  };
}

export interface ConnectionTestResult {
  success: boolean;
  message?: string;
  tool_count?: number;
  tools?: DiscoveredTool[];
  connect_latency_ms?: number;
  tools_latency_ms?: number;
  total_latency_ms?: number;
  error?: string;
  error_code?: string;
}

export interface ToolTestResult {
  success: boolean;
  result?: unknown;
  error?: string;
  latency_ms: number;
}

// Stop modes for run termination
export type StopMode = 'drain' | 'immediate';

export interface StopRunResponse {
  run_id: string;
  state: string;
}

// Validation types (matches backend ValidationIssue)
export interface ValidationIssue {
  level: 'error' | 'warning';
  code: string;
  message: string;
  json_pointer?: string;
  remediation?: string;
}

export type ValidationError = ValidationIssue;
export type ValidationWarning = ValidationIssue;

export interface ValidationResult {
  ok: boolean;
  errors: ValidationError[];
  warnings: ValidationWarning[];
}

// Agent detail types
export interface AgentDetail extends AgentInfo {
  tags?: Record<string, string>;
  process_info?: {
    pid?: number;
    listen_port?: number;
    process_regex?: string;
  };
  metrics_summary?: {
    total_samples: number;
    cpu_avg: number;
    cpu_max: number;
    mem_avg: number;
    mem_max: number;
  };
}

// SSE Event types (matches backend runmanager.RunEvent)
export interface RunEventCorrelation {
  stage?: string;
  stage_id?: string;
  worker_id?: string;
  vu_id?: string;
  session_id?: string;
}

export interface RunEvent {
  schema_version?: string;
  event_id: string;
  ts_ms?: number;
  run_id?: string;
  execution_id?: string;
  type: string;
  actor?: string;
  correlation?: RunEventCorrelation;
  payload?: Record<string, unknown>;
  evidence?: Array<{ kind: string; ref: string; note?: string }>;
  timestamp?: number;
  data: Record<string, unknown>;
}

export type RunEventHandler = (event: RunEvent) => void;
export type SSEErrorHandler = (error: Event) => void;

// Error signatures types (for api.ts consolidation)
export interface ErrorSignature {
  pattern: string;
  count: number;
  first_seen_ms: number;
  last_seen_ms: number;
  affected_operations: string[];
  affected_tools: string[];
  sample_error: string;
}

export interface ErrorSignaturesResponse {
  run_id: string;
  signatures: ErrorSignature[];
}
