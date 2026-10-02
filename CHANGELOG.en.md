# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
This log tracks maintenance changes in this fork only; upstream releases are documented in [`docs/releases/`](docs/releases/).

English | [繁體中文](CHANGELOG.md)

## [Unreleased]

## [0.7.12-fork.1] - 2026-10-02

### Added
- Initialize SanHsien's Windows-first maintenance fork.
- Establish Traditional Chinese entry [`README.md`](README.md), keep original Simplified Chinese document as [`README.zh-CN.md`](README.zh-CN.md), and provide English mirror [`README.en.md`](README.en.md).
- Create AI maintenance guide as single source of truth [`AGENTS.md`](AGENTS.md).
- Create fork documentation [`FORK.md`](FORK.md) and attribution notice [`NOTICE.md`](NOTICE.md).
- Add `.cursor/rules/no-upstream-pr.mdc` guard rule to prevent accidental pull requests to upstream.
- Create Windows native development gate script `tools/dev_check.ps1`.
- Add upstream tracker `tools/check_upstream_updates.py` with baseline ledger `tools/upstream_baseline.json` pinned at `867da68`.
- Add dependency freshness checker `tools/check_dependency_freshness.py` and relative link validator `tools/check_links.py`.
- Add documentation: [`docs/UPSTREAM.md`](docs/UPSTREAM.md), [`docs/DECISIONS.md`](docs/DECISIONS.md), and [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).
- Add full repository risk review snapshot [`REVIEW.md`](REVIEW.md).
