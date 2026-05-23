import type { CurrentMarket, LatestBook, PriceTick } from "./types";

const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  return res.json();
}

export const apiClient = {
  health(): Promise<{ status: string }> {
    return fetchJSON("/api/health");
  },
  latestPrice(exchange: string, symbol: string): Promise<PriceTick> {
    return fetchJSON(`/api/prices/latest?exchange=${exchange}&symbol=${symbol}`);
  },
  currentMarket(): Promise<CurrentMarket> {
    return fetchJSON("/api/polymarket/current");
  },
  latestBook(marketId: string): Promise<LatestBook> {
    return fetchJSON(`/api/polymarket/book/${encodeURIComponent(marketId)}`);
  },
};

export const sseBase = BASE;
