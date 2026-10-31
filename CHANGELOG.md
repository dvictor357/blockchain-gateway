# Changelog

## [0.1.0] - 2026-11-01

Initial portfolio release: a tiny, read-only, agent-friendly chain API, not a replacement for eRPC.

### Added

- MIT license, copyright Dimas Victor, 2026.
- GitHub Actions build, vet, and race tests with PostgreSQL 16 integration coverage, plus a README badge.
- Optional shared API-key authentication through `X-API-Key`; `/health` remains public.
- Exact, configurable RPC method allowlist, read-only by default. Single requests, whole batches, and REST-helper dependencies are checked before upstream work.
- Focused safety coverage for authentication, method rejection, batch atomic rejection, and explicit method opt-in.

### Changed

- PostgreSQL storage and the CoinGecko fetcher are disabled by default. A plain container can start without a database; disabled market data returns HTTP 503 with `MARKET_DATA_DISABLED`.
- Database mode preserves migrations and stored market queries. Fetching is a separate opt-in and requires database mode; only one instance per database should fetch.
- Blaze updated to `main` commit `e0a0000a93d8` (`v0.0.0-20261031064110-e0a0000a93d8`); routes migrated to its standard-library pattern syntax.
- README, chain guide, development environment example, and API reference aligned with eight EVM adapters plus Bitcoin: seven EVM chains and Bitcoin enabled by default, Fantom opt-in.
- README focused on quickstart, curl examples, deployment limits, safe configuration, and customization contact.

### Removed

- Stale project-management documents under `docs/templates/`.

### Security and migration notes

- Raw RPC forwarding is no longer unrestricted. `RPC_METHOD_ALLOWLIST` replaces the defaults with exact, case-sensitive method names; there are no wildcard grants. Explicitly permitting write or privileged methods removes the read-only guarantee for those methods.
- API keys with leading or trailing whitespace fail startup explicitly rather than leaving every caller unauthorized. With authentication enabled, Swagger is protected too; ordinary browser navigation needs a trusted header-injecting client/proxy.
- Set `DATABASE_ENABLED=true` to retain database-backed markets. Set `MARKET_FETCHER_ENABLED=true` on one instance to fetch CoinGecko data as well. Compose database profiles enable storage explicitly, but fetching remains opt-in.
- Local paths in the unmerged `dev` branch's investigation transcript were replaced with repository-relative references in a new commit. Historical content remains; no history was rewritten.

[0.1.0]: https://github.com/dvictor357/blockchain-gateway/releases/tag/v0.1.0
