# 网关路由：暴露 /v1/chat/completions 端点，兼容 OpenAI 风格，SSE 流式返回。

import logging
from typing import Any
from uuid import uuid4

from fastapi import APIRouter, Depends, Request
from fastapi.responses import StreamingResponse

from src.config import get_settings
from src.core.dialogue import DialogueEngine
from src.core.geocode import AMapGeocoder
from src.core.intent import IntentClassifier
from src.gateway.auth import verify_auth
from src.gateway.sse import generate_sse


router = APIRouter()
# 延迟初始化引擎，避免依赖未就绪时创建
_engine: DialogueEngine | None = None
logger = logging.getLogger(__name__)
SAFE_FALLBACK_REPLY = "不好意思，请您再说一遍？"
def get_engine() -> DialogueEngine:
    """获取或创建（单例）对话引擎。"""
    global _engine
    if _engine is None:
        settings = get_settings()
        _engine = DialogueEngine(
            intent_classifier=IntentClassifier(settings, use_llm=True),
            geocoder=AMapGeocoder(settings),
        )
    return _engine


@router.post("/v1/chat/completions")
async def chat_completions(request: Request, _=Depends(verify_auth)):
    """
    OpenAI 兼容的聊天接口。
    从 messages 中提取用户输入，委托 DialogueEngine 处理后 SSE 流式返回。
    """
    body = await request.json()
    settings = get_settings()
    if settings.gateway_log_requests:
        logger.info(
            "gateway_request headers=%s body=%s",
            dict(request.headers),
            body,
        )

    session_id = body.get("session_id") or str(uuid4())
    model = str(body.get("model") or settings.llm_model)
    messages = body.get("messages", [])
    biz_params = body.get("biz_params")
    user_text = _extract_user_text(messages)
    completion_id = f"chatcmpl-{uuid4().hex[:12]}"

    engine = get_engine()
    try:
        reply = engine.process_turn(session_id, user_text, biz_params=_coerce_biz_params(biz_params))
    except Exception:
        logger.exception("gateway_process_turn_failed session_id=%s", session_id)
        reply = SAFE_FALLBACK_REPLY

    return StreamingResponse(
        generate_sse(reply, model=model, completion_id=completion_id),
        media_type="text/event-stream",
    )


def _extract_user_text(messages: Any) -> str:
    """从 messages 列表中提取最新一条用户消息。"""
    if not isinstance(messages, list):
        return ""

    for msg in reversed(messages):
        if isinstance(msg, dict) and msg.get("role") == "user":
            return str(msg.get("content", ""))
    return ""


def _coerce_biz_params(value: Any) -> dict[str, Any] | None:
    """将 biz_params 规范化为 dict 或 None。"""
    if value is None:
        return None
    if isinstance(value, dict):
        return value
    return {"raw": value}
