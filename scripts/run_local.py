# 本地 Demo 入口：在终端运行完整的语音客服机器人，支持文本/麦克风两种输入模式。

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

from src.config import get_settings
from src.core.dialogue import ADDRESS_ONLY_STEPS, DEFAULT_FLOW_STEPS, DialogueEngine
from src.core.geocode import AMapGeocoder
from src.core.intent import IntentClassifier
from src.local.asr import ASRClient
from src.local.tts import TTSClient
from src.usage import UsageTracker


def build_parser() -> argparse.ArgumentParser:
    """构建 CLI 参数解析器。"""
    parser = argparse.ArgumentParser(description="语音客服机器人 Demo")
    parser.add_argument(
        "--text-mode",
        action="store_true",
        help="使用终端文字输入代替麦克风，便于本地联调。",
    )
    parser.add_argument(
        "--no-debug",
        action="store_true",
        help="关闭 ASR/意图/高德的中间结果打印。",
    )
    parser.add_argument(
        "--energy-threshold",
        type=float,
        default=None,
        help="覆盖本地 VAD 的能量阈值。",
    )
    parser.add_argument(
        "--address",
        action="store_true",
        help="仅启动地址询问流程，不询问前两个 yes/no 问题。",
    )
    parser.add_argument(
        "--tts",
        action="store_true",
        help="启用机器人语音播报，保留终端 print 的同时播放 TTS 音频。",
    )
    parser.add_argument(
        "--demo",
        action="store_true",
        help="演示模式：只显示对话内容，屏蔽所有调试和辅助信息。",
    )
    parser.add_argument(
        "--tts-voice",
        default=None,
        help="覆盖默认 TTS 音色，例如 longanyang。",
    )
    return parser


def select_steps(address_only: bool):
    """根据参数选择完整回访流程或仅地址核对。"""
    # demo 常用两种入口：完整回访流程 or 只演示地址核对。
    return ADDRESS_ONLY_STEPS if address_only else DEFAULT_FLOW_STEPS


def main() -> int:
    """
    主入口：解析参数 -> 加载配置 -> 组装引擎（ASR + Intent + Geocode + TTS） -> 运行对话。
    """
    parser = build_parser()
    args = parser.parse_args()
    settings = get_settings()

    demo = args.demo
    overrides = {
        "debug": not args.no_debug and not demo,
    }
    if args.energy_threshold is not None:
        overrides["vad_energy_threshold"] = args.energy_threshold
    if args.tts_voice:
        overrides["tts_voice"] = args.tts_voice
    settings = settings.override(**overrides)

    # 麦克风模式和 TTS 都依赖 DashScope；提前失败能避免跑到中途才报错。
    if not args.text_mode and not settings.dashscope_api_key:
        print(
            "未配置 DASHSCOPE_API_KEY，麦克风模式无法完成语音识别。"
            "可先使用 `python scripts/run_local.py --text-mode` 联调状态机。",
            file=sys.stderr,
        )
        return 1
    if args.tts and not settings.dashscope_api_key:
        print(
            "未配置 DASHSCOPE_API_KEY，无法启用 TTS 语音播报。",
            file=sys.stderr,
        )
        return 1

    tracker = UsageTracker()
    steps = select_steps(args.address)
    speaker = TTSClient(settings, tracker=tracker) if args.tts else None

    # 各模块在这里组装，后续替换成 Web/Gradio 入口时也可以复用同一套核心逻辑。
    engine = DialogueEngine(
        asr_client=ASRClient(settings, tracker=tracker),
        intent_classifier=IntentClassifier(
            settings,
            use_llm=True,
            tracker=tracker,
        ),
        geocoder=AMapGeocoder(settings),
        input_mode="text" if args.text_mode else "microphone",
        debug=settings.debug,
        speaker=speaker,
        steps=steps,
    )

    summary = engine.run()
    if not demo:
        print(json.dumps(summary, ensure_ascii=False, indent=2))
        tracker.print_summary()
    return 0 if summary.get("status") == "completed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
