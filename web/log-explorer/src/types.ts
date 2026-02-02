export interface OperationLog {
  timestamp_ms: number;
  run_id: string;
  execution_id?: string;
  stage: string;
  stage_id: string;
  worker_id: string;
  vu_id: string;
  session_id: string;
  operation: string;
  tool_name: string;
  latency_ms: number;
  ok: boolean;
  error_type: string;
  error_code: string;
  stream?: StreamInfo;
  token_index?: number;
}

export interface LogQueryResponse {
  run_id: string;
  total: number;
  offset: number;
  limit: number;
  logs: OperationLog[];
  logs_truncated?: boolean;
}

export interface StopReason {
  mode: string;
  reason: string;
  actor: string;
  at_ms: number;
}

export interface RunInfo {
  id: string;
  scenario_id: string;
  state: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  stop_reason?: StopReason;
}

export interface LogFilters {
  stage: string;
  stage_id: string;
  worker_id: string;
  session_id: string;
  vu_id: string;
  operation: string;
  tool_name: string;
  error_type: string;
  error_code: string;
  token_index?: number;
}

export interface PaginationState {
  offset: number;
  limit: number;
  total: number;
}

// Run Configuration Types
export type AuthType = 'none' | 'bearer_token';

export interface AuthConfig {
  type: AuthType;
  tokens?: string[];
  activeTokenIndex?: number;
}

export interface TargetConfig {
  kind: 'server' | 'gateway';
  url: string;
  transport: 'streamable_http' | 'stdio';
  headers?: Record<string, string>;
  auth?: AuthConfig;
}

export interface StopCondition {
  id?: string;
  metric: 'error_rate' | 'latency_p99_ms' | 'latency_p95_ms' | 'latency_p50_ms';
  comparator?: '>' | '>=' | '<' | '<=';
  threshold: number;
  window_ms: number;
  sustain_windows?: number;
}

export interface StreamingStopConditions {
  stream_stall_seconds?: number;
  min_events_per_second?: number;
}

export interface StageLoad {
  target_vus: number;
}

export interface StageConfig {
  stage_id: string;
  stage: 'preflight' | 'baseline' | 'ramp';
  enabled: boolean;
  duration_ms: number;
  load: StageLoad;
  stop_conditions?: StopCondition[];
  streaming_stop_conditions?: StreamingStopConditions;
}

export interface OpMixEntry {
  operation: 'tools/list' | 'tools/call' | 'ping' | 'resources/list' | 'resources/read' | 'prompts/list' | 'prompts/get';
  weight: number;
  tool_name?: string;
  arguments?: Record<string, unknown>;
  uri?: string;
  prompt_name?: string;
}

export interface WorkloadConfig {
  op_mix: OpMixEntry[];
}

export interface SessionPolicy {
  mode: 'reuse' | 'per_request' | 'pool' | 'churn';
  pool_size?: number;
  ttl_ms?: number;
  max_idle_ms?: number;
}

export interface HardCaps {
  max_vus?: number;
  max_duration_ms?: number;
}

export interface SafetyConfig {
  hard_caps?: HardCaps;
}

export interface AllowlistConfig {
  mode: 'deny_by_default' | 'allow_all';
  allowed_hosts?: string[];
}

export interface EnvironmentConfig {
  allowlist?: AllowlistConfig;
}

