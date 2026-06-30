# Changelog

## [0.3.0](https://github.com/rshade/ax-go/compare/v0.2.0...v0.3.0) (2026-06-30)


### Added

* **ax:** add Guard and Perform dry-run side-effect guards ([#93](https://github.com/rshade/ax-go/issues/93)) ([a6f09c7](https://github.com/rshade/ax-go/commit/a6f09c7e0805a9e9d4f0b7d65d9aa52aaf400549)), closes [#13](https://github.com/rshade/ax-go/issues/13)
* **ci:** enforce per-package and repo-wide coverage floors ([#80](https://github.com/rshade/ax-go/issues/80)) ([7b09049](https://github.com/rshade/ax-go/commit/7b09049d652b3b7201cc122041a332a2f9fc2e30)), closes [#21](https://github.com/rshade/ax-go/issues/21)
* **error:** add retryable and retry_after_seconds recovery fields to… ([#95](https://github.com/rshade/ax-go/issues/95)) ([e707c68](https://github.com/rshade/ax-go/commit/e707c684e445fbe39fe84145c056a3ec0a767216))
* **mcp:** run any ax-go CLI as a live MCP server ([#89](https://github.com/rshade/ax-go/issues/89)) ([85bfc13](https://github.com/rshade/ax-go/commit/85bfc139bdc7c6a08eaa59acefbef92d905c2bff))


### Fixed

* **deps:** update astro monorepo ([#85](https://github.com/rshade/ax-go/issues/85)) ([d7fca4b](https://github.com/rshade/ax-go/commit/d7fca4bb307c34f1daf4e7107fc762a957a2b208))
* **deps:** update dependency @astrojs/starlight to ^0.41.0 ([#86](https://github.com/rshade/ax-go/issues/86)) ([7231fab](https://github.com/rshade/ax-go/commit/7231fab619459ddebfd8d752cddefbf7212cd35a))


### Documentation

* **readme:** add compatibility matrix and CONTRIBUTING guide ([9fab58f](https://github.com/rshade/ax-go/commit/9fab58f1f78deeb810536670c201c98bb5c01dcd)), closes [#23](https://github.com/rshade/ax-go/issues/23)

## [0.2.0](https://github.com/rshade/ax-go/compare/v0.1.0...v0.2.0) (2026-06-21)


### Added

* add import-isolated public contract packages for thin consumers ([#79](https://github.com/rshade/ax-go/issues/79)) ([05f0536](https://github.com/rshade/ax-go/commit/05f053618f385c716ffead6309c7b1b26665e3d9)), closes [#78](https://github.com/rshade/ax-go/issues/78)


### Fixed

* **deps:** update dependency sharp to ^0.35.0 ([#74](https://github.com/rshade/ax-go/issues/74)) ([4ed45b7](https://github.com/rshade/ax-go/commit/4ed45b77bb2c5d998562c019ca222bc9bdb3f9fd))


### Documentation

* bumping version ([15051a4](https://github.com/rshade/ax-go/commit/15051a47c5c13ba22ff4f9de022a33b9089b621e))
* **site:** scaffold Astro Starlight docs site consuming rshade-theme ([#72](https://github.com/rshade/ax-go/issues/72)) ([30d78b3](https://github.com/rshade/ax-go/commit/30d78b39d41812b9ddefdd4db7e4d012981a9ff3)), closes [#68](https://github.com/rshade/ax-go/issues/68)

## [0.1.0](https://github.com/rshade/ax-go/compare/v0.0.2...v0.1.0) (2026-06-19)


### Added

* **loki:** add opt-in Loki direct-push addon with cardinality ([#60](https://github.com/rshade/ax-go/issues/60)) ([0652f76](https://github.com/rshade/ax-go/commit/0652f76ea983cdc94e366b9832474e884e8ff609))


### Changed

* **telemetry:** dedupe sanitizer + mutex writer, simplify fail-open helpers ([#51](https://github.com/rshade/ax-go/issues/51)) ([238d425](https://github.com/rshade/ax-go/commit/238d425e4b88c745880d80d6fbc74351411f58f1)), closes [#45](https://github.com/rshade/ax-go/issues/45) [#46](https://github.com/rshade/ax-go/issues/46) [#47](https://github.com/rshade/ax-go/issues/47) [#48](https://github.com/rshade/ax-go/issues/48)


### Documentation

* **governance:** add stability & deprecation policy as constitution Principles XI–XII ([#61](https://github.com/rshade/ax-go/issues/61)) ([aa50d76](https://github.com/rshade/ax-go/commit/aa50d76187c9029d774b3cd8b4e72d8a8870d519)), closes [#17](https://github.com/rshade/ax-go/issues/17)

## [0.0.2](https://github.com/rshade/ax-go/compare/v0.0.1...v0.0.2) (2026-06-13)


### Added

* **config:** add Hujson patch API and freeze v0.1.0 output contracts ([#37](https://github.com/rshade/ax-go/issues/37)) ([43187ad](https://github.com/rshade/ax-go/commit/43187adef95de52a58e6cf805fae481d763305f3)), closes [#3](https://github.com/rshade/ax-go/issues/3) [#6](https://github.com/rshade/ax-go/issues/6) [#14](https://github.com/rshade/ax-go/issues/14)
* **telemetry:** add real OTLP export and command span lifecycle ([#49](https://github.com/rshade/ax-go/issues/49)) ([c28f86a](https://github.com/rshade/ax-go/commit/c28f86a9893a8d3b52e88eb28ced99a3af9e6b3b)), closes [#2](https://github.com/rshade/ax-go/issues/2)

## 0.0.1 (2026-06-11)


### Added

* **ax:** bootstrap the Agentic Experience foundation for Go CLIs ([4b2b85f](https://github.com/rshade/ax-go/commit/4b2b85fe347adf2d68c606b6a3a21c723ca9b50f))
* **config:** bound config reads at the read boundary (1 MiB cap) ([d9c2950](https://github.com/rshade/ax-go/commit/d9c29507fae6fce0223ed098b46bde7c9179a858))
* **config:** bound config reads at the read boundary (1 MiB cap) ([ea74c7d](https://github.com/rshade/ax-go/commit/ea74c7de4c66b642c5251b627c9e178fbe5c3380)), closes [#1](https://github.com/rshade/ax-go/issues/1)
* **version:** inject build-time version via -ldflags ([de97321](https://github.com/rshade/ax-go/commit/de9732168d960d77edc645ec3faa868c4e1d2165))
* **version:** inject build-time version via -ldflags ([dd42c87](https://github.com/rshade/ax-go/commit/dd42c872b3a2555ff260752c9fd0d5b4415371a0)), closes [#6](https://github.com/rshade/ax-go/issues/6)


### Documentation

* **constitution:** amend Principle VII for documentation-as-contract (v1.1.0) ([5e09602](https://github.com/rshade/ax-go/commit/5e09602becc96c0ae38c81347140546f439ba1ea))
* **constitution:** ratify v1.0.0 and wire ADR-to-spec retirement ([4d836dc](https://github.com/rshade/ax-go/commit/4d836dc261e503337f9b3e7829944b04c9476f29))
