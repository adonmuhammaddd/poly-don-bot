import type { Metadata } from "next";
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
      <body className="antialiased">{children}</body>
    </html>
  );
}
