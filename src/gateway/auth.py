from fastapi import HTTPException, Request

from src.config import get_settings


async def verify_auth(request: Request) -> None:
    settings = get_settings()
    if not settings.gateway_auth_token:
        return

    auth_header = request.headers.get("Authorization", "")
    token = auth_header.replace("Bearer ", "").strip()
    if token != settings.gateway_auth_token:
        raise HTTPException(status_code=401, detail="Unauthorized")
