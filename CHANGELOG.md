# Changelog

## [0.4.0](https://github.com/rshade/ax-go/compare/v0.3.0...v0.4.0) (2026-07-24)


### Added

* **ci:** enforce performance regression budget in CI ([#107](https://github.com/rshade/ax-go/issues/107)) ([77af8f4](https://github.com/rshade/ax-go/commit/77af8f44b2f19cb2a171d7a59b396d8917cae0e9)), closes [#22](https://github.com/rshade/ax-go/issues/22)
* **logging:** add import-isolated logging package over internal/logcore ([#151](https://github.com/rshade/ax-go/issues/151)) ([93dd453](https://github.com/rshade/ax-go/commit/93dd4531b4db72df2e282ff3a6b7003f35617f35)), closes [#144](https://github.com/rshade/ax-go/issues/144)
* **schema:** enumerate non-deterministic fields per command output ([808b612](https://github.com/rshade/ax-go/commit/808b612580285fde15e1229b72952cf65ba1140b)), closes [#16](https://github.com/rshade/ax-go/issues/16)
* **schema:** enumerate non-deterministic fields per command output ([#146](https://github.com/rshade/ax-go/issues/146)) ([808b612](https://github.com/rshade/ax-go/commit/808b612580285fde15e1229b72952cf65ba1140b))
* **surfacecheck:** gate the root public API surface against a reviewed baseline ([#148](https://github.com/rshade/ax-go/issues/148)) ([99eda4a](https://github.com/rshade/ax-go/commit/99eda4a53b0786b90bc59bd5d1fe34b08a58b913)), closes [#18](https://github.com/rshade/ax-go/issues/18)
* **telemetry:** make OTLP export and gRPC dial independently opt-out… ([#150](https://github.com/rshade/ax-go/issues/150)) ([6e8f38a](https://github.com/rshade/ax-go/commit/6e8f38ac47f09c5c1dd9053cd7bac7b6520158c2))


### Fixed

* **deps:** update github.com/tailscale/hujson digest to 10d7940 ([#116](https://github.com/rshade/ax-go/issues/116)) ([6883ed3](https://github.com/rshade/ax-go/commit/6883ed3b257babc744e64f8aa58b430a0831f784))
* **deps:** update github.com/tailscale/hujson digest to 78b5b16 ([#141](https://github.com/rshade/ax-go/issues/141)) ([62b4beb](https://github.com/rshade/ax-go/commit/62b4beb0dc051cbf8cb486bfc906c0b0d075c08b))
* **deps:** update golang.org/x/perf digest to 82a0b07 ([#108](https://github.com/rshade/ax-go/issues/108)) ([0a9aab0](https://github.com/rshade/ax-go/commit/0a9aab05f8a1e319b9e55f16fa10e193aa402fec))
* **deps:** update module go.opentelemetry.io/proto/otlp to v1.11.0 ([#147](https://github.com/rshade/ax-go/issues/147)) ([d53e4f7](https://github.com/rshade/ax-go/commit/d53e4f7cbeb2370a14e6671d27c0dda8a09fd590))
* **deps:** update module google.golang.org/grpc to v1.82.0 ([#97](https://github.com/rshade/ax-go/issues/97)) ([b6744c8](https://github.com/rshade/ax-go/commit/b6744c8f95e4044fb0c0c9dda75e0cf78325c091))
* **deps:** update module google.golang.org/grpc to v1.82.1 ([#114](https://github.com/rshade/ax-go/issues/114)) ([6658ce8](https://github.com/rshade/ax-go/commit/6658ce80c37fd0f2cb26d3b4cdd7bb8e2b70821d))
* kimi evaluation fixes ([#117](https://github.com/rshade/ax-go/issues/117)) ([60fa703](https://github.com/rshade/ax-go/commit/60fa703238ad46da23fe24887f1863e598299b44))


### Documentation

* add Diátaxis tutorial, how-to, and explanation pages ([#98](https://github.com/rshade/ax-go/issues/98)) ([5bf703a](https://github.com/rshade/ax-go/commit/5bf703a7c8bf040ad3bdf60e05544b860cd35642))
* add SECURITY.md vulnerability disclosure policy ([fc8f87d](https://github.com/rshade/ax-go/commit/fc8f87d3078a3641a34c7b4c04d6f3d9ce55cbf6)), closes [#19](https://github.com/rshade/ax-go/issues/19)
* **contract:** document that import-isolated packages carry no live … ([#149](https://github.com/rshade/ax-go/issues/149)) ([31b1ea5](https://github.com/rshade/ax-go/commit/31b1ea58809762a6df243ba5829cd78b515c4a5e)), closes [#145](https://github.com/rshade/ax-go/issues/145)

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
