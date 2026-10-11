# Releasing Tallgrass

Releases are built by GitHub Actions (`.github/workflows/release.yml`) when a version tag is pushed.

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow tests, builds `tallgrass-linux-armv7` (Raspberry Pi, 32-bit OS), `tallgrass-linux-arm64` (64-bit Pi OS), `tallgrass-linux-amd64` (PCs, Steam Deck desktop) and macOS/Windows builds (`darwin-arm64`, `darwin-amd64`, `windows-amd64.exe`, for `tallgrass sync`), writes `SHA256SUMS`, signs it as `SHA256SUMS.sig`, and publishes a GitHub release with notes generated from the commits.

## The signing key

- Installed agents update themselves only from releases whose `SHA256SUMS` is signed by the release key. Its public half is `update.PublicKey`.
- The private key is `~/.config/tallgrass/release.key` on Cam's Mac (made with `go run ./cmd/sign -genkey`). **Back it up somewhere safe** (a password manager). If it's lost, agents can't verify new releases until they're reinstalled with a new key.
- GitHub needs it as a repository secret named `RELEASE_SIGNING_KEY` (Settings → Secrets and variables → Actions → New repository secret), with the file's contents as the value.
- Never commit it or paste it into chat.

## Versions

- `X.Y.Z`. The agent offers an update when the latest release's version is higher than its own.
- Builds from `make deploy` are `dev-<commit>` and never offer updates.

## Installing

`web/public/install.sh` is served by the website (`curl -fsSL https://tallgrass-delta.vercel.app/install.sh | bash`) and attached to each release. It downloads from GitHub releases, so **the repository has to be public** (or releases hosted somewhere public) for installs and updates to work for anyone else.
