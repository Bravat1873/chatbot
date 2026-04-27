# 语音识别客户端：本地 VAD（声音活动检测）+ 云端 DashScope 实时 ASR，返回转写文本。

from __future__ import annotations

import base64
import os
import tempfile
import threading
import time
import wave
from collections import deque
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from src.config import Settings

try:
    import numpy as np
except ImportError:  # pragma: no cover - exercised only when dependency is absent
    np = None

try:
    import sounddevice as sd
except ImportError:  # pragma: no cover - exercised only when dependency is absent
    sd = None


@dataclass
class AudioCaptureResult:
    """录音结果：包含原始 PCM、时长、是否检测到人声等。"""
    audio_bytes: bytes
    sample_rate: int
    duration_seconds: float
    speech_detected: bool
    timed_out: bool
    file_path: Path | None = None

    def cleanup(self) -> None:
        if self.file_path and self.file_path.exists():
            self.file_path.unlink(missing_ok=True)


class ASRClient:
    """
    语音识别客户端。
    - listen_once: 麦克风录音 + 流式 ASR，返回转写结果
    - record_audio + transcribe: 离线场景的先录后转模式
    """
    def __init__(self, settings: Settings, tracker: Any = None) -> None:
        self.settings = settings
        self.tracker = tracker

    def listen_once(self) -> dict[str, Any]:
        """
        流式录音+识别：边采音频边推流到云端，静音达到阈值后结束识别。
        返回 {"timed_out": bool, "text": str}。
        """
        # 轻量流式：仍然保持“一问一答”，但录音时就持续把音频分片推给云端 ASR，
        # 避免“本地录完一整段后再上传”带来的额外等待。
        self._validate_audio_settings()
        if np is None or sd is None:
            raise RuntimeError(
                "录音依赖未安装。请先执行 `pip install -r requirements.txt`。"
            )
        if not self.settings.dashscope_api_key:
            raise RuntimeError("未配置 DASHSCOPE_API_KEY，无法调用语音识别。")

        frames_per_chunk = max(
            1, int(self.settings.sample_rate * self.settings.chunk_ms / 1000)
        )
        chunk_seconds = frames_per_chunk / self.settings.sample_rate
        pre_roll = deque(maxlen=max(1, int(0.3 / chunk_seconds)))
        speech_detected = False
        trailing_silence = 0.0
        started_at = time.monotonic()
        conv = None
        done: threading.Event | None = None
        result: dict[str, Any] | None = None
        streamed_bytes = 0

        with sd.InputStream(
            samplerate=self.settings.sample_rate,
            channels=self.settings.channels,
            dtype="int16",
            blocksize=frames_per_chunk,
        ) as stream:
            while True:
                chunk, overflowed = stream.read(frames_per_chunk)
                if overflowed and self.settings.debug:
                    print("[ASR] 检测到录音缓冲区溢出，继续处理当前片段。")

                mono_chunk = chunk.reshape(-1).astype("float32")
                energy = float(np.sqrt(np.mean(np.square(mono_chunk)))) if mono_chunk.size else 0.0
                is_speech = energy >= self.settings.vad_energy_threshold
                elapsed = time.monotonic() - started_at

                if speech_detected:
                    # 一旦进入“用户已开口”状态，就边采边发，直到静音持续足够久再收尾。
                    chunk_bytes = chunk.astype("int16").tobytes()
                    self._append_audio_chunk(conv, chunk_bytes)
                    streamed_bytes += len(chunk_bytes)
                    if is_speech:
                        trailing_silence = 0.0
                    else:
                        trailing_silence += chunk_seconds
                        if trailing_silence >= self.settings.vad_end_silence_seconds:
                            break
                else:
                    pre_roll.append(chunk.copy())
                    if is_speech:
                        # 只有真正检测到人声时才建立云端会话，避免空连和无意义计费。
                        speech_detected = True
                        conv, done, result = self._open_realtime_conversation()
                        # 把开口前短暂缓冲一并补发，避免用户首音节被截掉。
                        for buffered in pre_roll:
                            buffered_bytes = buffered.astype("int16").tobytes()
                            self._append_audio_chunk(conv, buffered_bytes)
                            streamed_bytes += len(buffered_bytes)
                        trailing_silence = 0.0
                    elif elapsed >= self.settings.vad_no_response_seconds:
                        return {"timed_out": True, "text": ""}

                if elapsed >= self.settings.max_record_seconds:
                    break

        if not speech_detected or conv is None or done is None or result is None:
            return {"timed_out": True, "text": ""}

        wait_ok = False
        try:
            conv.commit()
            conv.end_session(timeout=15)
            wait_ok = done.wait(timeout=15)
        finally:
            conv.close()

        self._ensure_session_completed(wait_ok, result)

        if self.tracker:
            self.tracker.add(
                "asr",
                model=self.settings.asr_model,
                audio_seconds=streamed_bytes / 2 / self.settings.sample_rate,
            )

        text = "".join(result["texts"]).strip()
        return {"timed_out": not bool(text), "text": text}

    def record_audio(self) -> AudioCaptureResult:
        self._validate_audio_settings()
        if np is None or sd is None:
            raise RuntimeError(
                "录音依赖未安装。请先执行 `pip install -r requirements.txt`。"
            )

        frames_per_chunk = max(
            1, int(self.settings.sample_rate * self.settings.chunk_ms / 1000)
        )
        chunk_seconds = frames_per_chunk / self.settings.sample_rate
        # 预留一小段前置缓冲，避免刚开口时首音节被截断。
        pre_roll = deque(maxlen=max(1, int(0.3 / chunk_seconds)))
        collected_chunks: list[Any] = []
        speech_detected = False
        trailing_silence = 0.0
        started_at = time.monotonic()

        with sd.InputStream(
            samplerate=self.settings.sample_rate,
            channels=self.settings.channels,
            dtype="int16",
            blocksize=frames_per_chunk,
        ) as stream:
            while True:
                chunk, overflowed = stream.read(frames_per_chunk)
                if overflowed and self.settings.debug:
                    print("[ASR] 检测到录音缓冲区溢出，继续处理当前片段。")

                mono_chunk = chunk.reshape(-1).astype("float32")
                energy = float(np.sqrt(np.mean(np.square(mono_chunk)))) if mono_chunk.size else 0.0
                is_speech = energy >= self.settings.vad_energy_threshold
                elapsed = time.monotonic() - started_at

                if speech_detected:
                    # 一旦判定用户开始说话，就持续收集，直到连续静音达到结束阈值。
                    collected_chunks.append(chunk.copy())
                    if is_speech:
                        trailing_silence = 0.0
                    else:
                        trailing_silence += chunk_seconds
                        if trailing_silence >= self.settings.vad_end_silence_seconds:
                            break
                else:
                    pre_roll.append(chunk.copy())
                    if is_speech:
                        speech_detected = True
                        collected_chunks.extend(list(pre_roll))
                        trailing_silence = 0.0
                    elif elapsed >= self.settings.vad_no_response_seconds:
                        # 长时间没有检测到人声，交给上层状态机做“您还在吗”重试。
                        return AudioCaptureResult(
                            audio_bytes=b"",
                            sample_rate=self.settings.sample_rate,
                            duration_seconds=elapsed,
                            speech_detected=False,
                            timed_out=True,
                        )

                if elapsed >= self.settings.max_record_seconds:
                    break

        if not collected_chunks:
            return AudioCaptureResult(
                audio_bytes=b"",
                sample_rate=self.settings.sample_rate,
                duration_seconds=0.0,
                speech_detected=False,
                timed_out=True,
            )

        audio_array = np.concatenate(collected_chunks, axis=0)
        audio_bytes = audio_array.astype("int16").tobytes()
        wav_path = self._persist_to_wav(audio_bytes)
        duration_seconds = len(audio_bytes) / 2 / self.settings.sample_rate

        return AudioCaptureResult(
            audio_bytes=audio_bytes,
            sample_rate=self.settings.sample_rate,
            duration_seconds=duration_seconds,
            speech_detected=True,
            timed_out=False,
            file_path=wav_path,
        )

    def transcribe(self, capture: AudioCaptureResult) -> str:
        if capture.timed_out or not capture.file_path:
            return ""
        if not self.settings.dashscope_api_key:
            raise RuntimeError("未配置 DASHSCOPE_API_KEY，无法调用语音识别。")
        self._validate_audio_settings()

        os.environ["DASHSCOPE_API_KEY"] = self.settings.dashscope_api_key

        try:
            from dashscope.audio.qwen_omni import (
                MultiModality,
                OmniRealtimeCallback,
                OmniRealtimeConversation,
            )
            from dashscope.audio.qwen_omni.omni_realtime import TranscriptionParams
        except ImportError as exc:  # pragma: no cover
            raise RuntimeError("未安装 dashscope>=1.25.6，无法调用实时语音识别。") from exc

        done = threading.Event()
        result: dict[str, Any] = {"texts": [], "error": None, "session_finished": False}
        wait_ok = False
        sent_audio_seconds = 0.0

        class _Cb(OmniRealtimeCallback):
            def on_open(self) -> None:
                pass

            def on_close(self, code: Any, msg: Any) -> None:
                done.set()

            def on_event(self, resp: dict) -> None:
                t = resp.get("type", "")
                if t == "conversation.item.input_audio_transcription.completed":
                    text = resp.get("transcript", "")
                    if text:
                        result["texts"].append(text)
                elif t == "session.finished":
                    result["session_finished"] = True
                    done.set()
                elif t == "error":
                    error = resp.get("error", {})
                    result["error"] = error.get("message") or resp.get("message", str(resp))
                    done.set()

        conv = OmniRealtimeConversation(
            model=self.settings.asr_model,
            url="wss://dashscope.aliyuncs.com/api-ws/v1/realtime",
            callback=_Cb(),
        )
        try:
            conv.connect()

            conv.update_session(
                output_modalities=[MultiModality.TEXT],
                enable_turn_detection=False,
                enable_input_audio_transcription=True,
                transcription_params=TranscriptionParams(
                    language="zh",
                    sample_rate=self.settings.sample_rate,
                    input_audio_format="pcm",
                ),
            )

            # 从 WAV 中提取原始 PCM 并分块发送
            with wave.open(str(capture.file_path), "rb") as wf:
                pcm_data = wf.readframes(wf.getnframes())
                frame_rate = wf.getframerate() or self.settings.sample_rate
                sample_width = wf.getsampwidth()
                channels = max(1, wf.getnchannels())

            if channels != 1:
                raise RuntimeError("实时语音识别仅支持单声道音频，请将 CHANNELS 设为 1。")

            bytes_per_second = frame_rate * sample_width * channels
            sent_audio_seconds = len(pcm_data) / bytes_per_second if bytes_per_second else 0.0
            chunk_size = max(1, frame_rate * sample_width * channels // 10)  # 100ms per chunk
            for i in range(0, len(pcm_data), chunk_size):
                conv.append_audio(base64.b64encode(pcm_data[i:i + chunk_size]).decode())

            conv.commit()
            conv.end_session(timeout=15)
            wait_ok = done.wait(timeout=15)
        finally:
            conv.close()

        self._ensure_session_completed(wait_ok, result)

        if self.tracker:
            self.tracker.add(
                "asr",
                model=self.settings.asr_model,
                audio_seconds=sent_audio_seconds,
            )

        return "".join(result["texts"]).strip()

    def _open_realtime_conversation(
        self,
    ) -> tuple[Any, threading.Event, dict[str, Any]]:
        os.environ["DASHSCOPE_API_KEY"] = self.settings.dashscope_api_key

        try:
            from dashscope.audio.qwen_omni import (
                MultiModality,
                OmniRealtimeCallback,
                OmniRealtimeConversation,
            )
            from dashscope.audio.qwen_omni.omni_realtime import TranscriptionParams
        except ImportError as exc:  # pragma: no cover
            raise RuntimeError("未安装 dashscope>=1.25.6，无法调用实时语音识别。") from exc

        done = threading.Event()
        result: dict[str, Any] = {"texts": [], "error": None, "session_finished": False}

        class _Cb(OmniRealtimeCallback):
            def on_open(self) -> None:
                pass

            def on_close(self, code: Any, msg: Any) -> None:
                done.set()

            def on_event(self, resp: dict) -> None:
                t = resp.get("type", "")
                # 这里仍然只取“本轮最终转写”，不消费中间增量，
                # 这样可以维持现有状态机对一条完整用户输入的假设。
                if t == "conversation.item.input_audio_transcription.completed":
                    text = resp.get("transcript", "")
                    if text:
                        result["texts"].append(text)
                elif t == "session.finished":
                    result["session_finished"] = True
                    done.set()
                elif t == "error":
                    error = resp.get("error", {})
                    result["error"] = error.get("message") or resp.get("message", str(resp))
                    done.set()

        conv = OmniRealtimeConversation(
            model=self.settings.asr_model,
            url="wss://dashscope.aliyuncs.com/api-ws/v1/realtime",
            callback=_Cb(),
        )
        conv.connect()
        conv.update_session(
            output_modalities=[MultiModality.TEXT],
            # 当前 demo 仍以本地 VAD 控制“何时结束这一轮”，先不把 turn detection 的控制权
            # 完全交给云端，便于保持既有超时和重试语义。
            enable_turn_detection=False,
            enable_input_audio_transcription=True,
            transcription_params=TranscriptionParams(
                language="zh",
                sample_rate=self.settings.sample_rate,
                input_audio_format="pcm",
            ),
        )
        return conv, done, result

    @staticmethod
    def _append_audio_chunk(conv: Any, audio_bytes: bytes) -> None:
        conv.append_audio(base64.b64encode(audio_bytes).decode())

    def _validate_audio_settings(self) -> None:
        if self.settings.channels != 1:
            raise RuntimeError("实时语音识别仅支持单声道录音，请将 CHANNELS 设为 1。")
        if self.settings.sample_rate not in {8_000, 16_000}:
            raise RuntimeError("实时语音识别当前仅支持 8kHz 或 16kHz 采样率。")

    @staticmethod
    def _ensure_session_completed(wait_ok: bool, result: dict[str, Any]) -> None:
        if result["error"]:
            raise RuntimeError(f"语音识别失败: {result['error']}")
        if not wait_ok or not result["session_finished"]:
            raise RuntimeError("语音识别未在预期时间内完成，未收到 session.finished。")

    def _persist_to_wav(self, audio_bytes: bytes) -> Path:
        # ASR 直接读取本地 wav 文件，临时文件在对话轮次结束后删除。
        fd, file_name = tempfile.mkstemp(prefix="chatbot_", suffix=".wav")
        os.close(fd)
        path = Path(file_name)

        with wave.open(str(path), "wb") as wav_file:
            wav_file.setnchannels(self.settings.channels)
            wav_file.setsampwidth(2)
            wav_file.setframerate(self.settings.sample_rate)
            wav_file.writeframes(audio_bytes)

        return path
