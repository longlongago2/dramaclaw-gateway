# Video protocols

Verified against platform discovery, H3 Schema, and the platform documentation on 2026-10-12. Refresh these sources before adapting a new model or changing a limit. [MrToken complete documentation](https://www.mrtoken.cn/mrtoken/docs/ai-api/all.md); [Seedance provider API](https://docs.volcengine.com/docs/ark/create-video-generation-task-api?lang=zh).

## H3 / OpenImgVideo

| Exact platform ID | Inputs | Fixed seconds |
| --- | --- | --- |
| `minimax-h3-t2v` | Prompt | 1–30 |
| `minimax-h3-i2v` | Prompt, optional first frame and last frame | 1–30 |
| `minimax-h3-r2v` | Prompt and 1–4 reference images | 1–30 |
| `minimax-h3-max-t2v` | Prompt | 1–30 |
| `minimax-h3-max-i2v` | Required first frame, optional last frame | 1–30 |

1. Upload each new PNG/JPEG/WebP with `POST /v1/files?model={platform-model}`, multipart field `file`. A current-key `file_` ID can be reused. The file ID contains 32 lowercase hexadecimal characters after `file_`.
   The verified upload response is a JSON object with top-level `id`, e.g.
   `{"id":"file_0123456789abcdef0123456789abcdef"}`. Require a valid ID and the
   routing header before creating the task; do not pass the whole upload response
   as a reference or assume another provider's `data` envelope.
2. Preserve `X-OpenImgVideo-Vendor-Key-Id` from the upload response; use `?vendorKeyId=...` for remaining uploads and creation to keep the file and task on the same upstream route. Treat missing routing identity as an error when newly uploading references. Files belong to the same user key and upstream instance as the task.
3. Create with `POST /v1/videos`. Preserve a unique `Idempotency-Key` and business `X-Request-Id`. Accept HTTP 202 as well as successful 200/201. Extract the upstream task ID without exposing the platform credential or confusing it with the gateway's public task ID.
4. Poll `GET /v1/videos/{upstream-id}?model={original-platform-model}`.
5. Download `GET /v1/videos/{upstream-id}/content?model={original-platform-model}` with Bearer authentication. The original model is required on query and download in the verified deployment. Preserve Range/206 support.

R2V request shape:

```json
{
  "model": "minimax-h3-r2v",
  "prompt": "A blue paper boat floating on a quiet lake.",
  "size": "1280x736",
  "seconds": 9,
  "reference_images": [{"file_id": "<OWNED_FILE_ID>"}]
}
```

`input_reference: {file_id: ...}` is the first frame; `last_frame: {file_id: ...}` is the tail frame. Do not combine either with R2V `reference_images`. H3 does not accept video/audio references. Align supplied dimensions upward to multiples of 32, each edge at most 2048; default R2V size is 1280×736. A local 16:9/720p request (1280×720) therefore becomes 1280×736, whose actual aspect ratio differs slightly from 16:9.

Typical native create response is `{"id":"video_...","status":"queued"}`; polling returns that same ID with `status`, optional progress/error/usage fields. Completion is normally `completed`; obtain media from the authenticated content endpoint even when polling contains a provider result URL. The gateway returns a separate public `task_...` ID to clients. Preserve both IDs in their respective boundaries. Local omission defaults are five seconds and, for R2V, 1280×736; the verified native R2V Schema has a nine-second duration default, so send the explicitly requested duration rather than confusing the two.

H3 status normalization is case-insensitive: queued/pending are queued;
in_progress/processing/running are running; completed/succeeded/success are
successful; failed/error/cancelled/canceled are terminal failures. Preserve the
existing gateway's cancellation semantics when mapping provider cancellation.
Typical failure payload is `{"id":"video_...","status":"failed","error":{"message":"..."}}`.
Redact the platform key before parsing/logging errors. For HTTP errors prioritize
`detail.reasons`, `error.message`, string `detail`, then `message`; a validation
`detail` array must not be dumped verbatim with credentials. Unknown statuses or
malformed responses are polling errors, not successful completions.

Optional `seed`, `steps`, `sampler`, `scheduler` follow the live Schema. Non-MAX models accept `parameters.turbo` and `parameters.lora_strength`; MAX does not. Keep optional scalar fields nullable/omittable and preserve explicit zero/false. H3's `seconds` differs from DC-Media's `duration`.

## Native Seedance 2.0

Verified platform IDs: `doubao-seedance-2.0`, `doubao-seedance-2.0-fast`, `doubao-seedance-2.0-mini`. Do not replace them with dated Volcengine model IDs or H3/OpenImgVideo identifiers.

Create: `POST /v1/video-generation/seedance`. Poll: `GET /v1/video-generation/seedance/tasks/{upstream-id}`. Both use Bearer authentication. These routes also have documented `/v2/` equivalents. Creation returns a task ID; support `id`, `task_id`, `taskId`, and a `data` envelope where needed.

```json
{
  "model": "doubao-seedance-2.0-mini",
  "content": [
    {"type": "image_url", "image_url": {"url": "https://example.invalid/reference.png"}, "role": "reference_image"},
    {"type": "text", "text": "The blue paper boat drifts slowly."}
  ],
  "duration": 4,
  "resolution": "720p",
  "ratio": "16:9",
  "generate_audio": false,
  "return_last_frame": false,
  "seed": 0
}
```

Canonical DC-Media mapping:

| DC-Media source | Native content block |
| --- | --- |
| `prompt` | `type=text`, `text=prompt` |
| `image` | `type=image_url`, role `first_frame` |
| `metadata.last_frame_image` | `type=image_url`, role `last_frame` |
| `metadata.reference_images[]` | `type=image_url`, role `reference_image` |
| `metadata.reference_videos[]` | `type=video_url`, role `reference_video` |
| `metadata.reference_audios[]` | `type=audio_url`, role `reference_audio` |
| `duration` | top-level `duration` (integer seconds) |
| `metadata.resolution` / `metadata.ratio` | top-level `resolution` / `ratio` |
| `metadata.generate_audio` | top-level `generate_audio` |
| `metadata.seed` / `metadata.return_last_frame` / `metadata.watermark` | corresponding top-level optional scalar |

In a DramaClaw/NewAPI gateway, both families enter through `POST /v1/video/generations`
(or the configured `video/generations` endpoint relative to a local gateway SDK root).
The gateway adapter performs the native transformation; the platform root uses the
distinct native endpoints above. A canonical H3 R2V example is:

```json
{
  "model": "minimax-h3-r2v",
  "prompt": "A blue paper boat floating on a quiet lake.",
  "duration": 9,
  "width": 1280,
  "height": 720,
  "metadata": {
    "resolution": "720p",
    "ratio": "16:9",
    "reference_images": ["https://example.invalid/reference.png"]
  }
}
```

For canonical Seedance use `model: "doubao-seedance-2.0-mini"`, `duration: 4`,
the same `metadata.resolution`, `metadata.ratio`, and `metadata.reference_images`
fields, plus optional `metadata.generate_audio`, `metadata.seed`,
`metadata.return_last_frame`, and `metadata.watermark`. R2V images become uploaded
file references; Seedance images remain URL content blocks. The examples use the
same upstream/gateway model ID; a configured local alias must resolve to the exact
discovered platform model.

Canonical `model` is a gateway mapping ID or alias; `prompt` is required for H3,
but Seedance can omit it when a visual reference exists. `n` defaults to 1 and must
remain 1. Reference arrays contain strings, not `{url: ...}` or `{file_id: ...}`
objects. Use `image` for the first frame, and `metadata.last_frame_image` for its
optional tail; do not combine them with `metadata.reference_images`. The UI's
`gen_mode` selects a client endpoint/workflow and is not a supplier field in this
canonical gateway body. Omit unused references and optional scalars. Local omitted
Seedance duration/resolution/ratio defaults are 5 seconds, 720p, and 16:9. Clients
must use their local gateway relay token at this boundary, not the platform key;
the channel injects the platform key upstream.

Use nested `{url: ...}` media fields. Rebuild content from canonical inputs; do not forward supplier-shaped `metadata.content` that can override the prompt or reference validation. Fixed durations are 4–15 seconds. The current adapter requires fixed duration; it does not support auto-duration editing. Reject metadata duration/frames overrides before billing. Seedance uses resolution tiers and ratio, rather than H3's aligned `size` string; `auto` ratio maps to `adaptive`.

Advertise 480p/720p on Fast and Mini. The base model also supports 1080p for text/first-frame modes; reference-image/multimodal generation has a lower resolution limit. Refresh provider documentation when extending supported tiers.

Reference limits: up to 9 images, 3 videos, 3 audio clips, and 12 items in total. A last frame requires a first frame. Audio references require an image or video reference. Reference audio/video clips are 2–15 seconds each, total audio/video duration at most 15 seconds per type. Media URLs must be provider-reachable; local filesystem paths are not URLs. Use the client's authorized media relay for local assets. Seedance references are not uploaded through H3's `/v1/files` protocol.

Poll `queued/pending`, `running/processing/in_progress`. Treat `succeeded/completed` as success, `failed/error/canceled/cancelled` as terminal failure. Extract `content.video_url` from successful native responses and archive it before its URL expires. Do not invent a native Seedance `/videos/{id}/content` endpoint. Refresh an expired result by querying its existing task; do not create a new paid generation.
