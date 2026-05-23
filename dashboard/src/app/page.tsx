"use client";

import { useEffect, useState } from "react";
import { apiClient } from "@/lib/api-client";
import { useSSE } from "@/lib/sse";
import type {
  CurrentMarket,
  LatencyResponse,
  LatestBook,
  PriceTick,
} from "@/lib/types";

export default function Home() {
  const [marketId, setMarketId] = useState<string | null>(null);
  const [marketQuestion, setMarketQuestion] = useState<string>("");

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const market: CurrentMarket = await apiClient.currentMarket();
        if (!cancelled) {
          setMarketId(market.marketId);
          setMarketQuestion(market.question);
        }
      } catch {
        if (!cancelled) {
          setMarketId(null);
          setMarketQuestion("");
        }
      }
    };
    load();
    const id = setInterval(load, 10_000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  const tick = useSSE<PriceTick>("/api/stream/prices?exchange=binance&symbol=btcusdt");
  const book = useSSE<LatestBook>(
    marketId ? `/api/stream/book?marketId=${encodeURIComponent(marketId)}` : null
  );
  const latency = useSSE<LatencyResponse>("/api/stream/latency");

  return (
    <main className="p-6 max-w-6xl mx-auto space-y-4">
      <div className="grid gap-4 md:grid-cols-2">
        <BinanceCard tick={tick.data} state={tick.state} error={tick.error} />
        <PolymarketCard
          question={marketQuestion}
          book={book.data}
          state={book.state}
          error={book.error}
        />
      </div>
      <LatencyCard data={latency.data} state={latency.state} error={latency.error} />
    </main>
  );
}

function BinanceCard({
  tick,
  state,
  error,
}: {
  tick: PriceTick | null;
  state: string;
  error: string | null;
}) {
  return (
    <section className="rounded-lg border border-white/10 bg-white/[0.02] p-5 space-y-3">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wide opacity-70">
          BINANCE · BTC/USDT
        </h2>
        <ConnectionPill state={state} />
      </header>
      <div className="font-mono text-3xl">{tick ? formatUSD(tick.price) : "—"}</div>
      <div className="text-xs opacity-50">
        {tick
          ? new Date(tick.tsExchange).toLocaleTimeString()
          : "waiting for first tick…"}
      </div>
      {error && <div className="text-xs text-red-400">{error}</div>}
    </section>
  );
}

function PolymarketCard({
  question,
  book,
  state,
  error,
}: {
  question: string;
  book: LatestBook | null;
  state: string;
  error: string | null;
}) {
  return (
    <section className="rounded-lg border border-white/10 bg-white/[0.02] p-5 space-y-3">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wide opacity-70">
          POLYMARKET · 5M BTC
        </h2>
        <ConnectionPill state={state} />
      </header>
      <div className="font-mono text-3xl">{book?.mid ? formatProb(book.mid) : "—"}</div>
      <div className="text-xs opacity-50 truncate" title={question}>
        {question || "discovering market…"}
      </div>
      <div className="grid grid-cols-2 gap-2 text-xs font-mono pt-1">
        <Stat label="YES bid" value={book?.yesBid} />
        <Stat label="YES ask" value={book?.yesAsk} />
        <Stat label="NO bid" value={book?.noBid} />
        <Stat label="NO ask" value={book?.noAsk} />
      </div>
      {error && <div className="text-xs text-red-400">{error}</div>}
    </section>
  );
}

function LatencyCard({
  data,
  state,
  error,
}: {
  data: LatencyResponse | null;
  state: string;
  error: string | null;
}) {
  const stats = data?.stats;
  const samples = data?.samples ?? [];

  return (
    <section className="rounded-lg border border-white/10 bg-white/[0.02] p-5 space-y-4">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wide opacity-70">
          LATENCY · Binance move → Polymarket reprice
        </h2>
        <ConnectionPill state={state} />
      </header>
      <div className="grid grid-cols-4 gap-4">
        <BigStat label="last" value={stats?.lastDeltaMs} suffix="ms" />
        <BigStat label="p50" value={stats?.p50Ms} suffix="ms" />
        <BigStat label="p95" value={stats?.p95Ms} suffix="ms" />
        <BigStat
          label={`samples (${stats?.windowSecs ?? 60}s)`}
          value={stats?.count}
        />
      </div>
      <DeltaBars samples={samples} />
      <div className="text-xs opacity-50">
        Significant move = &gt;0.05% from reference. Pending pairs:{" "}
        {stats?.pendingMoves ?? 0}.
      </div>
      {error && <div className="text-xs text-red-400">{error}</div>}
    </section>
  );
}

function DeltaBars({ samples }: { samples: { deltaMs: number }[] }) {
  if (samples.length === 0) {
    return (
      <div className="text-xs opacity-50 italic h-12 flex items-center">
        Waiting for first paired move…
      </div>
    );
  }
  const max = Math.max(...samples.map((s) => s.deltaMs), 1);
  return (
    <div className="h-12 flex items-end gap-[2px]">
      {samples.map((s, i) => {
        const heightPct = (s.deltaMs / max) * 100;
        const tone =
          s.deltaMs < 1000
            ? "bg-emerald-500"
            : s.deltaMs < 5000
              ? "bg-amber-500"
              : "bg-red-500";
        return (
          <div
            key={i}
            className={`flex-1 rounded-sm ${tone} opacity-70`}
            style={{ height: `${heightPct}%` }}
            title={`${s.deltaMs} ms`}
          />
        );
      })}
    </div>
  );
}

function Stat({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="opacity-50 w-14">{label}</span>
      <span>{value ?? "—"}</span>
    </div>
  );
}

function BigStat({
  label,
  value,
  suffix,
}: {
  label: string;
  value?: number;
  suffix?: string;
}) {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-wider opacity-50">{label}</div>
      <div className="font-mono text-2xl">
        {value !== undefined && value !== null ? (
          <>
            {value.toLocaleString()}
            {suffix && (
              <span className="text-sm opacity-50 ml-1">{suffix}</span>
            )}
          </>
        ) : (
          "—"
        )}
      </div>
    </div>
  );
}

function ConnectionPill({ state }: { state: string }) {
  const cls =
    state === "open"
      ? "bg-emerald-500/20 text-emerald-300"
      : state === "connecting"
        ? "bg-amber-500/20 text-amber-300"
        : "bg-red-500/20 text-red-300";
  return (
    <span
      className={`text-[10px] uppercase tracking-wider px-2 py-0.5 rounded ${cls}`}
    >
      {state}
    </span>
  );
}

function formatUSD(price: string): string {
  const n = Number.parseFloat(price);
  if (Number.isNaN(n)) return price;
  return n.toLocaleString("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

function formatProb(p: string): string {
  const n = Number.parseFloat(p);
  if (Number.isNaN(n)) return p;
  return `${(n * 100).toFixed(1)}%`;
}
