import json
import re
import time
from typing import AsyncGenerator
from uuid import uuid4


def split_sentences(text: str) -> list[str]:
    parts = re.split(r"([，。？！；,\.!\?;])", text)
    sentences: list[str] = []
    current = ""
    for part in parts:
        current += part
        if re.match(r"[，。？！；,\.!\?;]", part):
            if current.strip():
                sentences.append(current)
            current = ""
    if current.strip():
        sentences.append(current)
    return sentences or [text]


async def generate_sse(
    reply: str,
    *,
    model: str,
    created: int | None = None,
    completion_id: str | None = None,
) -> AsyncGenerator[str, None]:
    created_at = created or int(time.time())
    response_id = completion_id or f"chatcmpl-{uuid4().hex[:12]}"
    for sentence in split_sentences(reply):
        chunk = {
            "choices": [{"delta": {"content": sentence}, "finish_reason": None, "index": 0}],
            "object": "chat.completion.chunk",
            "model": model,
            "created": created_at,
            "id": response_id,
        }
        yield f"data: {json.dumps(chunk, ensure_ascii=False)}\n\n"

    final = {
        "choices": [{"delta": {"content": ""}, "finish_reason": "stop", "index": 0}],
        "object": "chat.completion.chunk",
        "model": model,
        "created": created_at,
        "id": response_id,
    }
    yield f"data: {json.dumps(final, ensure_ascii=False)}\n\n"
    yield "data: [DONE]\n\n"
