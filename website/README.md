# NDG official website

Canonical: https://ndg.fnbapp.net. Typo alias https://ngd.fnbapp.net issues a 308 redirect.

Cloudflare Worker name: `ndg-official-site`. The root FNB website is independent and must remain unchanged.

## Editing

`index.html` holds product copy, release links, and static markup. `style.css` contains the original light, Apple-inspired UI. `privacy.html` is a site-only privacy notice; `favicon.svg` is not Apple's branding. `worker-template.mjs` implements routing and headers. `build.mjs` combines these files into one deployable Worker module without npm dependencies.

### Validate locally (Node.js 22)

```sh
node website/build.mjs
node --check website/dist/worker.mjs
node --test website/tests/site.test.mjs
```

The generated `website/dist/` is ignored. The live Worker currently also serves the legacy `ngd.` redirect, `/privacy`, `/style.css`, `/favicon.svg`, `/robots.txt`, `/sitemap.xml` and `/healthz`.

## GitHub Actions production deployment

PRs only build/test. Pushing merged changes under `website/**` or the workflow to `main` first runs tests then deploys to the existing Cloudflare Worker; manual runs from `main` also work. A failed gate never deploys.

**One-time secret required:** Create a Cloudflare API token with Workers Scripts Edit for the relevant Cloudflare account; add it to GitHub Actions secret `NDG_CLOUDFLARE_API_TOKEN`, preferably in GitHub environment `ndg-website-production`. Restrict the environment to the `main` branch and configure approval as appropriate. Do not store global API keys in source or logs. `NDG_CLOUDFLARE_ACCOUNT_ID` is nonsecret account metadata.

Until the secret and environment exist, **automatic deployment is configured but not operational**. The Cloudflare account connector does not transfer its login token into GitHub Actions.

## Release accuracy and smoke testing

Maintain the released-channel truth of NDG's root README and GitHub Releases. Never describe fnOS candidates or future governance work as shipped. Docker is read-only; permanent purge is not enabled in public releases.

After deployment verify externally: canonical HTTPS `/`, CSS, privacy, 308 alias, responsive display, health, and live download links. A Cloudflare API success response alone is not a full smoke-test PASS.
