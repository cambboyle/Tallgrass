import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Guide",
  description: "Install Tallgrass, set up your game, and get the most out of the dashboard.",
};

const sections = [
  ["install", "Install"],
  ["setup", "Set up your game"],
  ["dashboard", "The dashboard"],
  ["pin", "Lock it with a PIN"],
  ["editing", "Edit saves on your computer"],
  ["account", "Link to an account"],
  ["updates", "Updates"],
  ["troubleshooting", "Troubleshooting"],
  ["uninstall", "Uninstall"],
] as const;

function Cmd({ children }: { children: string }) {
  return <pre className="install"><code>{children}</code></pre>;
}

export default function Guide() {
  return (
    <main className="guide">
      <section className="stack" style={{ paddingTop: 24 }}>
        <h1>Guide</h1>
        <p className="lead">
          Tallgrass runs on the same device as your emulator and reads your game while you play. You need your own copy of
          the game, already playable; Tallgrass never downloads or shares game files.
        </p>
        <nav className="chips" aria-label="On this page">
          {sections.map(([id, title]) => <a key={id} className="chip" href={`#${id}`}>{title}</a>)}
        </nav>
      </section>

      <section className="card stack" id="install">
        <h2>Install</h2>
        <p>
          Tallgrass is built for a Raspberry Pi running RetroPie, and also runs on other Linux PCs (64-bit Arm and x86).
          Open a terminal on the device (or SSH in, e.g. <code>ssh pi@retropie.local</code>) and run:
        </p>
        <Cmd>curl -fsSL https://tallgrass-delta.vercel.app/install.sh | bash</Cmd>
        <p>
          The installer downloads the latest release for your device, checks it against the release&apos;s checksums, puts
          it in <code>~/tallgrass</code> and starts it as a service, so it runs whenever the device is on. It asks for your
          password once (to set up the service). When it finishes it prints an address like{" "}
          <code>http://192.168.0.20:8080/setup.html</code>: open that on your phone or computer, on the same network.
        </p>
        <p className="muted small">
          Tallgrass runs at low priority, so the emulator always comes first. To install a specific version, run the
          command with <code>TALLGRASS_VERSION=0.1.1</code> in front of <code>bash</code>.
        </p>
      </section>

      <section className="card stack" id="setup">
        <h2>Set up your game</h2>
        <p>
          The setup page asks which game you&apos;re playing, then checks everything that game needs. It fixes what it safely
          can with one button, and tells you what to do for the rest. It re-checks every few seconds, so you can follow
          along with the game running.
        </p>
        <div className="grid">
          <div className="stack-sm">
            <h3>Radical Red and Unbound (GBA)</h3>
            <ul>
              <li>Your ROM, found in <code>roms/gba</code> and matched by its fingerprint (any file name works).</li>
              <li>The <strong>lr-mgba</strong> emulator core, which lets other programs read game memory.</li>
              <li>RetroArch&apos;s <strong>network commands</strong>, which is how Tallgrass asks for memory. They only listen while a game runs.</li>
              <li><strong>Autosave every minute</strong>, so your save file on disk stays current and gets backed up.</li>
              <li>Your save, including converting a <code>.sav</code> from another emulator.</li>
              <li>A live read: start the game and load your save.</li>
            </ul>
          </div>
          <div className="stack-sm">
            <h3>HeartGold Generations (DS)</h3>
            <ul>
              <li>Your ROM, in <code>roms/nds</code>.</li>
              <li><strong>DraStic</strong>, the DS emulator from RetroPie-Setup&apos;s experimental packages.</li>
              <li>Permission for Tallgrass to read DraStic&apos;s memory (the default on Raspberry Pi OS).</li>
              <li>Your save in DraStic&apos;s save folder.</li>
              <li>A live read: start the game and load your save.</li>
            </ul>
          </div>
        </div>
        <p className="muted small">
          Before changing an emulator config file, Tallgrass keeps the original next to it as{" "}
          <code>&lt;file&gt;.bak-tallgrass</code>. It won&apos;t change configs while a game is running, because the emulator
          could overwrite them when it closes.
        </p>
      </section>

      <section className="card stack" id="dashboard">
        <h2>The dashboard</h2>
        <dl className="defs">
          <dt>Party</dt>
          <dd>
            Your team as it is right now: level, HP, nature, ability, IVs, EVs and moves. Below it, the route you&apos;re
            on, with what you can catch there by time of day (marked seen or caught), plus the items, shops and trainers on
            that map. Then your journey: catches, faints,
            evolutions and new areas, logged as they happen. Turn on <strong>Nuzlocke mode</strong> to see it as a run,
            with first-encounter routes and deaths.
          </dd>
          <dt>Boxes</dt>
          <dd>Every PC box, read from the running game every 15 seconds, or from any save file or backup.</dd>
          <dt>Trainers</dt>
          <dd>Every trainer in the game with their team, held items and moves, in the order you&apos;ll meet them. Search by trainer, area or Pokémon.</dd>
          <dt>Saves</dt>
          <dd>
            Every change to your save file is kept as a version. Download any version, or restore it: Tallgrass backs up the
            current save first, and refuses while a game is running (the emulator would overwrite it on exit). To edit a
            save on your computer, download the latest version, edit it, and use <strong>Upload edited save</strong>; it&apos;s
            checked and the current save is backed up first. Or let <a href="#editing">tallgrass sync</a> do it for you.
          </dd>
          <dt>Library</dt>
          <dd>Your ROMs by system, with whether Tallgrass supports each one and which have saves.</dd>
        </dl>
      </section>

      <section className="card stack" id="pin">
        <h2>Lock it with a PIN</h2>
        <p>
          Anyone on your network can open the dashboard, including the Saves page. To stop that, set a PIN under{" "}
          <strong>Account → Dashboard PIN</strong> (4 to 12 digits). Each browser then signs in once and stays signed in
          for 30 days. Changing or removing the PIN signs out every browser.
        </p>
        <p className="muted small">
          Forgot it? Delete <code>~/tallgrass/auth.json</code> on the device and restart Tallgrass (
          <code>sudo systemctl restart tallgrass</code>); the dashboard is open again.
        </p>
      </section>

      <section className="card stack" id="editing">
        <h2>Edit saves on your computer</h2>
        <p>
          To move Pokémon between games with <a href="https://github.com/andrewbenington/OpenHome">OpenHome</a> (or edit
          them with PKHeX), run <code>tallgrass sync</code> on your computer. It keeps a folder there in step with your
          device&apos;s saves: new progress on the device is copied down within seconds, and when you save in OpenHome the
          file is uploaded back. The device checks it, backs up the current save first, and waits while that game is
          running.
        </p>
        <p>On a Mac (Apple silicon; use <code>darwin-amd64</code> for Intel), in Terminal:</p>
        <Cmd>{`curl -fsSLo tallgrass https://github.com/cambboyle/Tallgrass/releases/latest/download/tallgrass-darwin-arm64
chmod +x tallgrass
./tallgrass sync 192.168.0.20`}</Cmd>
        <p>
          On Windows, download <code>tallgrass-windows-amd64.exe</code> from the{" "}
          <a href="https://github.com/cambboyle/Tallgrass/releases/latest">latest release</a> and run{" "}
          <code>tallgrass-windows-amd64.exe sync 192.168.0.20</code> in a terminal. Use your device&apos;s address (the one
          you open the dashboard on). If the dashboard has a PIN, it asks for it once.
        </p>
        <p>
          Saves land in a <strong>Tallgrass Saves</strong> folder in your home folder (<code>-dir</code> picks another).
          In OpenHome&apos;s saves window, add that folder under <strong>Save Folders</strong> and your saves appear under{" "}
          <strong>Suggested</strong>. Keep sync running while you edit.
        </p>
        <p className="muted small">
          Close the game on the device before editing, so you edit its latest save. If a save changes on both sides
          anyway, the device&apos;s copy wins and yours is kept in <code>Conflicts</code>. Every upload is backed up on the
          device first, so nothing is lost either way.
        </p>
      </section>

      <section className="card stack" id="account">
        <h2>Link to an account</h2>
        <p>
          Everything above works without an account. Linking a device to a Tallgrass account is optional; it lets you
          switch on <strong>Cloud backups</strong> (Account page), which copies your save backups and Journey history to
          your account every 10 minutes, to download from the website if the device is lost. On the dashboard&apos;s <strong>Account</strong> page, choose{" "}
          <strong>Link this device</strong>; it shows a short code. <Link href="/link">Enter that code here</Link> while
          signed in, and the device is linked. You can unlink devices from your <Link href="/account">account page</Link>.
        </p>
      </section>

      <section className="card stack" id="updates">
        <h2>Updates</h2>
        <p>
          Tallgrass checks for a new release every few hours and shows it under <strong>Account → Updates</strong>, with
          what changed. Choose <strong>Install</strong> and it restarts on the new version within a few seconds.
          Every release is signed, and an update is only installed if its signature and checksum match.
        </p>
      </section>

      <section className="card stack" id="troubleshooting">
        <h2>Troubleshooting</h2>
        <dl className="defs">
          <dt>The page doesn&apos;t load</dt>
          <dd>
            Check the device&apos;s address (<code>hostname -I</code> on the device) and that your phone is on the same
            network. <code>systemctl status tallgrass</code> shows whether it&apos;s running, and{" "}
            <code>journalctl -u tallgrass -n 50</code> shows its recent log.
          </dd>
          <dt>&ldquo;Not supported&rdquo; for my game</dt>
          <dd>
            Tallgrass recognises games by their ROM fingerprint (SHA-1, shown on the Party page). A different version of a
            hack has a different fingerprint. The versions on the <Link href="/games">games page</Link> are fully
            supported; other FireRed hacks often work through auto-detection. To add a game yourself, see{" "}
            <a href="https://github.com/cambboyle/Tallgrass/blob/main/docs/writing-a-profile.md">writing a game profile</a>.
          </dd>
          <dt>The party doesn&apos;t show, or stops updating</dt>
          <dd>
            Open the setup page: its live check says what&apos;s missing. For GBA games, network commands only take effect
            after restarting the game. For DS games, give DraStic a few seconds after loading your save.
          </dd>
          <dt>Backups miss my latest progress</dt>
          <dd>
            Games write the save file when you save in-game, and RetroArch writes it to disk at most once a minute (with
            the setup page&apos;s autosave fix) or when you quit. Save in-game before relying on a backup.
          </dd>
        </dl>
      </section>

      <section className="card stack" id="uninstall">
        <h2>Uninstall</h2>
        <Cmd>{`sudo systemctl disable --now tallgrass
sudo rm /etc/systemd/system/tallgrass.service
rm -r ~/tallgrass`}</Cmd>
        <p className="muted small">
          <code>~/tallgrass</code> also holds your save backups (<code>backups/</code>) and run history (
          <code>tallgrass.db</code>): copy them somewhere first if you want to keep them. Config changes made by the setup
          page have their originals saved as <code>.bak-tallgrass</code> files.
        </p>
      </section>
    </main>
  );
}
