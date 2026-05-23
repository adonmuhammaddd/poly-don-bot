import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "Poly Don Bot",
  description: "Polymarket latency arbitrage observation dashboard",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className="antialiased min-h-screen flex flex-col">
        <header className="border-b border-white/10 px-6 py-3 flex items-center gap-6">
          <Link href="/" className="text-sm font-semibold tracking-wide">
            POLY <span className="opacity-50">·</span> DON BOT
          </Link>
          <nav className="flex items-center gap-4 text-xs opacity-70">
            <span className="px-2 py-0.5 rounded bg-amber-500/15 text-amber-300">
              Phase 1 · observation
            </span>
          </nav>
        </header>
        <div className="flex-1">{children}</div>
      </body>
    </html>
  );
}
