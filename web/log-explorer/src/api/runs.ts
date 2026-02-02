import type {
  LogQueryResponse,
  RunInfo,
  LogFilters,
  RunConfig,
  ComparisonResult,
  ComparisonApiResponse,
  ComparisonDiff,
  RunMetrics,
  OpMixEntry,
} from '../types';
import type {
  BackendToolTemplate,
  BackendStopCondition,
  BackendRunConfig,
  ApiErrorBody,
  AgentInfo,
  StopMode,
  StopRunResponse,
  ValidationResult,
  AgentDetail,
  ErrorSignaturesResponse,
} from './types';

const API_BASE = '';

export function normalizeMcpUrl(url: string): string {
  if (!url) return url;
  
  const trimmed = url.trim();
  if (!trimmed) return trimmed;
  
  try {
    const parsed = new URL(trimmed);
    
    parsed.hash = '';
    
    let pathname = parsed.pathname.replace(/\/+$/, '');
    
    if (!pathname.endsWith('/mcp')) {
      pathname = `${pathname}/mcp`;
    }
    
    parsed.pathname = pathname;
    return parsed.toString();
  } catch {
    const withoutFragment = trimmed.split('#')[0];
    const withoutTrailingSlashes = withoutFragment.replace(/\/+$/, '');
    
    if (withoutTrailingSlashes.endsWith('/mcp')) {
      return withoutTrailingSlashes;
    }
    
    return `${withoutTrailingSlashes}/mcp`;
  }
}

function mapOperation(op: OpMixEntry['operation']): string {
  const mapping: Record<string, string> = {
    'tools/list': 'tools_list',
    'tools/call': 'tools_call',
    'resources/list': 'resources_list',
    'resources/read': 'resources_read',
    'prompts/list': 'prompts_list',
    'prompts/get': 'prompts_get',
    'ping': 'ping',
    'custom': 'custom',
    'initialize': 'initialize',
  };
  return mapping[op] || 'tools_list';
}

