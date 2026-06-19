# Changelog

## [0.2.1](https://github.com/honch-io/sandbox/compare/v0.2.0...v0.2.1) (2026-06-19)


### Bug Fixes

* **ci:** opt release workflows into node 24 ([57f6ca3](https://github.com/honch-io/sandbox/commit/57f6ca38d4b0b5c681a3b8315b138a7a515b82b3))
* **sandbox:** run services from platform repo ([ba052b0](https://github.com/honch-io/sandbox/commit/ba052b0cc57e5ddc38428ac6aee2d056e6b60da1))

## [0.2.0](https://github.com/honch-io/sandbox/releases/tag/v0.2.0) (2026-05-27)

### Features

- run canonical SDK on ESP32 hardware

### Bug Fixes

- default empty proxy bind to loopback
- format bind address with net.JoinHostPort
- preserve session state on load failure
- remember dismissed first run
- remove malformed pid files on stop
- report online proxy as healthy
- run stop commands after background errors
- wait for sandbox background ports to close
