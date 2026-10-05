import { request } from '@/api/client';
import { isInt, isRecord, isStr } from '@/api/guards';

// Response shapes of docs/api-contract.md. Status and reason codes stay `string`: the server can add one, and
// src/copy/codes.ts maps every code to copy with a fallback (D20).

export type Campaign = {
  id: string;
  name: string;
  status: string;
  redemption_status: string;
  rules: { rate_bps: number; min_payment: number; daily_cap: number; timezone: string };
};

export type CashbackSummary = {
  balance: number;
  today: { date: string; earned: number; remaining: number; resets_at: string };
};

export type CashbackResult = { awarded: number; reason: string };

export type HistoryItem =
  | {
      type: 'PAYMENT';
      id: number;
      reference: string;
      amount: number;
      status: string;
      created_at: string;
      cashback: CashbackResult;
    }
  | {
      type: 'REDEMPTION';
      id: number;
      reference: string;
      amount: number;
      status: string;
      destination: string;
      created_at: string;
    };

export type History = { items: readonly HistoryItem[] };

export const queryKeys = {
  campaign: () => ['campaign'] as const,
  cashback: (user: string) => ['cashback', user] as const,
  history: (user: string, limit: number) => ['history', user, limit] as const,
};


function isCampaign(v: unknown): v is Campaign {
  if (!isRecord(v) || !isRecord(v.rules)) return false;
  const r = v.rules;
  return (
    isStr(v.id) &&
    isStr(v.name) &&
    isStr(v.status) &&
    isStr(v.redemption_status) &&
    isInt(r.rate_bps) &&
    isInt(r.min_payment) &&
    isInt(r.daily_cap) &&
    isStr(r.timezone)
  );
}

function isCashbackSummary(v: unknown): v is CashbackSummary {
  if (!isRecord(v) || !isRecord(v.today)) return false;
  const t = v.today;
  return isInt(v.balance) && isStr(t.date) && isInt(t.earned) && isInt(t.remaining) && isStr(t.resets_at);
}

function isHistoryItem(v: unknown): v is HistoryItem {
  if (!isRecord(v)) return false;
  const common = isInt(v.id) && isStr(v.reference) && isInt(v.amount) && isStr(v.status) && isStr(v.created_at);
  if (!common) return false;
  if (v.type === 'PAYMENT') {
    return isRecord(v.cashback) && isInt(v.cashback.awarded) && isStr(v.cashback.reason);
  }
  return v.type === 'REDEMPTION' && isStr(v.destination);
}

function isHistory(v: unknown): v is History {
  return isRecord(v) && Array.isArray(v.items) && v.items.every(isHistoryItem);
}

async function get<T>(path: string, user: string, isValid: (body: unknown) => body is T): Promise<T> {
  const result = await request({ method: 'GET', path, user });
  if (result.kind === 'ok') {
    if (isValid(result.body)) return result.body;
    throw new Error(`GET ${path}: unexpected response shape`);
  }
  // Rejected or unknown: the query layer shows its error state and retries once (D33).
  throw new Error(
    result.kind === 'rejected' ? `GET ${path}: ${result.status} ${result.code}` : `GET ${path}: ${result.reason}`,
  );
}

export const campaignQuery = (user: string) => get('/campaign', user, isCampaign);
export const cashbackQuery = (user: string) => get('/me/cashback', user, isCashbackSummary);
export const historyQuery = (user: string, limit: number) => get(`/me/history?limit=${limit}`, user, isHistory);
