"use client";

import { useEffect, useState } from "react";
import { apiClient } from "@/lib/api-client";
import { useSSE } from "@/lib/sse";
import type { CurrentMarket, LatestBook, PriceTick } from "@/lib/types";

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
  const book = useSSE<LatestBook>(marketId ? `/api/stream/book?marketId=${encodeURIComponent(marketId)}` : null);

  return (
    <main className="p-6 max-w-6xl mx-auto">
      <div className="grid gap-4 md:grid-cols-2">
        <BinanceCard
          tick={tick.data}
          state={tick.state}
          error={tick.error}
        />
        <PolymarketCard
          question={marketQuestion}
          book={book.data}
          state={book.state}
          error={book.error}
        />
      </div>
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
        <h2 className="text-sm font-semibold tracking-wide opacity-70">BINANCE · BTC/USDT</h2>
        <ConnectionPill state={state} />
      </header>
      <div className="font-mono text-3xl">
        {tick ? formatUSD(tick.price) : "—"}
      </div>
      <div className="text-xs opacity-50">
        {tick ? new Date(tick.tsExchange).toLocaleTimeString() : "waiting for first tick…"}
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
  const yesMid = book?.mid;
  const yesBid = book?.yesBid;
  const yesAsk = book?.yesAsk;
  const noBid = book?.noBid;
  const noAsk = book?.noAsk;

  return (
    <section className="rounded-lg border border-white/10 bg-white/[0.02] p-5 space-y-3">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wide opacity-70">POLYMARKET · 5M BTC</h2>
        <ConnectionPill state={state} />
      </header>
      <div className="font-mono text-3xl">
        {yesMid ? formatProb(yesMid) : "—"}
      </div>
      <div className="text-xs opacity-50 truncate" title={question}>
        {question || "discovering market…"}
      </div>
      <div className="grid grid-cols-2 gap-2 text-xs font-mono pt-1">
        <Stat label="YES bid" value={yesBid} />
        <Stat label="YES ask" value={yesAsk} />
        <Stat label="NO bid" value={noBid} />
        <Stat label="NO ask" value={noAsk} />
      </div>
      {error && <div className="text-xs text-red-400">{error}</div>}
    </section>
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

function ConnectionPill({ state }: { state: string }) {
  const cls =
    state === "open"
      ? "bg-emerald-500/20 text-emerald-300"
      : state === "connecting"
        ? "bg-amber-500/20 text-amber-300"
        : "bg-red-500/20 text-red-300";
  return (
    <span className={`text-[10px] uppercase tracking-wider px-2 py-0.5 rounded ${cls}`}>
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
