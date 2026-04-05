from __future__ import annotations

import os
from dataclasses import dataclass, replace
from functools import lru_cache
from pathlib import Path

from dotenv import load_dotenv


BASE_DIR = Path(__file__).resolve().parent
# 启动时自动加载项目根目录下的 .env，便于本地直接运行 demo。
load_dotenv(BASE_DIR / ".env", override=False)


def _get_bool(name: str, default: bool) -> bool:
    # 统一兼容常见布尔环境变量写法，避免每个配置项单独判断。
    raw_value = os.getenv(name)
    if raw_value is None:
        return default
    return raw_value.strip().lower() in {"1", "true", "yes", "on"}


@dataclass(frozen=True)
class Settings:
    # 外部服务配置
    dashscope_api_key: str
    amap_key: str
    asr_model: str = "qwen3-asr-flash-realtime"
    llm_model: str = "qwen3.5-flash"
    tts_model: str = "cosyvoice-v3-flash"
    tts_voice: str = "longanyang"

    # 录音与本地 VAD 参数
    sample_rate: int = 16_000
    channels: int = 1
    chunk_ms: int = 100
    vad_energy_threshold: float = 700.0
    vad_end_silence_seconds: float = 1.0
    vad_no_response_seconds: float = 8.0
    max_record_seconds: float = 30.0
    debug: bool = True
    default_city: str = "广州"
    tts_format: str = "mp3"
    tts_sample_rate: int = 24_000
    tts_timeout_seconds: float = 20.0

    @classmethod
    def from_env(cls) -> "Settings":
        # 所有运行期配置统一从 .env / 环境变量装配，main 里只做少量 CLI 覆盖。
        return cls(
            # 阿里百炼平台API Key
            dashscope_api_key=os.getenv("DASHSCOPE_API_KEY", "").strip(),
            # 高德地图API Key
            amap_key=os.getenv("AMAP_KEY", "").strip(),
            # 模型配置
            asr_model=os.getenv("ASR_MODEL", "qwen3-asr-flash-realtime").strip(),
            llm_model=os.getenv("LLM_MODEL", "qwen3.5-flash").strip(),
            tts_model=os.getenv("TTS_MODEL", "cosyvoice-v3-flash").strip(),
            # TTS 配置
            tts_voice=os.getenv("TTS_VOICE", "longanyang").strip(),
            sample_rate=int(os.getenv("SAMPLE_RATE", "16000")),
            channels=int(os.getenv("CHANNELS", "1")),
            chunk_ms=int(os.getenv("CHUNK_MS", "100")),
            vad_energy_threshold=float(os.getenv("VAD_ENERGY_THRESHOLD", "700")),
            vad_end_silence_seconds=float(os.getenv("VAD_END_SILENCE_SECONDS", "1.0")),
            vad_no_response_seconds=float(os.getenv("VAD_NO_RESPONSE_SECONDS", "8.0")),
            max_record_seconds=float(os.getenv("MAX_RECORD_SECONDS", "30.0")),
            debug=_get_bool("DEBUG", True),
            default_city=os.getenv("DEFAULT_CITY", "广州").strip(),
            tts_format=os.getenv("TTS_FORMAT", "mp3").strip(),
            tts_sample_rate=int(os.getenv("TTS_SAMPLE_RATE", "24000")),
            tts_timeout_seconds=float(os.getenv("TTS_TIMEOUT_SECONDS", "20.0")),
        )

    def override(self, **kwargs: object) -> "Settings":
        # dataclass + replace 的方式可以保持 Settings 不可变，便于在不同入口安全复用。
        return replace(self, **kwargs)


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    # 整个进程内只读取一次配置，避免重复解析环境变量。
    return Settings.from_env()
