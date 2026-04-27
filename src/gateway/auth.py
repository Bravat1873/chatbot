# 网关鉴权：Bearer Token 验证，无 token 时放行。

from fastapi import HTTPException, Request

from src.config import get_settings


async def verify_auth(request: Request) -> None:
    """验证请求 Authorization header 中的 Bearer Token。"""
    settings = get_settings()
    if not settings.gateway_auth_token:
        return

    auth_header = request.headers.get("Authorization", "")
    token = auth_header.replace("Bearer ", "").strip()
    if token != settings.gateway_auth_token:
        raise HTTPException(status_code=401, detail="Unauthorized")
