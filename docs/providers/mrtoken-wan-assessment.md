# MrToken Wan 视频适配评估（2026-10-15）

状态：Wan 2.7 T2V/I2V/R2V 与 Wan 3.0 两型号的生成适配及契约测试已加入本功能分支，本地网关和 DramaClaw 媒体目录已更新。Wan 2.7 R2V 已通过一次 MrToken 原生创建；复用该任务完成本地网关轮询、DramaClaw 下载及导入画布后的播放验收。另从 UI 单次提交了最短 2 秒任务并获得上游 ID，但平台查询持续返回 HTTP 500，完整 UI 闭环尚未完成；其余型号的真实生成权限和 MrToken 实际计费仍待验证。

## 范围与依据

用户提供的百炼页面指向 `wan2.7-r2v`。阿里云另列出 Wan 2.7 文生、图生和视频编辑型号，它们应按各自协议评估，不应仅凭 `wan` 前缀共用 R2V 的请求转换。

- [Wan 2.7 R2V API](https://help.aliyun.com/zh/model-studio/wan-video-to-video-api-reference)
- [Wan 2.7 R2V 模型信息](https://help.aliyun.com/zh/model-studio/wan2-7-r2v)
- [Wan 3.0 视频生成 API](https://help.aliyun.com/zh/model-studio/wan3-video-generation-api-reference)
- [阿里云视频模型列表](https://help.aliyun.com/zh/model-studio/use-video-generation/)
- [MrToken 平台文档](https://www.mrtoken.cn/mrtoken/docs/ai-api/all.md)
- [MrToken 通用视频创建](https://www.mrtoken.cn/mrtoken/docs/ai-api/08-video-synthesis.md)与[任务查询](https://www.mrtoken.cn/mrtoken/docs/ai-api/10-video-task-query.md)

2026-10-15 使用仓库外授权密钥只读调用 `GET /v1/models`，共返回 100 个 ID；其中 Wan 视频为 `wan2.7-t2v`、`wan2.7-i2v`、`wan2.7-r2v`、`wan2.7-videoedit`、`wan3.0-video`、`wan3.0-video-prime`。另有图像模型 `wan2.7-image`、`wan2.7-image-pro`，不属于本次视频适配。上述 6 个视频 ID 的 `/v1/models/{id}/schema` 均返回 HTTP 422 `requires providerCode OpenImgVideo`；这是 Schema 接口不匹配，不代表模型发现失败或实际生成权限已验证。

MrToken 文档确认通用视频创建 `POST /v1/video-generation/video-synthesis`、查询 `GET /v1/video-generation/tasks/{task_id}`，并透传按目标模型决定的 `input`、`parameters` 内部结构。文档指出部分 DashScope 视频模型由平台添加必要的异步请求头；R2V 原生任务已实测完成，其余型号仍待实测。创建通常返回 `output.task_id`，查询透传 `output.task_status`、`output.video_url` 等供应商字段。

## 已确认的阿里云 R2V 契约

- 异步创建使用 `POST /api/v1/services/aigc/video-generation/video-synthesis`，请求头需要 `X-DashScope-Async: enable`；查询为 `GET /api/v1/tasks/{task_id}`。这些是**阿里云直连接口**，不能假定 MrToken 原样暴露。
- 请求体为 `model`、`input.prompt`、`input.media[]`、`parameters`。媒体元素含 `type` (`first_frame`、`reference_image`、`reference_video`)、`url`，图/视频参考可附 `reference_voice`。必须保留媒体次序，提示词的“图 N / 视频 N”依此分别编号。
- 最多一张首帧；图和视频参考合计 1–5 项。音色参考依附于某一图或视频，不是独立的音频列表。图像 20 MB，参考视频 100 MB 且 1–30 秒，音色文件 15 MB 且 1–10 秒；详细格式和尺寸以 API 文档为准。
- 输出 720P/1080P；默认 1080P。时长整数：含参考视频为 2–10 秒，否则为 2–15 秒，默认 5 秒。可选 `ratio`、`prompt_extend`、`watermark`、`seed`。首帧存在时，比例由首帧决定。
- 创建返回 `output.task_id`；轮询的 `output.task_status` 为 PENDING/RUNNING/SUCCEEDED/FAILED，成功后 `output.video_url` 仅保留 24 小时。应及时归档，提交结果不等于成片交付。

## 适配前缺口与处理

1. `relay/channel/task/mrtoken/request.go` 将已发现的五个生成 ID 纳入网关白名单；`wan2.7-videoedit` 仍未接入。
2. `relay/channel/task/mrtoken/wan.go` 与 `wan3.go` 分型号重建原生请求。通用接口创建、轮询、`output.task_id` 和 `output.video_url` 解析已接线；提交后的不确定响应沿用现有禁止自动重提机制。
3. `relay/common/dc_media_protocol.go` 的模型专用校验允许 Wan R2V 首帧加参考图、Wan I2V 尾帧加驱动音频或首段视频；具体媒体组合与计费时长仍由 Wan 规划器在预扣费前校验。
4. DC-Media 原有分组字段可用于无音色绑定的参考素材；R2V 的混排次序和逐素材音色使用 `metadata.wan_media`，例如 `[{"type":"reference_video","url":"https://…/a.mp4","reference_voice":"https://…/voice.mp3"},{"type":"reference_image","url":"https://…/b.png"}]`。该字段只能包含参考图/视频，不能与分组参考数组混用。
5. 按用户确认沿用开源项目原有下载和存储逻辑：与原生 Ali、Doubao 适配器一致，鉴权任务查询在 `metadata.url` 返回平台结果 URL，DramaClaw 不携带平台 Bearer Key，使用原下载器下载并原子写入项目存储；不依赖 MrToken 转存。可选的网关内容代理继续执行 SSRF 防护。本机 DNS 返回 Fake IP 会使该代理返回 403，此问题未修复；直接结果下载已验收通过，两条路径不能混为一谈。
6. DramaClaw 原仓库中的 `examples/mrtoken-wan27-media-models.json` 和 `examples/mrtoken-wan3-media-models.json` 只宣告目前可从通用 UI 完成的文生、首帧/首尾帧和图片参考模式；Wan 2.7 的视频/音色引用及 Wan 3.0 的音频引用可由规范 API 提交，但尚未宣告为 UI 模式。实际价格必须按 MrToken 配置。
7. 接受 DramaClaw 根据 `metadata.resolution`、`metadata.ratio` 推导的冗余 `width/height`，沿用 DC-Media 比例一致性校验；上游只接收分辨率档位和模型支持的比例，不发送像素尺寸。缺少档位或比例的尺寸输入，以及自定义 FPS，仍在预扣费前拒绝。

Wan 3.0 当前明确拒绝参考视频、文件和网页输入。其供应商 `usage.duration` 在包含参考视频时为输入与输出时长之和；网关现有预扣费只知道请求输出秒数，开放该模式前必须建立可信素材时长与结算规则。智能时长 `duration=-1` 同样尚未开放。

## 下一步验证门槛

- 已通过共享技能的 `scripts/discover_models.py` 完成发现；凭据只从用户指定的仓库外 JSON 文件读取，未写入仓库。VideoEdit 默认采用输入视频时长且账单可能包含输入/输出时长，需另行设计可信时长探测与预扣费；Wan 3.0 视频参考同理。
- 核对各型号的 MrToken 实际权限、输入限制与价格；模型发现及 Schema 422 都不能代替真实创建和查询结果验证。
- 继续验证模型映射 ID、混排媒体次序/音色绑定、首帧组合、时长边界、零值/false、任务 ID、终态错误、密钥脱敏、结果 URL 下载与防重提。当前已覆盖主要本地契约，真实权限与内容仍未验证。
- 2026-10-15 已提交一条最短 2 秒、720P、一张参考图的 `wan2.7-r2v` 原生 MrToken 任务：HTTP 200 接受，按同一上游 ID 轮询至 SUCCEEDED，视频 HTTP 200 下载完成；`ffprobe` 测得 MP4/H.264、1280×720、30fps、2.02 秒、AAC 音轨，FFmpeg 全片解码无报错。该测试证明平台路由与单图 R2V 权限，不等于本地网关部署及 DramaClaw UI 播放验收。
- 后续复用上述已完成任务，在本地网关恢复一条明确标记为恢复交付、额度为 0 的任务记录；生产轮询器从 SUBMITTED 更新为 SUCCESS。鉴权查询返回 completed，DramaClaw 原下载器成功归档 1,339,539 字节 MP4，SHA256 与原任务视频一致。随后通过 UI 导入测试项目，节点命名为“Wan 2.7 R2V 验收（复用任务）”；浏览器测得 readyState=4、duration=2.02，点击播放后 currentTime=2.02、ended=true，无媒体解码错误。这验证已有任务的恢复交付与播放，不等同于从 UI 新建生成任务。
- 其余型号及 Wan R2V 多图、视频、音色组合仅有本地契约覆盖。新建接口的 HTTP 请求、202 接受、任务 ID 和防重提标记由模拟上游契约测试验证；若真实建单响应不确定，禁止自动重提。
- 2026-10-15 通过 DramaClaw 页面单次提交 `wan2.7-r2v`，2 秒、720p、16:9、单图参考。网关公开 ID 为 `task_XacNdPniPu18CeUMDY6Eo8JvudoUuQwc`，上游 ID 为 `585bfbef-4867-40a2-a7ad-55550d1f5f19`。创建成功，但网关轮询与使用原平台密钥直接查询都返回 HTTP 500 / Internal Server Error，尚无新任务结果可供自动下载和播放验收。保留同一任务，未重新生成；后续仅恢复查询及交付。

本地 Go 定向测试及构建通过；DramaClaw 媒体目录合并保留了原模型，五个 Wan 型号在设置页面可见。运行中的网关已替换为本分支构建。本阶段未进行远程 CI、提交、推送或部署发布；开发位于两个原仓库的 `feat/mrtoken-wan-video` 分支。

## 页面失败根因与修复（2026-10-15）

指定画布的 `MrToken Wan` 节点（`55b19555-f267-4f4c-a162-aba220e8ec90`）对应网关任务
`task_ucLaisZJhREwTjMkCArwaHq5JewSafOn`、上游任务 `71531b89-e608-455c-a3a1-898c64fe0fab`。
网关记录显示 14:48:06 提交、14:50:06 成功（北京时间），但 DC-Media
`GET /v1/video/generations/{id}` 的 `taskResponseResultURL` 仍强制把 Wan 改写为签名代理。
此前只验证 OpenAI 风格查询和下载器，遗漏页面实际使用的 DC-Media 结果 URL。
因此生成成功后客户端仍反复请求网关代理并收到 SSRF 403；重启 API 后 inline 任务被清理为
“服务重启,任务已中断,请重新发起”，掩盖了先前的交付失败。

修复将原生 Wan 与 Ali、Doubao 及已接入的原生 MrToken 模型保持一致，DC-Media
`result_url` 和 `results[].url` 均保留平台结果 URL；H3 仍使用鉴权内容代理。
回归测试覆盖五个 Wan 型号及 Seedance、HappyHorse。运行网关更新后，同一任务查询返回
原生结果 URL，原 DramaClaw 下载器成功下载 6,226,869 字节；ffprobe 检查为 H.264/AAC、
1280×720、4.04 秒，FFmpeg 全片解码通过。修复未关闭 SSRF 防护，也未重提收费任务。
随后通过原节点的上传视频入口恢复这份成片，保留原提示词、Wan 型号和 4 秒参数。
浏览器在项目文件地址播放至 4.04 秒结束，readyState=4、ended=true、无媒体错误。
这是原任务恢复交付验收，并非修复后重新建单的自动完整闭环。服务已恢复运行于
API 8780、前端 5180、网关 18473；本机依赖加载较慢，本次启动等待时间设为 300 秒。
