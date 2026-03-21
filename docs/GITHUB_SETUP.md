# GitHub Setup — xMilo

This file tells you exactly what to do to get the GitHub repo wired up so
the sidecar builds and publishes automatically.

---

## Step 1 — Create the GitHub repo

1. Go to https://github.com/new
2. Name it: `xmilo-releases` (must match what install.sh expects)
3. Owner: `xmilo-app` (or your actual GitHub username/org — update `GITHUB_OWNER` in `scripts/termux/bootstrap_sidecar.sh` if different)
4. Set to **Public** — install.sh fetches releases without auth
5. Do NOT initialize with README

---

## Step 2 — Push the code

From `C:\xMilo\xMilo_v14` in PowerShell:

```powershell
cd C:\xMilo\xMilo_v14
git init
git add .
git commit -m "xMilo v14 — sidecar bootstrap + entitlement loss + resume flow"
git branch -M main
git remote add origin https://github.com/YOUR_USERNAME/xmilo-releases.git
git push -u origin main
```

---

## Step 3 — Add Fly API token as a GitHub Secret

Needed for the relay auto-deploy workflow.

1. Get your Fly token:
   ```powershell
   flyctl auth token
   ```
2. Go to your GitHub repo → Settings → Secrets and variables → Actions → New repository secret
3. Name: `FLY_API_TOKEN`
4. Value: paste the token from step 1

---

## Step 4 — Publish the first sidecar release

Tag the commit to trigger the build:

```powershell
git tag v0.1.0
git push origin v0.1.0
```

This triggers `.github/workflows/release-sidecar.yml` which:
- Builds `picoclaw-arm64`, `picoclaw-arm`, `picoclaw-amd64`
- Generates `.sha256` checksum files for each
- Creates a GitHub Release at `v0.1.0` with all 6 files attached

Watch it run at: `https://github.com/YOUR_USERNAME/xmilo-releases/actions`

---

## Step 5 — Serve install.sh at xmiloatyourside.com/install.sh

The install.sh is at `scripts/termux/bootstrap_sidecar.sh`.

Options:
- Serve it from a Fly.io static site (simplest, same infra)
- Serve it from Cloudflare Pages (free, fast)
- Serve it from any web host

The file just needs to be accessible at `https://xmiloatyourside.com/install.sh`.

### Quickest option — Cloudflare Pages

1. Go to https://pages.cloudflare.com
2. Connect your GitHub repo
3. Build command: (none — static file)
4. Output directory: `scripts/termux`
5. Set custom domain: `xmiloatyourside.com`

Then rename `bootstrap_sidecar.sh` → `install.sh` in a `public/` folder,
or set up a redirect/rewrite rule.

---

## Step 6 — Verify the full flow

Once the release is published and install.sh is live:

```bash
# On an Android phone in Termux:
curl -fsSL https://xmiloatyourside.com/install.sh | bash
```

Expected output:
```
✓  Termux detected
✓  Termux:API detected
✓  Required tools available
✓  CPU architecture: arm64 (from aarch64)
✓  Latest release: v0.1.0
✓  Workspace ready: /data/data/com.termux/files/home/.miloclaw
→  Downloading picoclaw-arm64...
✓  Checksum verified
✓  Binary installed
✓  Config written
→  Starting xMilo sidecar...
✓  Sidecar is healthy at http://127.0.0.1:42817/health

xMilo is ready.
```

---

## Updating the sidecar in future

Every time you push a new tag, the binary rebuilds and publishes automatically:

```powershell
git add .
git commit -m "sidecar: fix XYZ"
git tag v0.1.1
git push origin main
git push origin v0.1.1
```

Users who re-run `install.sh` get the latest version automatically.

---

## Updating the relay

Every push to `main` that touches `relay-go/` auto-deploys to Fly:

```powershell
git add relay-go/
git commit -m "relay: fix ABC"
git push origin main
```

Or trigger manually from GitHub Actions UI.
