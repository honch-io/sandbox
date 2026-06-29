# Changelog

## [0.3.0](https://github.com/honch-io/sandbox/compare/v0.2.0...v0.3.0) (2026-06-29)


### Features

* **sandbox:** remote-dev helper to run the stack/harnesses on another box ([f1dcc82](https://github.com/honch-io/sandbox/commit/f1dcc824de1e1331043ae3403d4c01514d0f62ca))
* **sandbox:** remote-dev helper to run the stack/harnesses on another box ([2a22763](https://github.com/honch-io/sandbox/commit/2a22763db14714b0279b0f39770f347d21f175e9))


### Bug Fixes

* **ci:** opt release workflows into node 24 ([57f6ca3](https://github.com/honch-io/sandbox/commit/57f6ca38d4b0b5c681a3b8315b138a7a515b82b3))
* **sandbox:** run services from platform repo ([ba052b0](https://github.com/honch-io/sandbox/commit/ba052b0cc57e5ddc38428ac6aee2d056e6b60da1))
* **sandbox:** target remote docker host ([4703866](https://github.com/honch-io/sandbox/commit/4703866c5d706298955bea58b96d51e7f1d0b2f4))

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
