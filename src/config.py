from __future__ import annotations

import os
from dataclasses import dataclass, replace
from functools import lru_cache
from pathlib import Path

from dotenv import load_dotenv


BASE_DIR = Path(__file__).resolve().parent.parent
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
    """不可变配置类，所有参数从环境变量装配，支持局部覆盖。"""

    # === 外部服务配置 ===
    dashscope_api_key: str      # 阿里云 DashScope（百炼）API Key
    amap_key: str               # 高德地图 API Key
    asr_model: str = "qwen3-asr-flash-realtime"   # 语音识别模型
    llm_model: str = "qwen3.5-flash"              # 意图分类用大模型
    tts_model: str = "cosyvoice-v3-flash"          # 语音合成模型
    tts_voice: str = "longanyang"                   # TTS 默认音色
    aliyun_access_key_id: str = ""                  # 阿里云 AK（AICCS 外呼使用）
    aliyun_access_key_secret: str = ""             # 阿里云 SK
    aiccs_app_code: str = ""                        # AICCS 应用 Code
    caller_number: str = ""                         # 主叫号码
    gateway_auth_token: str = ""                    # Gateway 鉴权 Token
    gateway_host: str = "0.0.0.0"                   # Gateway 监听地址
    gateway_port: int = 8000                        # Gateway 监听端口
    gateway_log_requests: bool = True               # 是否记录请求日志

    # === 录音与本地 VAD 参数 ===
    sample_rate: int = 16_000                       # 采样率
    channels: int = 1                               # 声道数（实时 ASR 仅支持单声道）
    chunk_ms: int = 100                             # 每次读取音频的毫秒数
    vad_energy_threshold: float = 700.0             # VAD 能量阈值
    vad_end_silence_seconds: float = 1.0            # 连续静音多久认为说话结束
    vad_no_response_seconds: float = 8.0            # 多久无回应当超时
    max_record_seconds: float = 30.0                # 单次录音最大秒数
    debug: bool = True                              # 调试模式，打印中间结果
    default_city: str = "广州"                       # 高德地理编码默认城市
    tts_format: str = "mp3"                         # TTS 音频格式
    tts_sample_rate: int = 24_000                   # TTS 采样率
    tts_timeout_seconds: float = 20.0               # TTS 合成超时

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
            aliyun_access_key_id=os.getenv("ALIYUN_ACCESS_KEY_ID", "").strip(),
            aliyun_access_key_secret=os.getenv("ALIYUN_ACCESS_KEY_SECRET", "").strip(),
            aiccs_app_code=os.getenv("AICCS_APP_CODE", "").strip(),
            caller_number=os.getenv("CALLER_NUMBER", "").strip(),
            gateway_auth_token=os.getenv("GATEWAY_AUTH_TOKEN", "").strip(),
            gateway_host=os.getenv("GATEWAY_HOST", "0.0.0.0").strip(),
            gateway_port=int(os.getenv("GATEWAY_PORT", "8000")),
            gateway_log_requests=_get_bool("GATEWAY_LOG_REQUESTS", True),
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
