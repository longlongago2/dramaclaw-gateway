from __future__ import annotations

import importlib.util
import json
from pathlib import Path

import httpx
import pytest


SCRIPT = Path(__file__).resolve().parents[1] / "scripts/discover_models.py"
SPEC = importlib.util.spec_from_file_location("mrtoken_skill_helper", SCRIPT)
HELPER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HELPER)


def test_skill_discovers_first_and_only_uses_authenticated_read_calls():
    paths = []

    def handle(request):
        paths.append(request.url.path)
        assert request.method == "GET"
        assert request.url.host == "api.mrtoken.cn"
        assert request.headers["Authorization"] == "Bearer test-platform-credential"
        if request.url.path == "/v1/models":
            return httpx.Response(200, json={"data": [{"id": "minimax-h3-r2v"}]})
        return httpx.Response(200, json={"kind": "video", "parameters": {"secret-reflection": "test-platform-credential"}, "irrelevant": "omitted"})

    with httpx.Client(transport=httpx.MockTransport(handle)) as client:
        result = HELPER.inspect_models("test-platform-credential", ["minimax-h3-r2v"], client)
    assert paths == ["/v1/models", "/v1/models/minimax-h3-r2v/schema"]
    assert "test-platform-credential" not in json.dumps(result)
    assert "irrelevant" not in result["schemas"]["minimax-h3-r2v"]


def test_skill_does_not_request_undiscovered_model():
    calls = []

    def handle(request):
        calls.append(request.url.path)
        return httpx.Response(200, json={"data": []})

    with httpx.Client(transport=httpx.MockTransport(handle)) as client:
        with pytest.raises(ValueError, match="not returned"):
            HELPER.inspect_models("test-platform-credential", ["unknown"], client)
    assert calls == ["/v1/models"]


def test_skill_error_priority_and_redaction():
    response = httpx.Response(403, json={"detail": {"reasons": ["invalid test-platform-credential"]}, "message": "lower priority"})
    assert HELPER.error_message(response, "test-platform-credential") == "invalid [REDACTED]"


@pytest.mark.parametrize("body", [{"data": None}, {}, []])
def test_skill_rejects_malformed_model_discovery(body):
    with httpx.Client(transport=httpx.MockTransport(lambda request: httpx.Response(200, json=body))) as client:
        with pytest.raises(ValueError, match="Invalid model discovery response"):
            HELPER.inspect_models("test-platform-credential", [], client)
