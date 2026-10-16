"""Read-only MrToken model discovery and optional schema inspection."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import sys

import httpx


BASE_URL = "https://api.mrtoken.cn"


def error_message(response: httpx.Response, key: str) -> str:
    message = f"HTTP {response.status_code}"
    try:
        data = response.json()
        detail = data.get("detail")
        if isinstance(detail, dict) and isinstance(detail.get("reasons"), list):
            message = "; ".join(item for item in detail["reasons"] if isinstance(item, str)) or message
        elif isinstance(data.get("error"), dict) and isinstance(data["error"].get("message"), str):
            message = data["error"]["message"]
        elif isinstance(detail, str):
            message = detail
        elif isinstance(data.get("message"), str):
            message = data["message"]
    except (ValueError, AttributeError):
        pass
    return " ".join(message.replace(key, "[REDACTED]").split())[:400]


def inspect_models(key: str, requested: list[str], client: httpx.Client) -> dict:
    """Discover first; never transmit the key to a key-file URL or redirects."""
    headers = {"Authorization": f"Bearer {key}"}
    response = client.get(f"{BASE_URL}/v1/models", headers=headers, follow_redirects=False)
    if not response.is_success:
        raise ValueError(error_message(response, key))
    data = response.json()
    if not isinstance(data, dict) or not isinstance(data.get("data"), list):
        raise ValueError("Invalid model discovery response")
    models = [item["id"] for item in data.get("data", []) if isinstance(item, dict) and isinstance(item.get("id"), str)]
    report = {"base_url": BASE_URL, "count": len(models), "models": models, "schemas": {}}
    for name in requested:
        if name not in models:
            raise ValueError("Requested model was not returned by discovery")
        response = client.get(f"{BASE_URL}/v1/models/{name}/schema", headers=headers, follow_redirects=False)
        if response.is_success:
            schema = response.json()
            if not isinstance(schema, dict):
                raise ValueError("Invalid model schema response")
            report["schemas"][name] = {field: schema[field] for field in ("kind", "operations", "capabilities", "parameters", "limits", "media_inputs") if field in schema}
        else:
            report["schemas"][name] = {"status": response.status_code, "message": error_message(response, key)}
    # The provider must not be able to reflect the credential in console output.
    return json.loads(json.dumps(report).replace(key, "[REDACTED]"))


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--key-file", type=Path, help="External JSON containing a key field")
    parser.add_argument("--model", action="append", default=[], help="Discover before requesting this model's schema")
    args = parser.parse_args(argv)
    key = os.environ.get("PLATFORM_API_KEY", "").strip()
    if args.key_file is not None:
        try:
            value = json.loads(args.key_file.read_text(encoding="utf-8-sig")).get("key")
            if not isinstance(value, str) or not value.strip():
                raise ValueError("missing key")
            key = value.strip()
        except (OSError, ValueError, AttributeError):
            print("Cannot read a valid key from the external JSON file.", file=sys.stderr)
            return 2
    if not key:
        print("Set PLATFORM_API_KEY or supply --key-file; do not send credentials in chat.", file=sys.stderr)
        return 2
    try:
        with httpx.Client(timeout=45, follow_redirects=False) as client:
            report = inspect_models(key, args.model, client)
        print(json.dumps(report, ensure_ascii=False, indent=2))
        return 0
    except (ValueError, httpx.HTTPError):
        print("Model inspection failed; verify credentials, model access, and the HTTPS connection.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
