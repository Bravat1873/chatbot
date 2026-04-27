# 阿里云 AICCS 外呼封装：使用官方 Python SDK 发起大模型外呼并查询通话详情。

from __future__ import annotations

from src.config import get_settings


def make_call(called_number: str) -> str:
    """发起 LlmSmartCall 外呼，返回 CallId。"""
    settings = get_settings()
    try:
        from alibabacloud_aiccs20191015 import models as aiccs_models
        from alibabacloud_aiccs20191015.client import Client
        from alibabacloud_tea_openapi import models as open_api_models
    except ImportError as exc:
        raise RuntimeError("未安装 alibabacloud-aiccs20191015") from exc

    config = open_api_models.Config(
        access_key_id=settings.aliyun_access_key_id,
        access_key_secret=settings.aliyun_access_key_secret,
    )
    config.endpoint = "aiccs.aliyuncs.com"
    client = Client(config)
    request = aiccs_models.LlmSmartCallRequest(
        called_number=called_number,
        caller_number=settings.caller_number,
        application_code=settings.aiccs_app_code,
        session_timeout=1200,
    )
    response = client.llm_smart_call(request)
    return response.body.call_id


def get_call_dialog_content(call_id: str, call_date: str | None = None) -> dict:
    """按 CallId 查询大模型外呼的对话内容与挂断信息。

    对应 AICCS 接口：GetCallDialogContent。

    Args:
        call_id: LlmSmartCall 返回的 CallId。
        call_date: 通话发生的日期，格式 YYYY-MM-DD，默认今天。
    """
    from datetime import date

    settings = get_settings()
    try:
        from alibabacloud_aiccs20191015 import models as aiccs_models
        from alibabacloud_aiccs20191015.client import Client
        from alibabacloud_tea_openapi import models as open_api_models
    except ImportError as exc:
        raise RuntimeError("未安装 alibabacloud-aiccs20191015") from exc

    config = open_api_models.Config(
        access_key_id=settings.aliyun_access_key_id,
        access_key_secret=settings.aliyun_access_key_secret,
    )
    config.endpoint = "aiccs.aliyuncs.com"
    client = Client(config)
    request = aiccs_models.GetCallDialogContentRequest(
        call_id=call_id,
        call_date=call_date or date.today().isoformat(),
    )
    response = client.get_call_dialog_content(request)
    body = response.body
    return body.to_map() if hasattr(body, "to_map") else body.__dict__