function convertToBackendConfig(config: RunConfig): BackendRunConfig {
  const targetUrl = new URL(config.target.url);
  const hostSuffix = targetUrl.hostname.includes('localhost') 
    ? 'localhost' 
    : `.${targetUrl.hostname.split('.').slice(-2).join('.')}`;

  const toolTemplates: BackendToolTemplate[] = [];
  config.workload.op_mix.forEach((op, idx) => {
    if (op.operation === 'tools/call' && op.tool_name) {
      toolTemplates.push({
        template_id: `tool_${idx}_${op.tool_name}`,
        tool_name: op.tool_name,
        weight: op.weight,
        arguments: (op.arguments || {}) as Record<string, unknown>,
        expects_streaming: false,
      });
    }
  });

  const maxVus = Math.max(...config.stages.map(s => s.load.target_vus), 10);
  const totalDuration = config.stages.reduce((sum, s) => sum + (s.enabled ? s.duration_ms : 0), 0);

  return {
    schema_version: 'run-config/v1',
    scenario_id: config.scenario_id,
    target: {
      kind: config.target.kind,
      url: normalizeMcpUrl(config.target.url),
      transport: config.target.transport,
      headers: config.target.headers || {},
      auth: {
        type: config.target.auth?.type || 'none',
        tokens: config.target.auth?.tokens,
      },
      identification: {
        run_id_header: {
          name: 'X-MCPDrill-Run-Id',
          value_template: '${run_id}',
        },
        user_agent: {
          value: 'mcpdrill/1.0 (run=${run_id})',
        },
      },
      timeouts: {
        connect_timeout_ms: 5000,
        request_timeout_ms: 30000,
        stream_stall_timeout_ms: 15000,
      },
      tls: {
        verify: true,
        ca_bundle_ref: null,
      },
      redirect_policy: {
        mode: 'deny',
        max_redirects: 3,
      },
    },
    environment: {
      allowlist: {
        mode: 'deny_by_default',
        allowed_targets: [
          { kind: 'suffix', value: hostSuffix },
        ],
      },
      forbidden_patterns: [],
    },
    session_policy: {
      mode: config.session_policy?.mode || 'reuse',
      pool_size: config.session_policy?.pool_size ?? 10,
      ttl_ms: config.session_policy?.ttl_ms ?? 60000,
      max_idle_ms: config.session_policy?.max_idle_ms ?? 30000,
    },
    workload: {
      in_flight_per_vu: 1,
      think_time: {
        mode: 'fixed',
        base_ms: 100,
        jitter_ms: 0,
      },
      operation_mix: config.workload.op_mix.map(op => ({
        operation: mapOperation(op.operation),
        weight: op.weight,
      })),
      tools: {
        selection: {
          mode: toolTemplates.length > 0 ? 'round_robin' : 'round_robin',
        },
        templates: toolTemplates,
      },
      payload_profiles: [],
    },
    stages: config.stages
      .filter(s => s.enabled)
      .map(stage => {
        let stopConditions: BackendStopCondition[] = (stage.stop_conditions || []).map((sc, idx) => ({
          id: sc.id || `${stage.stage}_condition_${idx}`,
          metric: sc.metric,
          comparator: sc.comparator || '>',
          threshold: sc.threshold,
          window_ms: sc.window_ms,
          sustain_windows: sc.sustain_windows || 1,
          scope: {},
        }));
        
        if ((stage.stage === 'baseline' || stage.stage === 'ramp') && stopConditions.length === 0) {
          stopConditions = [{
            id: `${stage.stage}_default_error_rate`,
            metric: 'error_rate',
            comparator: '>',
            threshold: 0.5,
            window_ms: 5000,
            sustain_windows: 1,
            scope: {},
          }];
        }
        
        return {
          stage_id: stage.stage_id,
          stage: stage.stage,
          enabled: true,
          duration_ms: stage.duration_ms,
          load: {
            target_vus: stage.load.target_vus,
            target_rps: stage.load.target_vus * 10,
          },
          stop_conditions: stopConditions,
        };
      }),
    safety: {
      ramp_by_default: false,
      emergency_stop_enabled: true,
      worker_failure_policy: 'fail_fast',
      hard_caps: {
        max_vus: Math.max(maxVus * 2, 100),
        max_rps: Math.max(maxVus * 20, 1000),
        max_connections: Math.max(maxVus * 2, 100),
        max_duration_ms: Math.max(totalDuration * 2, 300000),
        max_in_flight_per_vu: 2,
        max_telemetry_q_depth: 10000,
      },
      stop_policy: {
        mode: 'drain',
        drain_timeout_ms: 5000,
      },
      identification_required: false,
    },
    reporting: {
      formats: ['json'],
      retention: {
        raw_logs_days: 7,
        metrics_days: 30,
        reports_days: 90,
      },
      include: {
        store_raw_logs: true,
        store_metrics_snapshot: true,
        store_event_log: true,
      },
      redaction: {
        redact_headers: ['Authorization', 'X-API-Key'],
      },
    },
    telemetry: {
      structured_logs: {
        enabled: true,
        sample_rate: 1.0,
      },
      traces: {
        enabled: false,
        propagation: {
          accept_incoming_traceparent: false,
        },
      },
    },
    server_telemetry: config.server_telemetry?.enabled ? {
      enabled: config.server_telemetry.enabled,
      pair_key: config.server_telemetry.pair_key,
    } : undefined,
  };
}

async function extractErrorMessage(response: Response, fallbackAction: string): Promise<string> {
  const status = response.status;
  let serverMessage = '';
  
  try {
    const text = await response.text();
    if (text) {
      try {
        const json = JSON.parse(text) as ApiErrorBody;
        serverMessage = json.error || json.error_message || json.message || json.detail || text;
      } catch {
        serverMessage = text;
      }
    }
  } catch {
    serverMessage = response.statusText;
  }

  if (status === 400) {
    return `Invalid request: ${serverMessage || 'Please check your input'}`;
  }
  if (status === 401 || status === 403) {
    return `Authentication required: ${serverMessage || 'Please log in and try again'}`;
  }
  if (status === 404) {
    return `Not found: ${serverMessage || 'The requested resource does not exist'}`;
  }
  if (status === 409) {
    return `Conflict: ${serverMessage || 'The operation could not be completed'}`;
  }
  if (status === 429) {
    return `Too many requests: ${serverMessage || 'Please wait and try again'}`;
  }
  if (status >= 500) {
    return `Server error (${status}): ${serverMessage || 'Please try again later'}`;
  }
  
  return serverMessage 
    ? `${fallbackAction}: ${serverMessage}` 
    : `${fallbackAction}: ${response.statusText || 'Unknown error'}`;
}

async function handleResponse<T>(response: Response, errorAction: string): Promise<T> {
  if (!response.ok) {
    throw new Error(await extractErrorMessage(response, errorAction));
  }
  return response.json();
}

export async function createRun(config: RunConfig): Promise<string> {
  const backendConfig = convertToBackendConfig(config);
  const response = await fetch(`${API_BASE}/runs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config: backendConfig }),
  });
  if (!response.ok) {
    throw new Error(await extractErrorMessage(response, 'Failed to create run'));
  }
  const data = await response.json();
  return data.run_id;
}

export async function startRun(runId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/runs/${runId}/start`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw new Error(await extractErrorMessage(response, 'Failed to start run'));
  }
}

