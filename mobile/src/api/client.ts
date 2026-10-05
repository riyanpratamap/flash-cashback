export type ApiResult =
  | { kind: 'ok'; status: number; body: unknown; replayed: boolean }
  | { kind: 'rejected'; status: number; code: string; message: string; requestId: string }
  | { kind: 'unknown'; reason: 'timeout' | 'network' | 'server' };

export type ApiRequest = {
  method: 'GET' | 'POST';
  path: string;
  user: string;
  idempotencyKey?: string;
  body?: unknown;
};

const DEFAULT_BASE_URL = 'http://localhost:8080/v1';
const TIMEOUT_MS = 10_000;
const UNREADABLE_ERROR = 'UNREADABLE_ERROR';

export function apiBaseUrl(): string {
  return process.env.EXPO_PUBLIC_API_URL ?? DEFAULT_BASE_URL;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function errorFields(body: unknown): { code: string; message: string; requestId: string } {
  const error = isRecord(body) ? body.error : undefined;
  if (!isRecord(error) || typeof error.code !== 'string') {
    return { code: UNREADABLE_ERROR, message: '', requestId: '' };
  }
  return {
    code: error.code,
    message: typeof error.message === 'string' ? error.message : '',
    requestId: typeof error.request_id === 'string' ? error.request_id : '',
  };
}

async function readJson(response: Response): Promise<{ ok: true; value: unknown } | { ok: false }> {
  try {
    return { ok: true, value: await response.json() };
  } catch {
    return { ok: false };
  }
}

async function classify(response: Response): Promise<ApiResult> {
  const { status } = response;
  if (status >= 200 && status < 300) {
    const json = await readJson(response);
    if (!json.ok) return { kind: 'unknown', reason: 'server' };
    return {
      kind: 'ok',
      status,
      body: json.value,
      replayed: response.headers.get('Idempotent-Replayed') === 'true',
    };
  }
  if (status >= 400 && status < 500) {
    const json = await readJson(response);
    return { kind: 'rejected', status, ...errorFields(json.ok ? json.value : undefined) };
  }
  return { kind: 'unknown', reason: 'server' };
}

/**
 * One API call, classified. Expected failures are returned, never thrown. A timeout is unknown: the request may
 * have committed on the server.
 */
export async function request(req: ApiRequest): Promise<ApiResult> {
  const headers: Record<string, string> = { 'X-User-ID': req.user };
  if (req.method === 'POST') {
    headers['Content-Type'] = 'application/json';
    if (req.idempotencyKey !== undefined) headers['Idempotency-Key'] = req.idempotencyKey;
  }

  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, TIMEOUT_MS);

  try {
    const response = await fetch(`${apiBaseUrl()}${req.path}`, {
      method: req.method,
      headers,
      body: req.body === undefined ? undefined : JSON.stringify(req.body),
      signal: controller.signal,
    });
    return await classify(response);
  } catch {
    return { kind: 'unknown', reason: timedOut ? 'timeout' : 'network' };
  } finally {
    clearTimeout(timer);
  }
}
