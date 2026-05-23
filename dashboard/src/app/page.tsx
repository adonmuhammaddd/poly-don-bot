"use client";

import { useEffect, useState } from "react";
import { apiClient } from "@/lib/api-client";
import { useSSE } from "@/lib/sse";
import type {
  CurrentMarket,
  LatencyResponse,
  LatestBook,
  PriceTick,
  Signal,
  SignalDirection,
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
  const signals = useSSE<Signal[]>("/api/stream/signals");

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
      <SignalsCard data={signals.data} state={signals.state} error={signals.error} />
    </main>
  );
}

type SignalFilter = SignalDirection | "all";

function SignalsCard({
  data,
  state,
  error,
}: {
  data: Signal[] | null;
  state: string;
  error: string | null;
}) {
  const [filter, setFilter] = useState<SignalFilter>("all");

  const all = data ?? [];
  const filtered = filter === "all" ? all : all.filter((s) => s.direction === filter);
  const buckets = bucketByConfidence(all);

  return (
    <section className="rounded-lg border border-white/10 bg-white/[0.02] p-5 space-y-4">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wide opacity-70">
          SIGNALS · momentum detector
        </h2>
        <ConnectionPill state={state} />
      </header>

      <div className="flex items-center gap-2 text-xs">
        <FilterChip current={filter} value="all" label={`all (${all.length})`} onSelect={setFilter} />
        <FilterChip
          current={filter}
          value="up"
          label={`up (${all.filter((s) => s.direction === "up").length})`}
          onSelect={setFilter}
        />
        <FilterChip
          current={filter}
          value="down"
          label={`down (${all.filter((s) => s.direction === "down").length})`}
          onSelect={setFilter}
        />
        <span className="text-xs opacity-50 ml-auto">last {all.length} signals</span>
      </div>

      <ConfidenceHistogram buckets={buckets} />

      {filtered.length === 0 ? (
        <div className="text-xs opacity-50 italic py-6 text-center">
          {all.length === 0 ? "No signals yet — waiting for BTC to move >0.1%…" : "No signals match this filter."}
        </div>
      ) : (
        <ul className="space-y-1 max-h-72 overflow-y-auto text-xs font-mono">
          {filtered.map((sig) => (
            <SignalRow key={sig.id} sig={sig} />
          ))}
        </ul>
      )}

      {error && <div className="text-xs text-red-400">{error}</div>}
    </section>
  );
}

function FilterChip({
  current,
  value,
  label,
  onSelect,
}: {
  current: SignalFilter;
  value: SignalFilter;
  label: string;
  onSelect: (v: SignalFilter) => void;
}) {
  const active = current === value;
  return (
    <button
      type="button"
      onClick={() => onSelect(value)}
      className={`px-2 py-0.5 rounded uppercase tracking-wider text-[10px] ${
        active
          ? "bg-white/15 text-white"
          : "bg-white/[0.03] text-white/50 hover:text-white/80"
      }`}
    >
      {label}
    </button>
  );
}

function ConfidenceHistogram({ buckets }: { buckets: number[] }) {
  const max = Math.max(...buckets, 1);
  return (
    <div>
      <div className="flex justify-between text-[10px] opacity-50 mb-1">
        <span>0.0</span>
        <span>confidence distribution</span>
        <span>1.0</span>
      </div>
      <div className="h-12 flex items-end gap-[2px]">
        {buckets.map((count, i) => {
          const heightPct = (count / max) * 100;
          return (
            <div
              key={i}
              className="flex-1 rounded-sm bg-cyan-500/60"
              style={{ height: `${Math.max(heightPct, count > 0 ? 8 : 0)}%` }}
              title={`bucket ${(i / 10).toFixed(1)}-${((i + 1) / 10).toFixed(1)}: ${count}`}
            />
          );
        })}
      </div>
    </div>
  );
}

function SignalRow({ sig }: { sig: Signal }) {
  const time = new Date(sig.detectedAt).toLocaleTimeString();
  const magnitudePct = (Number.parseFloat(sig.magnitude) * 100).toFixed(3);
  const dirCls = sig.direction === "up" ? "text-emerald-400" : "text-red-400";
  const dirGlyph = sig.direction === "up" ? "↑" : "↓";
  const polyMid = sig.context?.polymarket?.yesMid;
  const polyMidStr = polyMid ? `${(Number.parseFloat(polyMid) * 100).toFixed(1)}%` : "—";

  return (
    <li className="grid grid-cols-[80px_36px_70px_60px_1fr] gap-2 py-1 border-b border-white/5">
      <span className="opacity-50">{time}</span>
      <span className={`text-base leading-none ${dirCls}`} aria-label={sig.direction}>
        {dirGlyph}
      </span>
      <span>{magnitudePct}%</span>
      <span className="opacity-80">{Number.parseFloat(sig.confidence).toFixed(2)}</span>
      <span className="opacity-50 truncate">
        poly YES {polyMidStr}
        {sig.actionTaken && (
          <span className="ml-2 px-1 rounded bg-white/5 text-[10px]">{sig.actionTaken}</span>
        )}
      </span>
    </li>
  );
}

function bucketByConfidence(signals: Signal[]): number[] {
  const buckets = new Array<number>(10).fill(0);
  for (const sig of signals) {
    const c = Number.parseFloat(sig.confidence);
    if (Number.isNaN(c)) continue;
    const idx = Math.min(Math.floor(c * 10), 9);
    buckets[idx]++;
  }
  return buckets;
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