export interface ServerTelemetryConfig {
  enabled: boolean;
  pair_key: string;
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

export interface RunConfig {
   scenario_id: string;
   target: TargetConfig;
   stages: StageConfig[];
   workload: WorkloadConfig;
   session_policy?: SessionPolicy;
   safety?: SafetyConfig;
   environment?: EnvironmentConfig;
   server_telemetry?: ServerTelemetryConfig;
   schema_version?: string;
 }

export type WizardStep = 'target' | 'stages' | 'workload' | 'review';

// Comparison Types
export interface RunMetrics {
  throughput: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  latency_p99_ms: number;
  error_rate: number;
  total_ops: number;
  failed_ops: number;
  duration_ms: number;
}

export interface RunMetricsResponse {
  run_id: string;
  throughput: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  latency_p99_ms: number;
  error_rate: number;
  total_ops: number;
  failed_ops: number;
  duration_ms: number;
}

// Raw API response from /runs/compare
export interface ComparisonApiResponse {
  run_a: RunMetricsResponse;
  run_b: RunMetricsResponse;
}

// Computed diff between two runs
export interface ComparisonDiff {
  latency_p50_ms: number;
  latency_p50_pct: number;
  latency_p95_ms: number;
  latency_p95_pct: number;
  latency_p99_ms: number;
  latency_p99_pct: number;
  throughput: number;
  throughput_pct: number;
  error_rate: number;
  error_rate_pct: number;
}

// Full comparison result with computed diff (used in components)
export interface ComparisonResult {
  run_a: RunMetricsResponse;
  run_b: RunMetricsResponse;
  diff: ComparisonDiff;
}

export type MetricDirection = 'higher_better' | 'lower_better';

export type MetricKey = keyof Omit<RunMetricsResponse, 'run_id'>;

export interface MetricConfig {
  key: MetricKey;
  label: string;
  unit: string;
  direction: MetricDirection;
  format: (value: number) => string;
}

// Real-time Metrics Types
export interface MetricsTimePoint {
  timestamp: number;
  success_ops: number;
  failed_ops: number;
  throughput: number;
  latency_p50: number;
  latency_p95: number;
  latency_p99: number;
  latency_mean: number;
  error_rate: number;
}

export interface LiveMetrics {
  run_id: string;
  throughput: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  latency_p99_ms: number;
  latency_mean?: number;
  error_rate: number;
  total_ops: number;
  failed_ops: number;
  success_ops?: number;
  duration_ms: number;
  timestamp?: number;
  time_series?: MetricsTimePoint[];
  operations_truncated?: boolean;
}

export interface MetricsDataPoint {
  timestamp: number;
  time: string;
  throughput: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  latency_p99_ms: number;
  latency_mean: number;
  error_rate: number;
  success_ops: number;
  failed_ops: number;
}

export interface MetricsSummary {
  total_ops: number;
  failed_ops: number;
  success_rate: number;
  avg_latency: number;
  peak_throughput: number;
  avg_error_rate: number;
  duration_seconds: number;
}

// JSON Schema type for tool input validation
export interface JSONSchema {
  type?: 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array' | 'null';
  properties?: Record<string, JSONSchema>;
  items?: JSONSchema;
  required?: string[];
  description?: string;
  default?: unknown;
  enum?: unknown[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  format?: string;
  title?: string;
  additionalProperties?: boolean | JSONSchema;
}

// MCP Tool annotations (from MCP spec)
export interface ToolAnnotations {
  readOnlyHint?: boolean;
  destructiveHint?: boolean;
  idempotentHint?: boolean;
  openWorldHint?: boolean;
}

// Tool fetched from MCP server
export interface FetchedTool {
  name: string;
  description?: string;
  inputSchema?: JSONSchema;
  annotations?: ToolAnnotations;
}

// Tool metrics from backend (matches analysis.OperationMetrics)
export interface ToolMetrics {
  total_ops: number;
  success_ops: number;
  failure_ops: number;
  latency_p50: number;
  latency_p95: number;
  latency_p99: number;
  error_rate: number;
}

// Aggregated metrics response with per-tool breakdown
export interface AggregatedMetrics extends LiveMetrics {
  by_tool?: Record<string, ToolMetrics>;
}

// Tool result content types
export interface ToolResultContent {
  type: 'text' | 'image' | 'resource';
  text?: string;
  data?: string;
  mimeType?: string;
  uri?: string;
}

// Tool execution result
export interface ToolExecutionResult {
  tool_name: string;
  timestamp_ms: number;
  latency_ms: number;
  ok: boolean;
  content?: ToolResultContent[];
  error_type?: string;
  error_message?: string;
  argument_size_bytes?: number;
}

// Argument preset for saving/loading
export interface ArgumentPreset {
  id: string;
  name: string;
  toolName: string;
  arguments: Record<string, unknown>;
  createdAt: number;
}

// Tool volume data point for time series
export interface ToolVolumeDataPoint {
  timestamp: number;
  time: string;
  [toolName: string]: number | string; // Dynamic tool names as keys
}

// Connection Stability Types
export interface ConnectionEvent {
  session_id: string;
  event_type: 'created' | 'active' | 'dropped' | 'terminated' | 'reconnect';
  timestamp: string;
  reason?: string;
  duration_ms?: number;
}

export interface ConnectionMetrics {
  session_id: string;
  created_at: string;
  last_active_at: string;
  terminated_at?: string;
  request_count: number;
  success_count: number;
  error_count: number;
  reconnect_count: number;
  protocol_errors: number;
  avg_latency_ms: number;
  state: string;
}

export interface StabilityTimePoint {
  timestamp: number;
  active_sessions: number;
  new_sessions: number;
  dropped_sessions: number;
  reconnects: number;
  avg_session_age_ms: number;
}

export interface StabilityMetrics {
  run_id: string;
  total_sessions: number;
  active_sessions: number;
  dropped_sessions: number;
  terminated_sessions: number;
  avg_session_lifetime_ms: number;
  reconnect_rate: number;
  protocol_error_rate: number;
  connection_churn_rate: number;
  stability_score: number;
  drop_rate: number;
  events?: ConnectionEvent[];
  session_metrics?: ConnectionMetrics[];
  time_series?: StabilityTimePoint[];
  data_truncated?: boolean;
}

// Server Telemetry Types (from mcpdrill-agent)
export interface ServerHostMetrics {
  cpu_percent: number;
  load_avg_1?: number;
  load_avg_5?: number;
  load_avg_15?: number;
  mem_total: number;
  mem_used: number;
  mem_available?: number;
}

export interface ServerProcessMetrics {
  pid: number;
  cpu_percent: number;
  mem_rss: number;
  mem_vms?: number;
  num_threads?: number;
  num_fds?: number;
  open_connections?: number;
}

export interface ServerMetricsSample {
  timestamp: number;
  host?: ServerHostMetrics;
  process?: ServerProcessMetrics;
}

export interface ServerMetricsAggregated {
  sample_count: number;
  cpu_max: number;
  cpu_avg: number;
  mem_max: number;
  mem_avg: number;
}

export interface ServerMetricsResponse {
  run_id: string;
  samples: ServerMetricsSample[];
  aggregated?: ServerMetricsAggregated;
}

export interface ServerMetricsDataPoint {
  timestamp: number;
  time: string;
  cpu_percent: number;
  memory_percent: number;
  memory_used_gb: number;
  memory_total_gb: number;
  load_avg_1: number;
  load_avg_5: number;
  load_avg_15: number;
}

// Truncation info for warning banner
export interface TruncationInfo {
  operationsTruncated: boolean;
  logsTruncated: boolean;
  dataTruncated: boolean;
}

// Stage marker for visualizing stage boundaries on charts
export interface StageMarker {
  timestamp: number;  // ms since epoch
  time: string;       // Formatted time string for X-axis positioning
  stage: string;      // 'preflight' | 'baseline' | 'ramp' | etc.
  label: string;      // Display text (uppercase stage name)
}

// Streaming operation telemetry (matches backend types.StreamInfo)
export interface StreamInfo {
  is_streaming: boolean;
  events_count: number;
  ended_normally: boolean;
  stalled: boolean;
  stall_duration_ms: number;
}
