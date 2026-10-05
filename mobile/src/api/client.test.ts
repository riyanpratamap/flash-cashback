import { apiBaseUrl, request } from './client';

type FetchInit = RequestInit & { headers: Record<string, string> };

function jsonResponse(status: number, body: unknown, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  });
}

const fetchMock = jest.fn<Promise<Response>, [string, FetchInit]>();

beforeEach(() => {
  fetchMock.mockReset();
  globalThis.fetch = fetchMock as unknown as typeof fetch;
});

afterEach(() => {
  jest.useRealTimers();
  delete process.env.EXPO_PUBLIC_API_URL;
});

const get = () => request({ method: 'GET', path: '/campaign', user: 'user_a' });
const post = () =>
  request({ method: 'POST', path: '/payments', user: 'user_a', idempotencyKey: 'key-1', body: { amount: 100000 } });

describe('apiBaseUrl (AC-68)', () => {
  it('defaults to localhost', () => {
    delete process.env.EXPO_PUBLIC_API_URL;
    expect(apiBaseUrl()).toBe('http://localhost:8080/v1');
  });

  it('uses EXPO_PUBLIC_API_URL when set, and requests go there', async () => {
    process.env.EXPO_PUBLIC_API_URL = 'http://192.168.1.5:8080/v1';
    fetchMock.mockResolvedValue(jsonResponse(200, {}));
    await get();
    expect(fetchMock.mock.calls[0]?.[0]).toBe('http://192.168.1.5:8080/v1/campaign');
  });
});

describe('headers', () => {
  it('sends X-User-ID of the given user on a GET and no idempotency key', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {}));
    await request({ method: 'GET', path: '/me/cashback', user: 'user_b' });
    const init = fetchMock.mock.calls[0]?.[1];
    expect(init?.headers['X-User-ID']).toBe('user_b');
    expect(init?.headers['Idempotency-Key']).toBeUndefined();
  });

  it('sends the key, JSON content type and body on a POST', async () => {
    fetchMock.mockResolvedValue(jsonResponse(201, {}));
    await post();
    const init = fetchMock.mock.calls[0]?.[1];
    expect(init?.method).toBe('POST');
    expect(init?.headers['X-User-ID']).toBe('user_a');
    expect(init?.headers['Idempotency-Key']).toBe('key-1');
    expect(init?.headers['Content-Type']).toBe('application/json');
    expect(init?.body).toBe('{"amount":100000}');
  });
});

describe('classification', () => {
  it('200 is ok with the body', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { a: 1 }));
    expect(await get()).toEqual({ kind: 'ok', status: 200, body: { a: 1 }, replayed: false });
  });

  it('201 is ok', async () => {
    fetchMock.mockResolvedValue(jsonResponse(201, { a: 1 }));
    expect(await post()).toMatchObject({ kind: 'ok', status: 201, replayed: false });
  });

  it('Idempotent-Replayed: true marks a replay', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {}, { 'Idempotent-Replayed': 'true' }));
    expect(await post()).toMatchObject({ kind: 'ok', replayed: true });
  });

  it.each([
    [400, 'MALFORMED_REQUEST'],
    [409, 'REDEMPTION_PAUSED'],
    [422, 'INSUFFICIENT_BALANCE'],
  ])('%d is rejected with its code', async (status, code) => {
    fetchMock.mockResolvedValue(jsonResponse(status, { error: { code, message: 'm', request_id: 'r-1' } }));
    expect(await post()).toEqual({ kind: 'rejected', status, code, message: 'm', requestId: 'r-1' });
  });

  it('a 4xx with a non-JSON body is rejected with a fallback code', async () => {
    fetchMock.mockResolvedValue(new Response('<html>bad gateway</html>', { status: 404 }));
    expect(await get()).toMatchObject({ kind: 'rejected', status: 404, code: 'UNREADABLE_ERROR' });
  });

  it.each([500, 503])('%d is unknown', async (status) => {
    fetchMock.mockResolvedValue(jsonResponse(status, { error: { code: 'SERVICE_BUSY', message: 'm', request_id: 'r' } }));
    expect(await post()).toEqual({ kind: 'unknown', reason: 'server' });
  });

  it('a network error is unknown', async () => {
    fetchMock.mockRejectedValue(new TypeError('Network request failed'));
    expect(await post()).toEqual({ kind: 'unknown', reason: 'network' });
  });

  it('a 2xx whose body cannot be read is unknown', async () => {
    fetchMock.mockResolvedValue(new Response('oops', { status: 201 }));
    expect(await post()).toEqual({ kind: 'unknown', reason: 'server' });
  });

  it('a request still pending after 10 s is aborted and is unknown, never rejected', async () => {
    jest.useFakeTimers();
    fetchMock.mockImplementation(
      (_url, init) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
        }),
    );
    const pending = post();
    await jest.advanceTimersByTimeAsync(9999);
    expect(fetchMock.mock.calls[0]?.[1].signal?.aborted).toBe(false);
    await jest.advanceTimersByTimeAsync(1);
    expect(await pending).toEqual({ kind: 'unknown', reason: 'timeout' });
  });
});
