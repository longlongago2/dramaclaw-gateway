# MrToken 视频：H3 与 Seedance 2.0

MrToken 渠道继续使用 OpenAI 兼容的 type 1，Base URL 为
`https://api.mrtoken.cn`。SDK 地址 `https://api.mrtoken.cn/v1` 同样可以识别。
视频提交时，网关按照配置中的完整主机名选择 `relay/channel/task/mrtoken/`
适配器，再按映射后的平台模型 ID 选择 H3 或 Seedance 协议，并记录 `platform=mrtoken`。普通 OpenAI / Sora 渠道仍使用
原有适配器。无需修改渠道 type 或迁移数据库；升级后需保持新版本网关负责查询这些任务。

先用平台 Key 调用 `GET /v1/models`，只使用实际返回的模型 ID。
模型映射支持逻辑别名，供应商地址与上游 Key 仍由现有渠道配置管理。

| 平台模型 | 支持输入 |
|---|---|
| `minimax-h3-t2v` | 文生视频 |
| `minimax-h3-i2v` | 文生、首帧、首尾帧、单图生视频 |
| `minimax-h3-r2v` | 1–4 张参考图 |
| `minimax-h3-max-t2v` | 文生视频 |
| `minimax-h3-max-i2v` | 必填首帧，可选尾帧 |

DC-Media 的 `duration` 转换成 `seconds`，取值 1–30；不支持 `duration=auto`。
`width/height` 转成 `size`，向上对齐到 32 的倍数，每边最多 2048。
例如 1280×720 转成 1280×736。MAX I2V 支持 `metadata.ratio=auto`。
不传尺寸时使用平台 Schema 的默认尺寸。

首帧 `image`、尾帧 `metadata.last_frame_image`、参考图
`metadata.reference_images` 可以传 HTTP(S) URL、PNG/JPEG/WebP Base64 data URL，
或者当前 Key 可用的 `file_...` ID。URL 下载使用现有 SSRF 防护，单图最多 20 MiB。
新上传文件通过 `/v1/files?model=...` 获取 file ID；保存响应中的
`X-OpenImgVideo-Vendor-Key-Id`，后续上传和创建以 `vendorKeyId` 固定路由。
不支持视频、音频、任意文件或链接参考。

可选参数通过 `metadata.seed`、`metadata.steps`、`metadata.sampler`、
`metadata.scheduler` 传入。普通 H3 支持 `metadata.parameters.turbo` 和
`metadata.parameters.lora_strength`；MAX 不接受这两个参数。显式零值及 false 会保留。

平台创建使用 `POST /v1/videos`，可返回 HTTP 202。查询和下载分别使用
`GET /v1/videos/{id}?model=...` 和 `GET /v1/videos/{id}/content?model=...`。
必须传创建时映射后的模型；2026-10-11 实测省略 model 会返回 HTTP 500。
创建请求使用公开任务 ID 作为 X-Request-Id / Idempotency-Key，但不假定平台能消除
所有重复请求：一旦开始提交，连接异常或错误响应不会触发网关自动重建任务。
查询采用现有网关轮询生命周期。完成结果通过现有的 24 小时签名下载代理和
DramaClaw 归档链交付，支持 Range / 206，不向调用方返回平台签名结果 URL。
内容下载只访问渠道配置生成的平台接口，复用渠道出站客户端，并禁止重定向；
参考图等用户提供的 URL 继续使用 SSRF 防护客户端。

## Seedance 2.0 系列

原样使用模型发现返回的 `doubao-seedance-2.0`、`doubao-seedance-2.0-fast`、
`doubao-seedance-2.0-mini`。这三个模型走 `POST /v1/video-generation/seedance`，
查询走 `GET /v1/video-generation/seedance/tasks/{id}`；不能使用 H3 的 `/videos`
或将模型替换成火山引擎带日期的 ID。平台的 OpenImgVideo Schema 接口对它们返回
providerCode 不匹配，并不表示模型不在可用列表。

网关重建原生 `content`：prompt → text，image → first_frame，
metadata.last_frame_image → last_frame，metadata.reference_images/videos/audios
→ reference_image/video/audio。每条媒体块用对应的 image_url/video_url/audio_url
对象保存 URL，不采用 metadata.content 中的覆盖值。

固定时长 4–15 秒，省略时为 5 秒。使用 metadata.resolution 和 metadata.ratio，
省略时分别为 720p、16:9，auto 比例转换成 adaptive。Fast/Mini 支持 480p、720p；
基础模型额外支持文生/首帧 1080p，图片参考/多模态场景不接受该档位。
metadata.generate_audio、seed、return_last_frame、watermark 转成原生可选字段，
显式 0 和 false 保留；不接受 metadata.duration/frames 覆盖标准时长。

支持最多 9 图、3 视频、3 音频，合计最多 12 项；纯音频参考、缺少首帧的尾帧和
任意文件/链接参考会在预扣费前拒绝。局部/全能参考使用供应商可访问的媒体 URL；
不使用 H3 文件上传协议。成功查询必须对应原任务并含 content.video_url，
结果 URL 沿用原生 Seedance 的媒体下载和 DramaClaw 归档链，不虚构 H3 内容接口。
时长边界、参考类型/上限、三种型号的提交/查询路径及可选零值由契约测试覆盖。
统一网关 `/v1/videos/{public-task-id}/content` 可从 Seedance 结果 URL 读取内容，
支持 Range，执行 SSRF 校验，并且不会把平台 Bearer Key 发给结果 CDN。

2026-10-12 本机 H3 R2V 一图测试已完成：请求 1 秒、1280×736，实际成片 1.625 秒，
成功归档并播放。Seedance 三个型号的真实创建均被上游以“模型不存在或无权访问”
拒绝；当前只能证明配置、协议转换和错误处理，真实成片仍需平台侧恢复可用路由。

本地预扣费采用已验证的请求时长作为 seconds 倍率。平台的实际生成时长可能不同
（本次 1 秒请求返回 actual_seconds=1.625），平台侧按实际 usage 计费；本适配器不
改写已有网关定价或完成时结算策略。需在本地网关为模型配置合适的价格。

验证：`go test ./relay/channel/task/mrtoken ./relay ./controller ./service`。
真实测试需要显式设置 `MRTOKEN_LIVE_KEY_FILE`，指向仓库外包含 `key` 字段的 JSON：

```powershell
$env:MRTOKEN_LIVE_KEY_FILE = "C:\path\outside-repository\platform-key.json"
go test ./relay/channel/task/mrtoken -run '^TestMrTokenLiveVideo$' -count=1 -v -timeout 8m
```

真实测试先查询模型，再创建一个收费的 1 秒文生任务，轮询终态并验证 MP4 下载。
2026-10-11 已通过；I2V、首尾帧、R2V、MAX 的转换与路由由契约测试覆盖，尚未逐一真实生成。
密钥仅从外部文件读入内存，不写入源码、测试输出或生成文件。

依据：[平台完整文档](https://www.mrtoken.cn/mrtoken/docs/ai-api/all.md)，
以及鉴权读取的 `/v1/models/{model}/schema`（2026-10-11）。
