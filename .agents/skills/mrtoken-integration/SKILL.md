---
name: mrtoken-integration
description: Configure MrToken API access and video models, adapt MiniMax H3, Seedance or HappyHorse protocols, and verify model discovery and delivery in DramaClaw or another client. Use for MrToken onboarding, channel configuration, video-provider integration, and related troubleshooting.
---

# MrToken integration

Read the current [platform documentation](https://www.mrtoken.cn/mrtoken/docs/ai-api/all.md). API root is `https://api.mrtoken.cn`; OpenAI-compatible SDK `base_url` / `baseURL` is `https://api.mrtoken.cn/v1`. Authenticate with `Authorization: Bearer <PLATFORM_API_KEY>`. These addresses are distinct from the local gateway address.

## Discover before configuring

Obtain the credential from an authorized environment variable or an external secret file. Never copy the credential into this skill, a repository, command arguments, logs, screenshots, or replies. If a UI file-access policy prevents reading a local credential, follow that policy and let the user enter it; do not switch browser surfaces to bypass the restriction.

First call `GET /v1/models`. Use exact returned `data[].id` values and preserve them through model mapping. Model availability does not prove a model's protocol, permissions, balance, or capabilities. Do not add models missing from the discovery result.

The read-only helper [scripts/discover_models.py](scripts/discover_models.py) requires Python 3.11+ and `httpx`:

```text
python scripts/discover_models.py
python scripts/discover_models.py --key-file /external/path/platform-key.json --model minimax-h3-r2v
```

The default credential is `PLATFORM_API_KEY`; a key file must contain a string `key` field. Its `url` field never overrides the destination. The helper always discovers models before requesting schemas and never creates a paid generation.

## Select the protocol

Read [references/video-protocols.md](references/video-protocols.md) when integrating video models. H3's OpenImgVideo `/v1/videos` and Seedance's `/v1/video-generation/seedance` are different protocols. Reusing an OpenAI-compatible channel type alone does not adapt either request or polling format. Chat and image compatibility does not establish video compatibility.

For HappyHorse, read [references/happyhorse.md](references/happyhorse.md). It uses the platform's generic video-synthesis and task-query endpoints with native `input.media` fields. [assets/happyhorse-media-models.json](assets/happyhorse-media-models.json) contains six generation-mode examples; merge only models returned by current discovery. Video editing requires separate duration and billing support and is not enabled by those examples.

For H3, query `GET /v1/models/{exact-model-id}/schema` to refresh limits. The same schema endpoint can reject native Seedance models with `requires providerCode OpenImgVideo`; use the Seedance documentation instead, without renaming the model or treating the rejection as a missing model.

For DramaClaw channel setup, media catalog merge, gateway build, browser configuration, and local troubleshooting, read [references/dramaclaw.md](references/dramaclaw.md). [assets/media-models.json](assets/media-models.json) supplies secret-free example entries; filter them against current discovery and merge with the existing catalog. For other clients, use the protocol reference directly; the skill does not require Codex-specific browser APIs.

## Verify the requested outcome

The helper's offline regression tests live in `tests/test_mrtoken_skill_helper.py`.
Run them with an interpreter containing `pytest` and `httpx`:

```text
python -m pytest tests/test_mrtoken_skill_helper.py -q
```

Run contract tests for canonical conversion, mapped model identity, input limits, optional zero/false values, task IDs, terminal errors, and secret redaction. A saved channel or accepted task is not evidence of a playable video.

If the user has requested or authorized real generation, submit one small representative task for each requested protocol that needs live verification, poll its original ID, download it, check its media metadata, and play it in the target UI. Additional variants can use contract tests unless the user requests live coverage. Record which variants were actually generated.

Once a paid submission begins, do not automatically recreate it after a timeout or uncertain response. Preserve the business request ID, body, and any upstream task ID; query the existing task or report an uncertain submission. Retry polling or delivery independently. Fix a download failure by delivering the completed task, not by generating another video.

Report configured models, actual validation results, local output/project links, and material limitations. Use masked credentials and avoid sharing signed provider URLs. Capture the successful UI state when configuring through a browser; do not refresh a form with an unsaved credential draft.
