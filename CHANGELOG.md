# Changelog

## [1.1.1](https://github.com/XxxXTeam/Qwen2API_Go/compare/v1.1.0...v1.1.1) (2026-07-03)


### Bug Fixes

* align docker go version with go.mod ([a106f88](https://github.com/XxxXTeam/Qwen2API_Go/commit/a106f889bdb3a76c7534c4cc05d361a244af9b4a))

## [1.1.0](https://github.com/XxxXTeam/Qwen2API_Go/compare/v1.0.0...v1.1.0) (2026-07-03)


### Features

* capture browser sessions for accounts and automate releases ([945c9af](https://github.com/XxxXTeam/Qwen2API_Go/commit/945c9afc25995e6bda02d62eaf5a780411d0a940))


### Bug Fixes

* preserve string tool arguments for write/edit ([ec5d68c](https://github.com/XxxXTeam/Qwen2API_Go/commit/ec5d68c87ccdcb0d164e10276740fae9aba44a33))
* preserve string tool arguments for write/edit ([05c9208](https://github.com/XxxXTeam/Qwen2API_Go/commit/05c92083a3344e7bd253572483b7b23fd78e33e0))
* stabilize guest and account chat sessions ([c566871](https://github.com/XxxXTeam/Qwen2API_Go/commit/c56687131141d93dda1e2d212ac57100ec148095))
* toolcall result ([37b2d04](https://github.com/XxxXTeam/Qwen2API_Go/commit/37b2d04b1e2624c646b5646fcaf3d040ff4e28e0))
* 优化Redis链接 ([de0146a](https://github.com/XxxXTeam/Qwen2API_Go/commit/de0146ac74d26382984a275e3c9ff0b701f34386))
* 修复亿点点小Buuuuuuuug ([f884469](https://github.com/XxxXTeam/Qwen2API_Go/commit/f884469601c6922d8cb5414874f71793c3e8960c))
* 错误修复 ([3565d89](https://github.com/XxxXTeam/Qwen2API_Go/commit/3565d89f637fcc28cc9e0d7f63f1dfde390d58e4))

## [1.0.0](https://github.com/XxxXTeam/Qwen2API_Go/compare/v0.6.0...v1.0.0) (2026-05-22)


### ⚠ BREAKING CHANGES

* 移除了 IPC 传输方式，现在只支持远程服务模式。 移除了 LINGMA_BACKEND, LINGMA_TRANSPORT, LINGMA_PIPE, LINGMA_WEBSOCKET_URL, LINGMA_SESSION_MODE 环境变量。

### Features

* **admin:** 添加模型列表手动刷新功能 ([2087bc8](https://github.com/XxxXTeam/Qwen2API_Go/commit/2087bc82516e53d2f7f4637e523ff9b9aeef407f))
* **toolemulation:** 支持解析嵌套JSON参数字符串 ([52c4e13](https://github.com/XxxXTeam/Qwen2API_Go/commit/52c4e133e2284676a8ea761664aafe8212703444))
* 优化前端 ([6b6e902](https://github.com/XxxXTeam/Qwen2API_Go/commit/6b6e902fa02d93646e44b8e1868eb5ede9d9e50b))
* 支持 Lingma 远程服务并优化配置 ([c6b3157](https://github.com/XxxXTeam/Qwen2API_Go/commit/c6b31578d0c52e4d76487ea2811b83d0628b0d9d))
* 添加提示词管理和自定义功能 ([de3cb00](https://github.com/XxxXTeam/Qwen2API_Go/commit/de3cb00e093a6cc88f6d9c43e37ca7f8a289691b))


### Bug Fixes

* emit thinking content as reasoning_content field ([a17629a](https://github.com/XxxXTeam/Qwen2API_Go/commit/a17629ab4ae9e7ec18ade5e668af891cd4080897))

## [0.6.0](https://github.com/XxxXTeam/Qwen2API_Go/compare/v0.5.0...v0.6.0) (2026-05-10)


### Features

* boom 重构前端 ([34a3011](https://github.com/XxxXTeam/Qwen2API_Go/commit/34a30112342514e75b9c98083bdb8b44b719d974))
* del chat ([0cc14f6](https://github.com/XxxXTeam/Qwen2API_Go/commit/0cc14f6d416d7f1b8e1325d265b50c7370844b14))
* **openai:** 支持 Anthropic API 的新字段和功能 ([da36476](https://github.com/XxxXTeam/Qwen2API_Go/commit/da36476993558b107502c2586c80cdc3b1f577d3))
* **qwen:** 添加阿里云人机验证错误处理机制 ([69210c3](https://github.com/XxxXTeam/Qwen2API_Go/commit/69210c3865deea7d7e6e62cef70288e39880ef05))
* reasoning_effort 兼容 ([231cb26](https://github.com/XxxXTeam/Qwen2API_Go/commit/231cb26d43f22ac60289c8bd11a20d9117f99a41))
* web 端 AI生图/视频 / 修复 Video 模型 ([a2f9eb3](https://github.com/XxxXTeam/Qwen2API_Go/commit/a2f9eb3d145a17e0277f19c2428d33dd6619379f))
* 无独有偶,我已放弃 ([be003b8](https://github.com/XxxXTeam/Qwen2API_Go/commit/be003b8aa5711764a787fc59746d8db59146515e))


### Bug Fixes

* README ([993037e](https://github.com/XxxXTeam/Qwen2API_Go/commit/993037efcc1273c033722ce700282ee9ac57b7b5))

## [0.5.0](https://github.com/XxxXTeam/Qwen2API_Go/compare/v0.4.0...v0.5.0) (2026-04-30)


### Features

* add runtime config hot reload ([8f764e8](https://github.com/XxxXTeam/Qwen2API_Go/commit/8f764e895e1c9fb2adaed8c480418bc24332d01d))


### Bug Fixes

* toolcall and rm sb huawei ([6e579f8](https://github.com/XxxXTeam/Qwen2API_Go/commit/6e579f8a02708511e11522962f9edf7d90133056))
* use env guards for docker workflow secrets ([fe6921c](https://github.com/XxxXTeam/Qwen2API_Go/commit/fe6921c92508ebf564d76fbefa5eb95cc20a13c9))

## [0.4.0](https://github.com/XxxXTeam/Qwen2API_Go/compare/v0.3.0...v0.4.0) (2026-04-27)


### Features

* fix token stats and add docker release workflow ([63f244c](https://github.com/XxxXTeam/Qwen2API_Go/commit/63f244cc48af604a731862bd1485fc4533e75103))

## [0.3.0](https://github.com/XxxXTeam/Qwen2API_Go/compare/v0.2.0...v0.3.0) (2026-04-26)


### Features

* 每个模型token统计 ([c3c71d1](https://github.com/XxxXTeam/Qwen2API_Go/commit/c3c71d17733f81bab23847cef2c358a93269d72a))


### Bug Fixes

* token计算,优化webui样式 ([655cd6e](https://github.com/XxxXTeam/Qwen2API_Go/commit/655cd6ed559745b787ec41e61b0bedf9c288eb0b))

## [0.2.0](https://github.com/XxxXTeam/Qwen2API_Go/compare/v0.1.0...v0.2.0) (2026-04-26)


### Features

* Fast/Thinking 模型 ([a750ef1](https://github.com/XxxXTeam/Qwen2API_Go/commit/a750ef1d78d31cdef16ff5c01d242525df0ce102))
* xx ([31de88b](https://github.com/XxxXTeam/Qwen2API_Go/commit/31de88b6db572369d57c84b1ed2a715ae27af29e))
* 上下文支持 ([07864bd](https://github.com/XxxXTeam/Qwen2API_Go/commit/07864bdbf9613924522886bfd2d7d036cc71a9cf))


### Bug Fixes

* q ([29d2f4b](https://github.com/XxxXTeam/Qwen2API_Go/commit/29d2f4b657c99621f12225709b498c54f031ce76))
* tool ([8a7fcf1](https://github.com/XxxXTeam/Qwen2API_Go/commit/8a7fcf17d958daa5d9e78c4e2e072a413d6cf609))
* toolcall ([cce176b](https://github.com/XxxXTeam/Qwen2API_Go/commit/cce176ba9145b540b134e1a8c631e5eef39154c3))
* toolcall ([71c77e0](https://github.com/XxxXTeam/Qwen2API_Go/commit/71c77e06b5323c86203294c42df6801ac4eded33))