export async function fetchRuns(): Promise<RunInfo[]> {
  const response = await fetch(`${API_BASE}/runs`);
  if (!response.ok) {
    throw new Error(await extractErrorMessage(response, 'Failed to fetch runs'));
  }
  const data = await response.json();
  const runs = (data.runs || []).map((run: { run_id?: string; id?: string; scenario_id: string; state: string; created_at?: string; created_at_ms?: number }) => ({
    id: run.run_id || run.id || '',
    scenario_id: run.scenario_id,
    state: run.state,
    created_at: run.created_at || (run.created_at_ms ? new Date(run.created_at_ms).toISOString() : ''),
  }));
  runs.sort((a: RunInfo, b: RunInfo) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
  return runs;
}

export async function fetchRun(runId: string): Promise<RunInfo> {
  const response = await fetch(`${API_BASE}/runs/${runId}`);
  if (!response.ok) {
    throw new Error(await extractErrorMessage(response, 'Failed to fetch run'));
  }
  const run = await response.json();
  return {
    id: run.run_id || run.id || runId,
    scenario_id: run.scenario_id || '',
    state: run.state || '',
    created_at: run.created_at || (run.created_at_ms ? new Date(run.created_at_ms).toISOString() : ''),
    started_at: run.started_at || (run.started_at_ms ? new Date(run.started_at_ms).toISOString() : undefined),
    completed_at: run.completed_at || (run.completed_at_ms ? new Date(run.completed_at_ms).toISOString() : undefined),
    stop_reason: run.stop_reason,
  };
}

export async function fetchAgents(pairKey?: string): Promise<AgentInfo[]> {
  const params = new URLSearchParams();
  if (pairKey) params.set('pair_key', pairKey);
  
  const url = params.toString() ? `${API_BASE}/agents?${params}` : `${API_BASE}/agents`;
  const response = await fetch(url);
  
  if (!response.ok) {
    if (response.status === 404) {
      return [];
    }
    throw new Error(await extractErrorMessage(response, 'Failed to fetch agents'));
  }
  
  const data = await response.json();
  return data.agents || [];
}

export async function fetchLogs(
  runId: string,
  filters: LogFilters,
  offset: number,
  limit: number,
  signal?: AbortSignal
): Promise<LogQueryResponse> {
  const params = new URLSearchParams();
  
  if (filters.stage) params.set('stage', filters.stage);
  if (filters.stage_id) params.set('stage_id', filters.stage_id);
  if (filters.worker_id) params.set('worker_id', filters.worker_id);
  if (filters.session_id) params.set('session_id', filters.session_id);
  if (filters.vu_id) params.set('vu_id', filters.vu_id);
  if (filters.operation) params.set('operation', filters.operation);
  if (filters.tool_name) params.set('tool_name', filters.tool_name);
  if (filters.error_type) params.set('error_type', filters.error_type);
  if (filters.error_code) params.set('error_code', filters.error_code);
  
  params.set('offset', offset.toString());
  params.set('limit', limit.toString());
  params.set('order', 'desc');
  
  const url = `${API_BASE}/runs/${runId}/logs?${params.toString()}`;
  const response = await fetch(url, { signal });
  return handleResponse<LogQueryResponse>(response, 'Failed to fetch logs');
}

export function exportAsJSON(logs: unknown[], filename: string): void {
  const json = JSON.stringify(logs, null, 2);
  downloadFile(json, filename, 'application/json');
}

export function exportAsCSV<T extends object>(logs: T[], filename: string): void {
   if (logs.length === 0) return;
   
   const headers = Object.keys(logs[0]) as Array<keyof T>;
   const csvRows = [
     headers.join(','),
     ...logs.map(log => 
       headers.map(h => {
         const val = log[h];
         const str = String(val ?? '');
         const dangerousPrefix = /^[=+\-@\t\r]/.test(str);
         const needsQuotes = str.includes(',') || str.includes('"') || str.includes('\n') || dangerousPrefix;
         const escaped = needsQuotes ? `"${str.replace(/"/g, '""')}"` : str;
         if (dangerousPrefix) {
           return `'${escaped}`;
         }
         return escaped;
       }).join(',')
     )
   ];
   
   downloadFile(csvRows.join('\n'), filename, 'text/csv');
}

function downloadFile(content: string, filename: string, mimeType: string): void {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}

export async function fetchRunMetrics(runId: string): Promise<RunMetrics> {
  const response = await fetch(`${API_BASE}/runs/${runId}/metrics`);
  return handleResponse<RunMetrics>(response, 'Failed to fetch metrics');
}

function computeDiff(runA: ComparisonApiResponse['run_a'], runB: ComparisonApiResponse['run_b']): ComparisonDiff {
  const pctDiff = (a: number, b: number) => a === 0 ? (b === 0 ? 0 : 100) : ((b - a) / a) * 100;
  
  return {
    latency_p50_ms: runB.latency_p50_ms - runA.latency_p50_ms,
    latency_p50_pct: pctDiff(runA.latency_p50_ms, runB.latency_p50_ms),
    latency_p95_ms: runB.latency_p95_ms - runA.latency_p95_ms,
    latency_p95_pct: pctDiff(runA.latency_p95_ms, runB.latency_p95_ms),
    latency_p99_ms: runB.latency_p99_ms - runA.latency_p99_ms,
    latency_p99_pct: pctDiff(runA.latency_p99_ms, runB.latency_p99_ms),
    throughput: runB.throughput - runA.throughput,
    throughput_pct: pctDiff(runA.throughput, runB.throughput),
    error_rate: runB.error_rate - runA.error_rate,
    error_rate_pct: pctDiff(runA.error_rate, runB.error_rate),
  };
}

export async function fetchComparison(runIdA: string, runIdB: string): Promise<ComparisonResult> {
  const response = await fetch(`${API_BASE}/runs/${runIdA}/compare/${runIdB}`);
  const apiResponse = await handleResponse<ComparisonApiResponse>(response, 'Failed to fetch comparison');
  
  return {
    run_a: apiResponse.run_a,
    run_b: apiResponse.run_b,
    diff: computeDiff(apiResponse.run_a, apiResponse.run_b),
  };
}

/**
 * Stop a running test with specified mode
 * @param runId - The run ID to stop
 * @param mode - 'drain' (graceful, wait for in-flight) or 'immediate' (cancel immediately)
 * @param actor - Actor name for audit logging (defaults to 'ui')
 */
export async function stopRun(
  runId: string,
  mode: StopMode = 'drain',
  actor: string = 'ui'
): Promise<StopRunResponse> {
  const response = await fetch(`${API_BASE}/runs/${runId}/stop`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode, actor }),
  });
  return handleResponse<StopRunResponse>(response, 'Failed to stop run');
}

