import asyncio
import json
import unittest

from src.gateway.sse import generate_sse, split_sentences


class SseTestCase(unittest.TestCase):
    def test_split_sentences_keeps_punctuation_chunks(self) -> None:
        self.assertEqual(
            split_sentences("你好，今天怎么样？挺好。"),
            ["你好，", "今天怎么样？", "挺好。"],
        )

    def test_generate_sse_uses_openai_chunk_format(self) -> None:
        async def _collect() -> list[str]:
            return [
                chunk
                async for chunk in generate_sse(
                    "你好，世界。",
                    model="qwen-plus",
                    created=1734523000,
                    completion_id="chatcmpl-fixed",
                )
            ]

        chunks = asyncio.run(_collect())

        self.assertEqual(chunks[-1], "data: [DONE]\n\n")
        first_payload = json.loads(chunks[0][6:])
        final_payload = json.loads(chunks[-2][6:])
        self.assertEqual(first_payload["object"], "chat.completion.chunk")
        self.assertEqual(first_payload["model"], "qwen-plus")
        self.assertEqual(first_payload["created"], 1734523000)
        self.assertEqual(first_payload["id"], "chatcmpl-fixed")
        self.assertEqual(first_payload["choices"][0]["delta"]["content"], "你好，")
        self.assertEqual(final_payload["choices"][0]["finish_reason"], "stop")
        self.assertEqual(final_payload["id"], "chatcmpl-fixed")


if __name__ == "__main__":
    unittest.main()
