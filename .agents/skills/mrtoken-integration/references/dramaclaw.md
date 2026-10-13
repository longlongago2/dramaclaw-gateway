# DramaClaw configuration and delivery

Applies to a local CE installation using the companion `dramaclaw-gateway`. The platform protocol reference remains usable in other applications.

## Configure a channel and catalog

Inspect `AGENTS.md`, runtime ports, active gateway mode, and existing configuration first. Keep repository paths and process IDs local to the installation; do not reuse another machine's paths or IDs from a previous run.

The MrToken provider uses the existing NewAPI OpenAI-compatible channel type 1 and platform root `https://api.mrtoken.cn`. The updated gateway chooses the MrToken video adapter using the trusted exact channel host and mapped upstream model; a stock NewAPI type 1 configuration alone is insufficient. A root or SDK URL ending in `/v1` must produce only one `/v1` path segment.

The changes described here were source-built on 2026-10-12; they do not imply a
published NewAPI release/version. Verify the companion checkout contains the
MrToken adapter and `relay/channel/task/mrtoken/seedance.go`, its factory routing,
and the focused regression tests, then build and verify the running executable.
For another gateway, implement those protocol contracts or upgrade to a version
whose documented capabilities explicitly include both families.

In **Settings → Models and Channels → Provider Channel Management**:

1. Add/select MrToken; enter the authorized key, then save the channel promptly. Saving verifies `GET /v1/models`. Avoid source edits or page reloads while the key is only an unsaved draft.
2. Fetch available models and preserve their exact IDs.
3. In media model configuration, add the requested models with `provider=mrtoken`, the exact `upstreamModel`, `mediaType=video`, enabled status, and the corresponding secret-free config from `assets/media-models.json`.
4. Save media configuration separately from channel credentials. Preserve existing unrelated models. The CE media-model save API replaces the whole supplied list: read current entries, merge by model ID, then save. Never post just the new example entries over an existing catalog.

H3 R2V uses only `image_reference`, `referenceImageMax=4`, duration 1–30, and default local 720p/16:9; the gateway aligns 1280×720 to 1280×736. H3 does not generate native audio, so advertise `supportsGenerateAudio=false`.

Local H3 PNG/JPEG/WebP images (at most 20 MiB each) are encoded as data URLs by
DramaClaw and uploaded by the gateway directly to the platform file endpoint.
They do not require a public media relay. External image URLs still pass the
gateway's SSRF-protected fetch path. This does not change Seedance's relay needs.

Seedance uses text, first-frame, first/last-frame, image-reference and multimodal-reference modes, durations 4–15, and 9/3/3 reference limits. Example catalogs also declare media size/format/duration constraints. The gateway enforces the provider's combined 12-item cap.

## Build and validate

Build the updated companion gateway and run focused Go tests:

```text
go test ./relay/channel/task/mrtoken ./relay ./controller ./service
go build -o <temporary-binary> .
```

In DramaClaw, run relevant Python catalog/backend tests. The example source files are `examples/mrtoken-h3-media-models.json` and `examples/mrtoken-seedance2-media-models.json`. `src/novelvideo/video_duration.py` controls canonical backend duration limits.

Build before stopping the service. Replace only the intended local gateway executable after the old process has exited and released its Windows file handle. Restart with the same database/environment; do not reset SQLite, restart unrelated services, or discard saved channel settings. On Windows launch background helpers with a hidden window. Prefer the installation's `scripts/local_gateway.py` launcher when available.

Check the active binary and gateway health, then verify the model dropdown in the client. Saving a catalog does not update an old running gateway binary. Run secret scanning on tracked changes and new untracked files before sharing code; generated media and local runtime/credential state must remain ignored.

## Delivery troubleshooting

| Symptom | Resolution |
| --- | --- |
| H3 already completed, download returns 401 | Return the gateway's existing expiring signed `/v1/public/videos/{public-task}/content` link; the media downloader does not attach a gateway user token. Keep HMAC/expiry checks. Deliver the completed task. |
| H3 query/content fails without `model` | Use the original mapped platform model on both endpoints. |
| Operator-configured API endpoint resolves to a proxy fake IP | Use the provider outbound client for that fixed channel endpoint and prohibit redirects. Keep SSRF protection for user-supplied reference URLs; do not disable global protection. |
| Seedance task succeeded but no playable video | Parse `content.video_url`, then download/archive it through the existing delivery flow. Do not use H3's content endpoint. |
| An input URL cannot be fetched | Verify authorized media relay configuration and format/size limits. Do not change reference-fetch security policy to make a single URL work. |
| Model appears but submission is unavailable | Discovery is not a balance/routing guarantee. Report the sanitized upstream error and stop; do not silently switch paid models. |

For an authorized live test, use a small harmless scene and existing authorized reference assets. Preserve the upstream task after submission, distinguish provider completion from local delivery completion, confirm completed history, check real duration/dimensions/codec with a media probe, and play the saved video. A browser screenshot of the successful result is useful evidence. State separately which other variants have only contract coverage.
