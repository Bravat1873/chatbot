import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from src.config import get_settings
from src.gateway.routes import router


logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(_: FastAPI):
    settings = get_settings()
    logger.info(
        "gateway_started host=%s port=%s request_logging=%s",
        settings.gateway_host,
        settings.gateway_port,
        settings.gateway_log_requests,
    )
    yield


app = FastAPI(title="Chatbot Gateway", lifespan=lifespan)
app.include_router(router)


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.exception_handler(Exception)
async def handle_unexpected_error(request: Request, exc: Exception) -> JSONResponse:
    logger.exception("unhandled_gateway_error path=%s", request.url.path, exc_info=exc)
    return JSONResponse(status_code=500, content={"detail": "Internal Server Error"})

