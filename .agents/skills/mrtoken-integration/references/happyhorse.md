# HappyHorse through MrToken

Checked model discovery and documentation on 2026-10-14. Refresh discovery before
configuring; model availability does not prove successful paid generation.

## Sources and protocol

- [MrToken platform API](https://www.mrtoken.cn/mrtoken/docs/ai-api/all.md)
- [Alibaba HappyHorse API reference](https://help.aliyun.com/zh/model-studio/happyhorse-api-reference/)
- [T2V](https://help.aliyun.com/zh/model-studio/happyhorse-text-to-video-api-reference)
- [I2V](https://help.aliyun.com/zh/model-studio/happyhorse-image-to-video-api-reference)
- [R2V](https://help.aliyun.com/zh/model-studio/happyhorse-reference-to-video-api-reference)
- [Video editing](https://help.aliyun.com/zh/model-studio/happyhorse-video-edit-api-reference)

Use `https://api.mrtoken.cn` with `Authorization: Bearer <PLATFORM_API_KEY>`.
An SDK configured with `https://api.mrtoken.cn/v1` must not append another `/v1`.
The platform owns upstream routing; do not send its key directly to Alibaba.

Create: `POST /v1/video-generation/video-synthesis`.
Poll: `GET /v1/video-generation/tasks/{upstream-task-id}`.
Unlike H3, HappyHorse does not use `/v1/files`, `/v1/videos` or their content route.
The OpenImgVideo model schema endpoint returns HTTP 422 for these models; that
does not mean they are unavailable. Use the native provider reference for inputs.

Exact discovered generation IDs: `happyhorse-1.0-t2v`, `happyhorse-1.0-i2v`,
`happyhorse-1.0-r2v`, `happyhorse-1.1-t2v`, `happyhorse-1.1-i2v`,
`happyhorse-1.1-r2v`. Do not map them to an invented generic `happyhorse-1.0` ID.

```json
{
  "model": "happyhorse-1.1-r2v",
  "input": {
    "prompt": "[Image 1] shows a blue paper boat drifting slowly on a lake.",
    "media": [{"type": "reference_image", "url": "https://example.invalid/boat.png"}]
  },
  "parameters": {"resolution": "720P", "ratio": "16:9", "duration": 5, "seed": 0, "watermark": false}
}
```

T2V has no media. I2V requires exactly one `type=first_frame` item; its aspect
ratio follows the image, so omit `parameters.ratio`. R2V requires 1–9
`type=reference_image` items. Images can be public HTTP(S) URLs or image data URLs;
PNG/JPEG/WebP, at most 20 MiB, each edge at least 300 px. A local path is not a URL.
Use the client's media relay or authorized data encoding. Prompt references use
`[Image 1]`, `[Image 2]`, etc.

Generation durations are integer 3–15 seconds. Resolution values are uppercase
`480P`, `720P`, `1080P`. Ratios for T2V/R2V: 16:9, 9:16, 1:1, 4:3, 3:4, 4:5,
5:4, 9:21, 21:9. Seed is 0–2147483647; watermark is optional boolean. Preserve
explicit zero/false and omit absent optional values. No tail-frame capability or
audio-reference input is advertised by the generation examples.

DramaClaw may send disabled canonical flags `generate_audio=false` and
`return_last_frame=false`. Treat these as disabled and omit them from the native
HappyHorse body; do not reject an otherwise valid request merely because those
fields are present. Reject enabled unsupported flags rather than silently
advertising capabilities the provider does not expose.

## Canonical gateway mapping and delivery

DramaClaw enters through `POST /v1/video/generations`, using its local relay
token. The channel injects the platform key. Map `prompt` to `input.prompt`,
`image` to a first-frame media item, `metadata.reference_images[]` to reference
media items, and duration/resolution/ratio/seed/watermark to `parameters`.
Rebuild the native body; supplier-shaped `metadata.input/media/parameters` must
not override validated inputs or duration used for billing.

Create responses normally contain `output.task_id`; save it without confusing
it with the gateway's public ID. Poll `output.task_status` and read successful
`output.video_url`. PENDING/QUEUED/RUNNING/PROCESSING are nonterminal;
SUCCEEDED/SUCCESS/COMPLETED succeed; FAILED/ERROR/CANCELED/CANCELLED fail.
Treat unknown states and success without a video URL as errors. Preserve
`output.message` on failure with credentials redacted.

Download the completed result URL without forwarding the platform key to its
CDN. Archive before expiry and verify playback. Do not invent a content endpoint
or resubmit generation to repair delivery.

## Separate video-edit capability

Discovery also returned `happyhorse-1.0-video-edit`. Native `input.media` requires
one `type=video` item and accepts 0–5 reference images. Resolution is 720P/1080P;
optional `audio_setting` is `auto` or `origin`. Output duration follows the input
up to 15 seconds; it does not take generation `duration` or `ratio` parameters.
Input videos may be 3–60 seconds and at most 100 MiB. Do not enable this model
by copying a fixed-duration generation entry: implement media validation,
input-derived output duration, and correct billing first. Current examples and
gateway HappyHorse adapter cover the six generation variants only.
