import type { Metadata } from "next";
import Link from "next/link";
import { Bricolage_Grotesque, Instrument_Sans, JetBrains_Mono } from "next/font/google";
import "./globals.css";

const display = Bricolage_Grotesque({ subsets: ["latin"], variable: "--font-display", weight: ["600", "700"] });
const body = Instrument_Sans({ subsets: ["latin"], variable: "--font-body" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono" });

export const metadata: Metadata = {
  title: { default: "Tallgrass", template: "%s · Tallgrass" },
  description: "A companion for Pokémon romhacks on your own hardware: live party, encounters, run tracking and game docs.",
};

function Mark() {
  // Three blades of tall grass.
  return (
    <svg className="brand-mark" viewBox="0 0 24 24" aria-hidden="true">
      <path d="M5 22 C5 14 3 9 2 5 C6 9 8 15 8 22 Z" fill="var(--grass)" />
      <path d="M10 22 C10 13 11 7 12 2 C14 8 15 14 14 22 Z" fill="var(--grass)" />
      <path d="M16 22 C16 16 18 11 22 8 C21 13 19 17 19 22 Z" fill="var(--grass)" />
    </svg>
  );
}

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={`${display.variable} ${body.variable} ${mono.variable}`}>
      <body>
        <header className="site-header">
          <div className="inner">
            <Link href="/" className="brand"><Mark />Tallgrass</Link>
            <nav className="nav">
              <Link href="/games">Games</Link>
              <Link href="/link">Link a device</Link>
              <Link href="/account">Account</Link>
            </nav>
          </div>
        </header>
        {children}
        <footer className="footer">
          Tallgrass is a fan project, not affiliated with Nintendo, Game Freak or The Pokémon Company. Game data on this
          site is read from the games themselves; no game files are hosted here.
        </footer>
      </body>
    </html>
  );
}
