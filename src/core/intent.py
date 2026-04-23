from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass
from typing import Any

from src.config import Settings


YES_HINTS = {
    "有",
    "有的",
    "好的",
    "好",
    "嗯",
    "嗯嗯",
    "是",
    "是的",
    "对",
    "对的",
    "没错",
    "收到",
    "收到了",
    "满意",
    "解决了",
    "还行",
    "还可以",
    "不错",
    "挺好",
    "可以",
}
NO_HINTS = {
    "没",
    "没有",
    "还没",
    "不是",
    "不满意",
    "没收到",
    "不行",
    "不对",
    "未解决",
}
ADDRESS_HINTS = {
    "路",
    "街",
    "巷",
    "弄",
    "号",
    "栋",
    "单元",
    "室",
    "小区",
    "广场",
    "大厦",
    "村",
    "镇",
    "乡",
    "省",
    "市",
    "区",
    "县",
}
UNCLEAR_HINTS = {"啊", "哈", "喂", "你说什么", "什么意思", "没听清", "再说一遍"}


@dataclass
class IntentResult:
    intent: str
    address: str = ""
    confidence: str = "medium"
    source: str = "llm"
    raw_text: str = ""

    def to_dict(self) -> dict[str, str]:
        return {
            "intent": self.intent,
            "address": self.address,
            "confidence": self.confidence,
            "source": self.source,
            "raw_text": self.raw_text,
        }


