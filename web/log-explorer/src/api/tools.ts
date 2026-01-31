import { normalizeMcpUrl } from './runs';
import type { ApiErrorBody, ConnectionTestResult, DiscoveredTool, ToolTestResult } from './types';

const API_BASE = '';

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

export async function discoverTools(
  targetUrl: string,
  headers?: Record<string, string>
): Promise<DiscoveredTool[]> {
  const response = await fetch(`${API_BASE}/discover-tools`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ 
      target_url: normalizeMcpUrl(targetUrl),
      headers: headers || {},
    }),
  });
  if (!response.ok) {
    throw new Error(await extractErrorMessage(response, 'Failed to discover tools'));
  }
  const data = await response.json();
  return data.tools || [];
}

export async function testConnection(
  targetUrl: string, 
  headers?: Record<string, string>
): Promise<ConnectionTestResult> {
  const response = await fetch(`${API_BASE}/test-connection`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ target_url: normalizeMcpUrl(targetUrl), headers: headers || {} }),
  });
  
  if (!response.ok) {
    const errorMsg = await extractErrorMessage(response, 'test connection');
    return {
      success: false,
      message: errorMsg,
    };
  }
  
  const data = await response.json();
  return data;
}

export async function testTool(
  targetUrl: string,
  toolName: string,
  args: Record<string, unknown>,
  headers?: Record<string, string>
): Promise<ToolTestResult> {
  const response = await fetch(`${API_BASE}/test-tool`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      target_url: normalizeMcpUrl(targetUrl),
      tool_name: toolName,
      arguments: args,
      headers: headers || {},
    }),
  });
  
  if (!response.ok) {
    const errorMsg = await extractErrorMessage(response, 'test tool');
    return {
      success: false,
      error: errorMsg,
      latency_ms: 0,
    };
  }
  
  const data = await response.json();
  return data;
}
