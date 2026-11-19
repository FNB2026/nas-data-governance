# NDG official website

Canonical: https://ndg.fnbapp.net. Typo alias https://ngd.fnbapp.net issues a 308 redirect (path and query are preserved). Keep links and public release claims aligned with the root README, current immutable GitHub Releases and the website.

The site exists to present three things and nothing more: the **public product positioning** (local-first governance, human-in-control), the **public version entry points** (macOS Public Beta, read-only Linux Docker Preview, and the explicitly unshipped platforms), and the **security boundaries** (what the public builds cannot do). It is not a roadmap, a benchmark or a feature announcement channel.

Cloudflare Worker name: `ndg-official-site`. The root FNB website is independent and must remain unchanged.

## Editing

`index.html` holds product copy, release links and static markup. The header and footer use the original NDG **blue (#0071e3) tile with the full white `NDG` letters**, followed by an aligned two-line brand label. `style.css` contains the original light, Apple-inspired UI. `privacy.html` is a site-only privacy notice; `favicon.svg` uses the same three-letter logo and is not Apple's branding. `worker-template.mjs` implements routing and headers. `build.mjs` combines these files into one deployable Worker module without npm dependencies.

### Validate locally (Node.js 22)

```sh
node website/build.mjs
node --check website/dist/worker.mjs
node --test website/tests/site.test.mjs
```

The generated `website/dist/` is ignored. The live Worker currently also serves the legacy `ngd.` redirect, `/privacy`, `/style.css`, `/favicon.svg`, `/robots.txt`, `/sitemap.xml` and `/healthz`.

### Asset versioning and cache policy

`build.mjs` writes the first 10 hex characters of the SHA-256 of `style.css` and `favicon.svg` into every `/style.css?v=…` and `/favicon.svg?v=…` reference. HTML is served with `max-age=300`; hashed asset URLs with `max-age=31536000, immutable`; an unhashed asset URL only with `max-age=300, must-revalidate`.

Never hand-edit the `?v=` value, and never give an unversioned path a long cache. HTML expires in minutes while assets used to live for a day, so one CSS deploy could pair new markup with yesterday's stylesheet — that is exactly how the brand tile reverted to the old black square in a browser that had visited before the deploy. The test suite pins both the hash-to-content match and the two cache policies.

## GitHub Actions production deployment

PRs only build/test. Pushing merged changes under `website/**` or the workflow to `main` first runs tests then deploys to the existing Cloudflare Worker; manual runs from `main` also work. A failed gate never deploys.

**One-time secret required:** Create a Cloudflare API token with Workers Scripts Edit for the relevant Cloudflare account; add it to GitHub Actions secret `NDG_CLOUDFLARE_API_TOKEN`, preferably in GitHub environment `ndg-website-production`. Restrict the environment to the `main` branch and configure approval as appropriate. Do not store global API keys in source or logs. `NDG_CLOUDFLARE_ACCOUNT_ID` is nonsecret account metadata.

Once the environment secret has been configured, the next guarded `main` push triggers a deploy. **Do not claim production auto-deployment is verified until its actual GitHub Actions deploy job passes.** The Cloudflare account connector does not transfer its login token into GitHub Actions.

## Release accuracy and smoke testing

Maintain the released-channel truth of NDG's root README and GitHub Releases. Never describe fnOS candidates or future governance work as shipped. Docker is read-only; permanent purge is not enabled in public releases.

After deployment verify externally: canonical HTTPS `/`, CSS, privacy, 308 alias, responsive display, health, and live download links. A Cloudflare API success response alone is not a full smoke-test PASS.