class IntentClassifier:
    def __init__(self, settings: Settings, use_llm: bool = True, tracker: Any = None) -> None:
        self.settings = settings
        self.use_llm = use_llm
        self.tracker = tracker

    def classify(self, text: str, context: dict[str, Any]) -> dict[str, str]:
        cleaned_text = text.strip()
        if not cleaned_text:
            return IntentResult(
                intent="unclear",
                confidence="low",
                source="heuristic",
                raw_text=text,
            ).to_dict()

        # 本地联调或未配置 key 时，自动退化到规则判断，保证状态机仍可测试。
        if not self.use_llm or not self.settings.dashscope_api_key:
            return self._heuristic_classify(cleaned_text, context).to_dict()

        try:
            return self._classify_with_llm(cleaned_text, context).to_dict()
        except Exception as exc:
            if self.settings.debug:
                print(f"[INTENT] LLM 调用失败，回退到启发式: {exc}")
            return self._heuristic_classify(cleaned_text, context).to_dict()

    def generate_address_confirmation_prompt(
        self,
        *,
        original_text: str,
        matched_text: str,
        matched_name: str = "",
        focus_text: str = "",
        fallback_prompt: str = "",
    ) -> str:
        if not self.use_llm or not self.settings.dashscope_api_key:
            return fallback_prompt or self._fallback_address_prompt(
                focus_text or matched_text,
                matched_name=matched_name,
            )

        try:
            prompt = self._generate_address_confirmation_with_llm(
                original_text=original_text,
                matched_text=matched_text,
                matched_name=matched_name,
                focus_text=focus_text,
            )
        except Exception as exc:
            if self.settings.debug:
                print(f"[INTENT] 地址确认话术生成失败，回退到规则话术: {exc}")
            prompt = ""

        prompt = self._clean_confirmation_prompt(prompt)
        # LLM 有时会只复述楼名或路名。对有明确名称的地点，这里再做一层硬兜底，
        # 确保最终播报里带上“公司/门店/小区”等关键称呼。
        prompt = self._ensure_named_place_prompt(
            prompt,
            matched_text=matched_text,
            matched_name=matched_name,
            focus_text=focus_text,
        )
        return prompt or fallback_prompt or self._fallback_address_prompt(
            focus_text or matched_text,
            matched_name=matched_name,
        )

    def _classify_with_llm(self, text: str, context: dict[str, Any]) -> IntentResult:
        os.environ["DASHSCOPE_API_KEY"] = self.settings.dashscope_api_key

        expected = context.get("expected_intent", "generic")
        stage = context.get("stage", "unknown")
        question = context.get("question", "")
        system_prompt, user_prompt = self._build_llm_prompts(
            text=text,
            stage=stage,
            expected=expected,
            question=question,
        )

        response = self._call_dashscope_llm(
            system_prompt=system_prompt,
            user_prompt=user_prompt,
            expected=expected,
        )

        status_code = getattr(response, "status_code", 200)
        if status_code != 200:
            raise RuntimeError(f"意图分类失败: {response}")

        self._record_usage(response)

        content = self._extract_generation_content(response)
        parsed = self._parse_json_content(content)
        return IntentResult(
            intent=parsed.get("intent", "unclear"),
            address=parsed.get("address", ""),
            confidence="high",
            source="llm",
            raw_text=content,
        )

    def _call_dashscope_llm(
        self,
        *,
        system_prompt: str,
        user_prompt: str,
        expected: str,
    ) -> Any:
        try:
            from dashscope import Generation, MultiModalConversation
        except ImportError as exc:  # pragma: no cover - dependency check
            raise RuntimeError("未安装 dashscope，无法调用意图分类。") from exc

        options = self._build_llm_options(expected)
        if self._should_use_multimodal_api(self.settings.llm_model):
            return MultiModalConversation.call(
                model=self.settings.llm_model,
                messages=[
                    {"role": "system", "content": [{"text": system_prompt}]},
                    {"role": "user", "content": [{"text": user_prompt}]},
                ],
                **options,
            )

        return Generation.call(
            model=self.settings.llm_model,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            result_format="message",
            **options,
        )

    def _generate_address_confirmation_with_llm(
        self,
        *,
        original_text: str,
        matched_text: str,
        matched_name: str,
        focus_text: str,
    ) -> str:
        os.environ["DASHSCOPE_API_KEY"] = self.settings.dashscope_api_key

        system_prompt = (
            "你是电话地址确认助手。"
            "你的任务是生成一句简短、自然、口语化的中文确认话术。"
            "只确认真正不确定的局部，不要确认省、市、区这类常识信息。"
            "禁止解释、禁止输出多句、禁止输出引号、禁止输出编号。"
            "只输出一句可以直接念给用户听的话。"
        )
        user_prompt = (
            f"用户原话: {original_text}\n"
            f"系统匹配地址: {matched_text}\n"
            f"系统匹配名称: {matched_name or '无'}\n"
            f"建议重点确认的局部: {focus_text or '无'}\n"
            "要求:\n"
            "1. 如果存在村名、路名、门牌号、小区名差异，只确认这些局部。\n"
            "2. 不要问“广东省/广州市/海珠区这类大范围信息对不对”。\n"
            "3. 话术尽量像真人客服，控制在 25 个字以内。\n"
            "4. 如果 focus_text 已经足够具体，优先围绕它来问。\n"
            "5. 不要照抄用户原话里的错别字，优先使用系统匹配地址中的名称。\n"
            "6. 如果系统匹配名称里有公司名、店名、小区名、楼盘名，优先把这个名称说出来再确认。\n"
            "7. 对公司类地点，优先生成“您说的是XX公司，地址在XX，对吗？”这种话术。\n"
            "只输出一句最终话术。"
        )

        response = self._call_text_llm(
            system_prompt=system_prompt,
            user_prompt=user_prompt,
            max_tokens=64,
        )
        self._record_usage(response)
        return self._extract_generation_content(response).strip()

    def _call_text_llm(
        self,
        *,
        system_prompt: str,
        user_prompt: str,
        max_tokens: int,
    ) -> Any:
        try:
            from dashscope import Generation, MultiModalConversation
        except ImportError as exc:  # pragma: no cover - dependency check
            raise RuntimeError("未安装 dashscope，无法调用地址确认话术生成。") from exc

        options = {
            "temperature": 0.2,
            "max_tokens": max_tokens,
            "enable_thinking": False,
        }
        if self._should_use_multimodal_api(self.settings.llm_model):
            return MultiModalConversation.call(
                model=self.settings.llm_model,
                messages=[
                    {"role": "system", "content": [{"text": system_prompt}]},
                    {"role": "user", "content": [{"text": user_prompt}]},
                ],
                **options,
            )

        return Generation.call(
            model=self.settings.llm_model,
            messages=[
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            result_format="message",
            **options,
        )

    @staticmethod
    def _build_llm_prompts(*, text: str, stage: str, expected: str, question: str = "") -> tuple[str, str]:
        if expected == "yes_no":
            system_prompt = (
                "你是严格的意图分类器。"
                "禁止解释、禁止补充、禁止输出 markdown、禁止输出思考过程。"
                '你只能输出且必须输出一个最短 JSON：{"intent":"yes"}、{"intent":"no"} 或 {"intent":"unclear"}。'
            )
            question_line = f"机器人问题: {question}\n" if question else ""
            user_prompt = (
                f"当前阶段: {stage}\n"
                f"{question_line}"
                "任务: 仅判断用户对上述问题的真实意图是肯定、否定还是无法判断。\n"
                "注意: 反问句('满意？你觉得我满意吗？')、反讽、质问语气通常表达的是相反意图，请识别真实态度而非字面关键词。\n"
                f"用户文本: {text}\n"
                "只返回一个最短 JSON，然后立刻结束。"
            )
            return system_prompt, user_prompt

        if expected == "address":
            system_prompt = (
                "你是严格的地址提取与意图分类器。"
                "禁止解释、禁止补充、禁止输出 markdown、禁止输出思考过程。"
                '你只能输出且必须输出一个最短 JSON：{"intent":"address","address":"详细地址"}'
                ' 或 {"intent":"unclear","address":""}。'
            )
            user_prompt = (
                f"当前阶段: {stage}\n"
                "任务: 如果用户在提供地址，则提取尽可能完整的地址；否则返回 unclear。\n"
                f"用户文本: {text}\n"
                "只返回一个最短 JSON，然后立刻结束。"
            )
            return system_prompt, user_prompt

        system_prompt = (
            "你是严格的意图分类器。"
            "禁止解释、禁止补充、禁止输出 markdown、禁止输出思考过程。"
            '你只能输出且必须输出一个 JSON：{"intent":"yes|no|address|unclear","address":"提取出的地址或空字符串"}。'
        )
        user_prompt = (
            f"当前阶段: {stage}\n"
            f"期望意图: {expected}\n"
            f"用户文本: {text}\n"
            "只返回一个 JSON，然后立刻结束。"
        )
        return system_prompt, user_prompt

    @staticmethod
    def _build_llm_options(expected: str) -> dict[str, Any]:
        max_tokens = 20 if expected == "yes_no" else 64
        return {
            "temperature": 0,
            "max_tokens": max_tokens,
            "enable_thinking": False,
        }

    @staticmethod
    def _should_use_multimodal_api(model: str) -> bool:
        normalized = model.strip().lower()
        return normalized.startswith("qwen3.5")

    def _heuristic_classify(self, text: str, context: dict[str, Any]) -> IntentResult:
        normalized = re.sub(r"\s+", "", text)

        if self._looks_unclear(normalized):
            return IntentResult("unclear", confidence="low", source="heuristic", raw_text=text)

        # 地址优先级高于 yes/no，避免"朝阳区"这类文本被误判成普通肯否回答。
        if self._looks_like_address(normalized):
            extracted = self._extract_address(normalized)
            if extracted:
                return IntentResult(
                    intent="address",
                    address=extracted,
                    confidence="medium",
                    source="heuristic",
                    raw_text=text,
                )

        # 否定感知打分：被否定前缀（不/不太/没/未/别）修饰的肯定词计入 no_score。
        no_score = sum(1 for token in NO_HINTS if token in normalized)
        yes_score = 0
        for token in YES_HINTS:
            if token in normalized:
                idx = normalized.find(token)
                if self._is_negated(normalized, idx):
                    no_score += 1
                else:
                    yes_score += 1

        if yes_score > no_score and yes_score > 0:
            return IntentResult("yes", confidence="medium", source="heuristic", raw_text=text)
        if no_score > 0:
            return IntentResult("no", confidence="medium", source="heuristic", raw_text=text)

        if self._looks_like_address(normalized):
            return IntentResult(
                intent="address",
                address=self._extract_address(normalized),
                confidence="medium",
                source="heuristic",
                raw_text=text,
            )

        return IntentResult("unclear", confidence="low", source="heuristic", raw_text=text)

    @staticmethod
    def _is_negated(text: str, idx: int) -> bool:
        for prefix in ("不太", "不", "没", "未", "别"):
            plen = len(prefix)
            if idx >= plen and text[idx - plen : idx] == prefix:
                return True
        return False

    @staticmethod
    def _looks_unclear(text: str) -> bool:
        if any(hint in text for hint in UNCLEAR_HINTS):
            return True
        if len(text) <= 1 and text not in YES_HINTS and text not in NO_HINTS:
            return True
        return False

    @staticmethod
    def _looks_like_address(text: str) -> bool:
        has_digits = bool(re.search(r"\d", text))
        has_region = sum(1 for hint in ADDRESS_HINTS if hint in text) >= 2
        return has_digits or has_region

    @staticmethod
    def _extract_address(text: str) -> str:
        return text.strip("，。,. ")

    @staticmethod
    def _extract_generation_content(response: Any) -> str:
        output = getattr(response, "output", None)
        choices = getattr(output, "choices", None)
        if not choices:
            return ""

        message = getattr(choices[0], "message", None) or choices[0].get("message", {})
        content = getattr(message, "content", None)
        if content is None and isinstance(message, dict):
            content = message.get("content")

        if isinstance(content, str):
            return content
        if isinstance(content, list):
            fragments: list[str] = []
            for item in content:
                if isinstance(item, dict):
                    value = item.get("text") or item.get("value")
                    if value:
                        fragments.append(str(value))
                elif isinstance(item, str):
                    fragments.append(item)
            return "\n".join(fragments)
        return ""

    @staticmethod
    def _parse_json_content(content: str) -> dict[str, str]:
        try:
            parsed = json.loads(content)
            if isinstance(parsed, dict):
                return {
                    "intent": str(parsed.get("intent", "unclear")),
                    "address": str(parsed.get("address", "")),
                }
        except json.JSONDecodeError:
            pass

        match = re.search(r"\{.*\}", content, re.DOTALL)
        if not match:
            return {"intent": "unclear", "address": ""}

        try:
            # 有些模型会在 JSON 前后补解释文本，这里做一次二次截取。
            parsed = json.loads(match.group(0))
        except json.JSONDecodeError:
            return {"intent": "unclear", "address": ""}

        return {
            "intent": str(parsed.get("intent", "unclear")),
            "address": str(parsed.get("address", "")),
        }

    def _record_usage(self, response: Any) -> None:
        usage = getattr(response, "usage", None)
        if not self.tracker or not usage:
            return

        input_tok = getattr(usage, "input_tokens", 0)
        output_tok = getattr(usage, "output_tokens", None)
        if output_tok is None:
            total_tok = getattr(usage, "total_tokens", 0)
            output_tok = max(0, total_tok - input_tok)
        self.tracker.add(
            "llm",
            model=self.settings.llm_model,
            input_tokens=input_tok,
            output_tokens=output_tok,
        )

    @staticmethod
    def _fallback_address_prompt(focus_text: str, *, matched_name: str = "") -> str:
        cleaned = focus_text.strip() or "这个地址"
        if IntentClassifier._is_named_place(matched_name):
            location = IntentClassifier._address_without_name(cleaned, matched_name)
            if location:
                return f"请问是{matched_name.strip()}，地址在{location}吗？"
            return f"请问是{matched_name.strip()}吗？"
        return f"我核对到的详细位置像是“{cleaned}”，请问对吗？"

    @staticmethod
    def _clean_confirmation_prompt(prompt: str) -> str:
        cleaned = prompt.strip().strip("\"'“”")
        cleaned = re.sub(r"^\d+[.)、]\s*", "", cleaned)
        return cleaned.splitlines()[0].strip() if cleaned else ""

    @staticmethod
    def _ensure_named_place_prompt(
        prompt: str,
        *,
        matched_text: str,
        matched_name: str,
        focus_text: str,
    ) -> str:
        if not IntentClassifier._is_named_place(matched_name):
            return prompt
        if matched_name.strip() in prompt:
            return prompt

        # 一旦识别出候选本身是“有名字的地方”，优先回到稳定模板，
        # 避免模型把确认问成只有地址、没有地点名的半截话。
        location = IntentClassifier._address_without_name(
            focus_text.strip() or matched_text.strip(),
            matched_name,
        )
        if location:
            return f"请问是{matched_name.strip()}，地址在{location}吗？"
        return f"请问是{matched_name.strip()}吗？"

    @staticmethod
    def _is_named_place(name: str) -> bool:
        stripped = name.strip()
        if not stripped:
            return False
        return any(
            token in stripped
            for token in ("公司", "店", "中心", "广场", "大厦", "公寓", "小区", "苑", "城", "园")
        )

    @staticmethod
    def _address_without_name(text: str, name: str) -> str:
        cleaned = text.strip()
        stripped_name = name.strip()
        if not cleaned:
            return ""
        if stripped_name:
            cleaned = cleaned.replace(stripped_name, "", 1).strip(" ，,。；;：:")
        return cleaned
