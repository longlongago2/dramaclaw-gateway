# VolcEngine 音频生成

VolcEngine（type 45）新增 `seed-audio-1.0`，通过现有 `/v1/audio/speech`
统一入口调用豆包语音的非流式 `POST /api/v3/tts/create`。网关按模型映射后的
上游模型选择协议；旧 TTS 模型仍沿用原协议。DoubaoAudio（type 62）共享同一转换实现。

配置上游模型为 `seed-audio-1.0`，填写**豆包语音新版控制台的 API Key**，
请求使用 `X-Api-Key`，不能假定方舟 API Key 可用于此接口。旧版语音控制台可填写
`app-id|access-token`，适配器将发送 `X-Api-App-Id` 和 `X-Api-Access-Key`；新版 Key 优先。
默认请求地址为 `https://openspeech.bytedance.com/api/v3/tts/create`，也可配置
独立语音 Base URL。一个渠道只有一份凭据；若 Ark 和语音服务凭据不同，使用独立
DoubaoAudio 渠道配置语音，避免替换已有 Ark 渠道 Key。

DoubaoAudio 的“支持模型”返回内置目录 `seed-audio-1.0`，不会请求不存在的
OpenAI `/v1/models`。该目录仅表示网关已实现的协议，不代表账号已开通或 Key 已通过验证。
渠道首次同步通过网关动态元数据解析类型，后续更新保留模型映射与渠道类型。

```json
{
  "model": "seed-audio-1.0",
  "input": "雨夜街道的环境音，远处传来汽车声",
  "response_format": "wav",
  "metadata": {
    "references": [{"image_url": "https://example.com/scene.png"}],
    "audio_config": {"sample_rate": 48000, "pitch_rate": 0}
  }
}
```

`input` 映射为 `text_prompt`；`voice` 映射为音色参考，原有 `metadata.audio_url`
仍可用于音频参考。原生 `metadata.references` 支持 speaker/audio_data/audio_url/
image_data/image_url，每项只选一种；最多三个音频参考或一个图片参考，不能混用。
参考音频的时长限制由上游校验，已提供 Base64 的资源在本地校验编码和 10 MB 大小限制。
`metadata.audio_config` 支持 sample_rate、speech_rate、loudness_rate、pitch_rate、
enable_subtitle；`metadata.watermark` 支持 aigc_watermark 和 aigc_metadata 的官方字段。
显式零值和 false 会保留。格式支持 mp3、wav、pcm、opus/ogg_opus，拒绝 SSE。

开启 `metadata.audio_config.enable_subtitle=true` 时返回 JSON，含 `audio.url`、
`audio.data`（上游有 Base64 时）、`subtitle`、`duration`、`original_duration`。
字幕的句子和词时间戳按上游的毫秒单位保留。未开启字幕时继续返回音频二进制或
`audio.url` JSON，以兼容现有调用方。生成结果 URL 有效期由上游控制，文档标注为两小时。

成功时返回音频二进制或规范化音频 URL；按 original_duration 统计音频用量，
保留上游错误码及 X-Tt-Logid。模型输出最长 120 秒，具体权限及计费以供应商为准。

依据：[音频生成 HTTP 文档](https://docs.volcengine.com/docs/DoubaoVoice/audio-generation-http?lang=zh)。
方舟模型控制台需要登录及动态加载；此次仅新增文档明确列出的 `seed-audio-1.0`。
