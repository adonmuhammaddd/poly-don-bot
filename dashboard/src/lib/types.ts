// Mirror Go DTOs in internal/api/types.go. Keep in sync.

export interface PriceTick {
  exchange: string;
  symbol: string;
  price: string;
  tsExchange: string;
  tsReceived: string;
}

export interface CurrentMarket {
  marketId: string;
  question: string;
  lastSeen: string;
}

export interface LatestBook {
  marketId: string;
  yesBid?: string;
  yesAsk?: string;
  noBid?: string;
  noAsk?: string;
  yesUpdated: string;
  noUpdated: string;
  mid?: string;
}

export interface LatencyStats {
  count: number;
  avgMs: number;
  p50Ms: number;
  p95Ms: number;
  lastDeltaMs: number;
  windowSecs: number;
  pendingMoves: number;
}

export interface LatencyMeasurement {
  binanceMoveAt: string;
  polymarketReprice: string;
  deltaMs: number;
}

export interface LatencyResponse {
  stats: LatencyStats;
  samples: LatencyMeasurement[];
}