/**
 * Emergency stop a running test - immediate termination, no cleanup
 * @param runId - The run ID to emergency stop
 * @param actor - Actor name for audit logging (defaults to 'ui')
 */
export async function emergencyStopRun(
  runId: string,
  actor: string = 'ui'
): Promise<StopRunResponse> {
  const response = await fetch(`${API_BASE}/runs/${runId}/emergency-stop`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ actor }),
  });
  return handleResponse<StopRunResponse>(response, 'Failed to emergency stop run');
}

/**
 * Validate run configuration before creating
 */
export async function validateRunConfig(config: RunConfig): Promise<ValidationResult> {
  const backendConfig = convertToBackendConfig(config);
  const response = await fetch(`${API_BASE}/runs/validate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config: backendConfig }),
  });
  
  if (!response.ok) {
    // Try to extract structured validation errors
    try {
      const data = await response.json();
      if (data.errors || data.warnings) {
        return {
          ok: false,
          errors: data.errors || [],
          warnings: data.warnings || [],
        };
      }
    } catch {
      // Fall through to generic error
    }
    throw new Error(await extractErrorMessage(response, 'Failed to validate config'));
  }
  
  return response.json();
}

/**
 * Fetch detailed information about a specific agent
 */
export async function fetchAgentDetail(agentId: string): Promise<AgentDetail> {
  const response = await fetch(`${API_BASE}/agents/${agentId}`);
  return handleResponse<AgentDetail>(response, 'Failed to fetch agent details');
}

/**
 * Fetch error signatures for a run
 */
export async function fetchErrorSignatures(runId: string): Promise<ErrorSignaturesResponse> {
  const response = await fetch(`${API_BASE}/runs/${runId}/errors/signatures`);
  
  if (!response.ok) {
    if (response.status === 404) {
      return { run_id: runId, signatures: [] };
    }
    throw new Error(await extractErrorMessage(response, 'Failed to fetch error signatures'));
  }
  
  return response.json();
}

export interface CloneRunResponse {
  run_id: string;
}

export async function cloneRun(runId: string, actor: string = 'ui'): Promise<CloneRunResponse> {
  const response = await fetch(`${API_BASE}/runs/${runId}/clone`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ actor }),
  });
  return handleResponse<CloneRunResponse>(response, 'Failed to clone run');
}
