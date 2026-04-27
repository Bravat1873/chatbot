# API 用量统计：记录 LLM/ASR/TTS 调用次数与 Token 消耗，估算费用。

from __future__ import annotations

from dataclasses import dataclass

# 官方定价参考 https://help.aliyun.com/zh/model-studio/billing-for-model-studio
PRICING: dict[str, dict[str, float]] = {
    # 文本生成 (元/百万Token)
    "qwen3.5-flash": {"input_per_mtok": 0.2, "output_per_mtok": 2.0},
    "qwen3.5-plus": {"input_per_mtok": 0.8, "output_per_mtok": 4.8},
    "qwen3-max": {"input_per_mtok": 2.5, "output_per_mtok": 10.0},
    # 实时语音识别 (元/秒)
    "qwen3-asr-flash-realtime": {"per_second": 0.00033},
    "qwen3-asr-flash-filetrans": {"per_second": 0.00022},
    # 语音合成 (元/万字符)
    "cosyvoice-v3-flash": {"input_per_10k_chars": 1.0},
    "qwen3-tts-flash-realtime": {"input_per_10k_chars": 1.0},
    "qwen3-tts-flash": {"input_per_10k_chars": 0.8},
}



@dataclass
class UsageRecord:
    """单次 API 调用记录。"""
    service: str
    model: str = ""
    input_tokens: int = 0
    output_tokens: int = 0
    audio_seconds: float = 0.0
    input_chars: int = 0


class UsageTracker:
    """用量追踪器：汇总各模型调用次数、Token、音频时长并估算费用。"""
    def __init__(self) -> None:
        self.records: list[UsageRecord] = []

    def add(
        self,
        service: str,
        *,
        model: str = "",
        input_tokens: int = 0,
        output_tokens: int = 0,
        audio_seconds: float = 0.0,
        input_chars: int = 0,
    ) -> None:
        self.records.append(UsageRecord(
            service=service,
            model=model,
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            audio_seconds=audio_seconds,
            input_chars=input_chars,
        ))

    def print_summary(self) -> None:
        if not self.records:
            return

        # 先按模型聚合，方便 demo 时直接看到“哪一层最花钱”。
        by_key: dict[str, dict] = {}
        for r in self.records:
            key = r.model or r.service
            if key not in by_key:
                by_key[key] = {
                    "calls": 0,
                    "input_tokens": 0,
                    "output_tokens": 0,
                    "audio_seconds": 0.0,
                    "input_chars": 0,
                }
            s = by_key[key]
            s["calls"] += 1
            s["input_tokens"] += r.input_tokens
            s["output_tokens"] += r.output_tokens
            s["audio_seconds"] += r.audio_seconds
            s["input_chars"] += r.input_chars

        print("\n" + "=" * 50)
        print("  API 用量统计")
        print("=" * 50)
        total_cost = 0.0
        for model, stats in by_key.items():
            cost = _estimate_cost(model, stats)
            total_cost += cost
            parts = [f"{stats['calls']} 次"]
            if stats["input_tokens"]:
                parts.append(f"输入 {stats['input_tokens']} tok")
            if stats["output_tokens"]:
                parts.append(f"输出 {stats['output_tokens']} tok")
            if stats["audio_seconds"] > 0:
                parts.append(f"音频 {stats['audio_seconds']:.1f}s")
            if stats["input_chars"] > 0:
                parts.append(f"字符 {stats['input_chars']}")
            parts.append(f"¥{cost:.6f}")
            print(f"  {model}: {' | '.join(parts)}")
        print(f"  {'─' * 46}")
        print(f"  总计: ¥{total_cost:.6f}")
        print("=" * 50)


def _estimate_cost(model: str, stats: dict) -> float:
    # 本地只是做粗略估算，不保证和平台账单逐分一致，但足够支持 demo 成本讨论。
    pricing = PRICING.get(model, {})
    cost = 0.0
    if "input_per_mtok" in pricing:
        cost += stats["input_tokens"] / 1_000_000 * pricing["input_per_mtok"]
    if "output_per_mtok" in pricing:
        cost += stats["output_tokens"] / 1_000_000 * pricing["output_per_mtok"]
    if "per_second" in pricing:
        cost += stats["audio_seconds"] * pricing["per_second"]
    if "input_per_10k_chars" in pricing:
        cost += stats["input_chars"] / 10_000 * pricing["input_per_10k_chars"]
    return cost
