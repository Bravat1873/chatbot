from __future__ import annotations

import shutil
import subprocess
import tempfile
import threading
from pathlib import Path
from typing import Any

from config import Settings

try:
    import dashscope
    from dashscope.audio.tts_v2 import ResultCallback, SpeechSynthesizer
    from dashscope.audio.tts_v2.speech_synthesizer import AudioFormat
except ImportError:  # pragma: no cover - exercised only when dependency is absent
    dashscope = None
    ResultCallback = object  # type: ignore[assignment]
    SpeechSynthesizer = None  # type: ignore[assignment]
    AudioFormat = None  # type: ignore[assignment]


class TTSClient:
    def __init__(self, settings: Settings, tracker: Any = None) -> None:
        self.settings = settings
        self.tracker = tracker

    def speak(self, text: str) -> None:
        # TTS 仍然保持同步播放，确保 demo 中“机器人说完再轮到用户”。
        cleaned = text.strip()
        if not cleaned:
            return

        audio_path = self._synthesize_to_file(cleaned)
        try:
            self._play_audio(audio_path)
        finally:
            audio_path.unlink(missing_ok=True)

    def _synthesize_to_file(self, text: str) -> Path:
        if not self.settings.dashscope_api_key:
            raise RuntimeError("未配置 DASHSCOPE_API_KEY，无法调用语音合成。")
        if dashscope is None or SpeechSynthesizer is None:
            raise RuntimeError("未安装 dashscope，无法使用 DashScope 语音合成。")

        dashscope.api_key = self.settings.dashscope_api_key
        dashscope.base_websocket_api_url = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"

        suffix = f".{self.settings.tts_format.lower()}"
        temp_file = tempfile.NamedTemporaryFile(delete=False, suffix=suffix)
        audio_path = Path(temp_file.name)
        temp_file.close()

        # DashScope TTS 通过回调持续吐音频分片，这里先完整落盘，再调用本机播放器播报。
        finished = threading.Event()
        synthesis_error: dict[str, str] = {}

        class Callback(ResultCallback):
            def __init__(self, output_path: Path) -> None:
                super().__init__()
                self.file = output_path.open("wb")

            def on_complete(self) -> None:
                self.file.close()
                finished.set()

            def on_error(self, message: str) -> None:
                synthesis_error["message"] = message
                self.file.close()
                finished.set()

            def on_data(self, data: bytes) -> None:
                self.file.write(data)

        synthesizer = SpeechSynthesizer(
            model=self.settings.tts_model,
            voice=self.settings.tts_voice,
            format=self._resolve_audio_format(),
            callback=Callback(audio_path),
        )
        try:
            synthesizer.call(text)
            if not finished.wait(timeout=self.settings.tts_timeout_seconds):
                raise RuntimeError("语音合成超时，请稍后重试。")
            if synthesis_error:
                raise RuntimeError(f"语音合成失败: {synthesis_error['message']}")
            if not audio_path.exists() or audio_path.stat().st_size == 0:
                raise RuntimeError("语音合成失败：未生成有效音频文件。")
        except Exception:
            audio_path.unlink(missing_ok=True)
            raise
        finally:
            close = getattr(synthesizer, "close", None)
            if callable(close):
                close()

        if self.tracker:
            self.tracker.add(
                service="tts",
                model=self.settings.tts_model,
                input_chars=len(text),
            )

        return audio_path

    def _play_audio(self, audio_path: Path) -> None:
        command = self._build_play_command(audio_path)
        if command is None:
            raise RuntimeError(
                "当前系统未找到可用音频播放器。macOS 可使用 afplay，Linux 可安装 ffplay 或 mpg123。"
            )

        subprocess.run(
            command,
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    @staticmethod
    def _build_play_command(audio_path: Path) -> list[str] | None:
        # 优先用系统现成播放器，避免为了 demo 再引入额外 Python 音频播放依赖。
        if shutil.which("afplay"):
            return ["afplay", str(audio_path)]
        if shutil.which("ffplay"):
            return [
                "ffplay",
                "-nodisp",
                "-autoexit",
                "-loglevel",
                "quiet",
                str(audio_path),
            ]
        if audio_path.suffix.lower() == ".mp3" and shutil.which("mpg123"):
            return ["mpg123", "-q", str(audio_path)]
        if audio_path.suffix.lower() == ".wav" and shutil.which("aplay"):
            return ["aplay", "-q", str(audio_path)]
        return None

    def _resolve_audio_format(self):
        if AudioFormat is None:
            raise RuntimeError("当前 dashscope SDK 不支持 AudioFormat，无法初始化 TTS。")

        # 不同 SDK 版本对采样率的表达方式不一样，这里统一映射到枚举，避免直接传裸参数。
        fmt = self.settings.tts_format.strip().lower()
        sample_rate = int(self.settings.tts_sample_rate)
        if fmt == "mp3":
            bitrate = "128KBPS" if sample_rate in {8000, 16000} else "256KBPS"
            enum_name = f"MP3_{sample_rate}HZ_MONO_{bitrate}"
        elif fmt == "wav":
            enum_name = f"WAV_{sample_rate}HZ_MONO_16BIT"
        elif fmt == "pcm":
            enum_name = f"PCM_{sample_rate}HZ_MONO_16BIT"
        elif fmt == "opus":
            rate_map = {8000: "8KHZ", 16000: "16KHZ", 24000: "24KHZ", 48000: "48KHZ"}
            if sample_rate not in rate_map:
                raise RuntimeError(
                    f"当前 opus 格式不支持采样率 {sample_rate}，请改用 8000/16000/24000/48000。"
                )
            enum_name = f"OGG_OPUS_{rate_map[sample_rate]}_MONO_32KBPS"
        else:
            raise RuntimeError(f"不支持的 TTS_FORMAT: {self.settings.tts_format}")

        audio_format = getattr(AudioFormat, enum_name, None)
        if audio_format is None:
            raise RuntimeError(
                "当前 dashscope SDK 不支持该音频组合："
                f"format={self.settings.tts_format}, sample_rate={sample_rate}"
            )
        return audio_format
