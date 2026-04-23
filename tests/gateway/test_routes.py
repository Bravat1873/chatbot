import json
import os
import unittest
from unittest.mock import patch

from fastapi.testclient import TestClient

from src.gateway import routes
from src.gateway.app import app


class FakeEngine:
    def __init__(self, reply: str = "第一句，第二句。", should_fail: bool = False) -> None:
        self.reply = reply
        self.should_fail = should_fail
        self.calls: list[tuple[str, str, dict | None]] = []

    def process_turn(self, session_id: str, user_text: str, biz_params: dict | None = None) -> str:
        self.calls.append((session_id, user_text, biz_params))
        if self.should_fail:
            raise RuntimeError("boom")
        return self.reply


class GatewayRoutesTestCase(unittest.TestCase):
    def setUp(self) -> None:
        routes._engine = None

    def tearDown(self) -> None:
        routes._engine = None

    def test_chat_completions_streams_sse_reply(self) -> None:
        engine = FakeEngine()
        routes._engine = engine
        with patch.dict(
            os.environ,
            {"GATEWAY_AUTH_TOKEN": "secret", "LLM_MODEL": "default-model"},
            clear=False,
        ):
            client = TestClient(app)
            response = client.post(
                "/v1/chat/completions",
                headers={"Authorization": "Bearer secret"},
                json={
                    "model": "qwen-plus",
                    "session_id": "session-1",
                    "biz_params": {"customer_name": "张三"},
                    "messages": [
                        {"role": "assistant", "content": "您好"},
                        {"role": "user", "content": "有的"},
                    ],
                },
            )

        self.assertEqual(response.status_code, 200)
        self.assertEqual(engine.calls, [("session-1", "有的", {"customer_name": "张三"})])

        data_lines = [line for line in response.text.splitlines() if line.startswith("data: ")]
        self.assertEqual(data_lines[-1], "data: [DONE]")
        first_payload = json.loads(data_lines[0][6:])
        self.assertEqual(first_payload["choices"][0]["delta"]["content"], "第一句，")
        self.assertEqual(first_payload["model"], "qwen-plus")
        self.assertIn("created", first_payload)
        self.assertTrue(first_payload["id"].startswith("chatcmpl-"))

    def test_chat_completions_requires_auth_when_token_configured(self) -> None:
        routes._engine = FakeEngine()
        with patch.dict(os.environ, {"GATEWAY_AUTH_TOKEN": "secret"}, clear=False):
            client = TestClient(app)
            response = client.post(
                "/v1/chat/completions",
                json={"session_id": "session-1", "messages": [{"role": "user", "content": "有的"}]},
            )

        self.assertEqual(response.status_code, 401)

    def test_chat_completions_returns_safe_reply_when_engine_fails(self) -> None:
        routes._engine = FakeEngine(should_fail=True)
        with patch.dict(os.environ, {"GATEWAY_AUTH_TOKEN": "secret"}, clear=False):
            client = TestClient(app)
            response = client.post(
                "/v1/chat/completions",
                headers={"Authorization": "Bearer secret"},
                json={"session_id": "session-1", "messages": [{"role": "user", "content": "有的"}]},
            )

        self.assertEqual(response.status_code, 200)
        data_lines = [line for line in response.text.splitlines() if line.startswith("data: ")]
        first_payload = json.loads(data_lines[0][6:])
        self.assertEqual(first_payload["choices"][0]["delta"]["content"], "不好意思，")

    def test_healthz_returns_ok(self) -> None:
        client = TestClient(app)

        response = client.get("/healthz")

        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json(), {"status": "ok"})


if __name__ == "__main__":
    unittest.main()
