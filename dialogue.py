from __future__ import annotations

import difflib
import json
import re
from dataclasses import dataclass
from typing import Any, Callable, Protocol

from asr import ASRClient
from geocode import AMapGeocoder
from intent import IntentClassifier
from pinyin_utils import normalize_text


@dataclass(frozen=True)
class DialogueStep:
    key: str
    question: str
    expected_intent: str
    retry_prompt: str

# 默认的对话步骤
DEFAULT_STEPS = [
    DialogueStep(
        key="appointment_confirmed",
        question="您好，请问已经有师傅和您预约上门时间了吗？",
        expected_intent="yes_no",
        retry_prompt="抱歉，我没有听清。请问是否已经预约上门时间了？",
    ),
    DialogueStep(
        key="service_satisfied",
        question="请问本次服务您满意吗？",
        expected_intent="yes_no",
        retry_prompt="抱歉，我再确认一下，您对本次服务是否满意？",
    ),
    DialogueStep(
        key="address",
        question="为了方便核对，请您说一下详细地址，尽量具体到门牌号。",
        expected_intent="address",
        retry_prompt="地址还不够清楚，请您再说一遍，尽量包含路名和门牌号。",
    ),
]
DEFAULT_FLOW_STEPS = tuple(DEFAULT_STEPS[:2])
ADDRESS_ONLY_STEPS = (DEFAULT_STEPS[2],)


class Speaker(Protocol):
    def speak(self, text: str) -> None: ...


class DialogueEngine:
    def __init__(
        self,
        asr_client: ASRClient,
        intent_classifier: IntentClassifier,
        geocoder: AMapGeocoder,
        *,
        input_mode: str = "microphone",
        debug: bool = True,
        input_func: Callable[[str], str] | None = None,
        speaker: Speaker | None = None,
        steps: tuple[DialogueStep, ...] | list[DialogueStep] | None = None,
        max_unclear_retries: int = 2,
        max_timeout_retries: int = 1,
        max_address_retries: int = 2,
    ) -> None:
        self.asr_client = asr_client
        self.intent_classifier = intent_classifier
        self.geocoder = geocoder
        self.input_mode = input_mode
        self.debug = debug
        self.input_func = input_func or input
        self.speaker = speaker
        self.steps = tuple(steps) if steps is not None else DEFAULT_FLOW_STEPS
        self.max_unclear_retries = max_unclear_retries
        self.max_timeout_retries = max_timeout_retries
        self.max_address_retries = max_address_retries

    def run(self) -> dict[str, Any]:
        # results 保存结构化结论，transcript 保存对话原文，便于后续调试或接 UI。
        summary: dict[str, Any] = {
            "status": "completed",
            "results": {},
            "transcript": [],
        }

        for step in self.steps:
            handled = self._run_step(step, summary)
            if not handled:
                summary["status"] = "terminated"
                break

        if summary["status"] == "completed":
            self._say("本次回访结束，感谢您的配合。")
        return summary

    def _run_step(self, step: DialogueStep, summary: dict[str, Any]) -> bool:
        unclear_retries = 0
        timeout_retries = 0
        address_retries = 0
        prompt = step.question

        while True:
            self._say(prompt)
            listen_result = self._listen()

            if listen_result["timed_out"]:
                # 无响应与"答非所问"分开处理，前者只允许一次唤醒重试。
                if timeout_retries < self.max_timeout_retries:
                    timeout_retries += 1
                    prompt = "您好，请问您还在吗？"
                    continue

                summary["results"][step.key] = {"status": "timeout"}
                summary["transcript"].append(
                    {"speaker": "bot", "text": prompt},
                )
                self._say("长时间未收到回应，本次回访先结束。")
                return False

            user_text = listen_result["text"].strip()
            summary["transcript"].append({"speaker": "bot", "text": prompt})
            summary["transcript"].append({"speaker": "user", "text": user_text})

            if self.debug:
                print(f"[ASR] {user_text}")

            context = {
                "stage": step.key,
                "expected_intent": step.expected_intent,
                "question": prompt,
            }
            classified = self.intent_classifier.classify(user_text, context)

            if self.debug:
                print(f"[INTENT] {json.dumps(classified, ensure_ascii=False)}")

            intent = classified.get("intent", "unclear")
            if step.expected_intent == "yes_no" and intent in {"yes", "no"}:
                summary["results"][step.key] = {
                    "status": "ok",
                    "intent": intent,
                    "text": user_text,
                }
                reply = self._build_yes_no_reply(step.key, intent)
                self._say(reply)
                return True

            if step.expected_intent == "address":
                search_text = classified.get("address") or user_text
                search_result = self.geocoder.resolve_place(search_text)
                if self.debug:
                    print(f"[ADDRESS] {json.dumps(search_result, ensure_ascii=False)}")

                error = search_result.get("error", "")
                if error.startswith(("未配置 AMAP_KEY", "未安装 requests")):
                    summary["results"][step.key] = {
                        "status": "unverified",
                        "text": user_text,
                        "intent": "address",
                    }
                    self._say("好的，地址已记录。当前环境暂时无法完成地点搜索。")
                    return True

                if search_result.get("found") and search_result.get("best"):
                    top = search_result["best"]
                    if self._address_needs_confirmation(search_text, top):
                        confirmed = self._confirm_address_candidate(search_text, top, summary)
                        if confirmed:
                            summary["results"][step.key] = {
                                "status": "ok",
                                "text": user_text,
                                "intent": "address",
                                "place": top,
                            }
                            self._say("好的，地址已记录。")
                            return True
                    else:
                        summary["results"][step.key] = {
                            "status": "ok",
                            "text": user_text,
                            "intent": "address",
                            "place": top,
                        }
                        self._say("好的，地址已记录。")
                        return True

                if address_retries < self.max_address_retries:
                    address_retries += 1
                    prompt = "我还没完全核对到这个地址，请再详细说一下，尽量包含小区、路名和门牌号。"
                    continue

                summary["results"][step.key] = {
                    "status": "not_found",
                    "text": user_text,
                    "intent": "address",
                }
                self._say("好的，地址已记录，后续会由人工进一步核实。")
                return True

            if unclear_retries < self.max_unclear_retries:
                unclear_retries += 1
                prompt = step.retry_prompt
                continue

            summary["results"][step.key] = {
                "status": "unclear",
                "text": user_text,
                "intent": intent,
            }
            self._say("好的，这一项我先为您标记为待确认。")
            return True

    def _confirm_place(self, place: dict[str, Any], summary: dict[str, Any]) -> bool:
        name = place.get("name", "")
        addr = place.get("address", "")
        display = f"{name}（{addr}）" if addr else name
        prompt = f"请问您说的是 {display} 吗？"
        self._say(prompt)
        listen_result = self._listen()
        summary["transcript"].append({"speaker": "bot", "text": prompt})
        if listen_result["timed_out"]:
            return False
        user_text = listen_result["text"].strip()
        summary["transcript"].append({"speaker": "user", "text": user_text})
        if self.debug:
            print(f"[ASR] {user_text}")
        context = {"stage": "address_confirm", "expected_intent": "yes_no"}
        classified = self.intent_classifier.classify(user_text, context)
        if self.debug:
            print(f"[INTENT] {json.dumps(classified, ensure_ascii=False)}")
        return classified.get("intent") == "yes"

    def _confirm_address_candidate(
        self,
        original_text: str,
        candidate: dict[str, Any],
        summary: dict[str, Any],
    ) -> bool:
        # 先抽出“用户原话”和“系统候选”真正不一致的局部，再交给 LLM 生成自然确认话术。
        focus_text = self._extract_address_difference(
            original_text,
            self._meaningful_candidate_text(candidate),
        )
        prompt = self.intent_classifier.generate_address_confirmation_prompt(
            original_text=original_text,
            matched_text=self._meaningful_candidate_text(candidate),
            matched_name=candidate.get("name", ""),
            focus_text=focus_text,
            fallback_prompt=self._build_address_confirmation_prompt(original_text, candidate),
        )
        self._say(prompt)
        listen_result = self._listen()
        summary["transcript"].append({"speaker": "bot", "text": prompt})
        if listen_result["timed_out"]:
            return False
        user_text = listen_result["text"].strip()
        summary["transcript"].append({"speaker": "user", "text": user_text})
        if self.debug:
            print(f"[ASR] {user_text}")
        context = {"stage": "address_confirm", "expected_intent": "yes_no"}
        classified = self.intent_classifier.classify(user_text, context)
        if self.debug:
            print(f"[INTENT] {json.dumps(classified, ensure_ascii=False)}")
        return classified.get("intent") == "yes"

    @staticmethod
    def _address_needs_confirmation(original_text: str, candidate: dict[str, Any]) -> bool:
        original = normalize_text(original_text)
        compare_basis = DialogueEngine._meaningful_candidate_text(candidate)
        compare_text = normalize_text(compare_basis)
        if not original or not compare_text:
            return False
        if compare_text in original:
            return False
        return original != compare_text

    @staticmethod
    def _build_address_confirmation_prompt(original_text: str, candidate: dict[str, Any]) -> str:
        name = str(candidate.get("name", "")).strip()
        compare_text = DialogueEngine._meaningful_candidate_text(candidate)
        differing_text = DialogueEngine._extract_address_difference(original_text, compare_text)
        min_len = max(4, len(compare_text) - 2)
        if DialogueEngine._is_named_place(name):
            # 对“公司/店/公寓/小区”这类命名地点，单念路名楼名不够像真人客服，
            # 所以规则兜底也优先走“名称 + 地址”确认。
            location = DialogueEngine._address_without_name(
                differing_text if DialogueEngine._is_good_address_fragment(differing_text, min_len=min_len) else compare_text,
                name,
            )
            if location:
                return f"请问是{name}，地址在{location}吗？"
            return f"请问是{name}吗？"
        if DialogueEngine._is_good_address_fragment(differing_text, min_len=min_len):
            return f"我核对到的详细位置像是“{differing_text}”，请问对吗？"
        return f"我核对到的地址是“{compare_text}”，请问对吗？"

    @staticmethod
    def _extract_address_difference(original_text: str, candidate_text: str) -> str:
        original = normalize_text(original_text)
        candidate = normalize_text(candidate_text)
        if not original or not candidate or original == candidate:
            return ""

        matcher = difflib.SequenceMatcher(a=original, b=candidate)
        fragments: list[str] = []
        for tag, _, _, j1, j2 in matcher.get_opcodes():
            if tag == "equal":
                continue
            fragment = candidate[j1:j2].strip()
            if fragment:
                fragments.append(fragment)

        if not fragments:
            return ""
        return max(fragments, key=len)

    @staticmethod
    def _is_good_address_fragment(text: str, *, min_len: int = 4) -> bool:
        if not text or len(text) < min_len:
            return False
        return any(token in text for token in {"路", "街", "巷", "号", "村", "苑", "厦", "城", "园"})

    @staticmethod
    def _is_named_place(name: str) -> bool:
        stripped = name.strip()
        if not stripped:
            return False
        return any(
            token in stripped
            for token in {"公司", "店", "中心", "广场", "大厦", "公寓", "小区", "苑", "城", "园"}
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

    @staticmethod
    def _meaningful_candidate_text(candidate: dict[str, Any]) -> str:
        text = (
            candidate.get("display_text")
            or candidate.get("formatted")
            or candidate.get("compare_text")
            or ""
        ).strip()
        if not text:
            return ""

        suffix_pattern = re.compile(r".+?(?:省|市|区|县|镇|乡|街道)")
        prefix_end = 0
        for match in suffix_pattern.finditer(text):
            prefix_end = match.end()

        if 0 < prefix_end < len(text):
            # 地址确认只关心真正需要核对的局部，省/市/区/街道这类前缀通常是噪声。
            trimmed = text[prefix_end:].strip()
            if trimmed:
                return trimmed
        return text

    def _listen(self) -> dict[str, Any]:
        if self.input_mode == "text":
            # 文本模式用于本地调试，不依赖麦克风和外部 ASR。
            text = self.input_func("用户：").strip()
            return {
                "timed_out": not bool(text),
                "text": text,
            }

        return self.asr_client.listen_once()

    def _say(self, text: str) -> None:
        print(f"机器人：{text}")
        if self.speaker is None:
            return

        try:
            self.speaker.speak(text)
        except Exception as exc:
            if self.debug:
                print(f"[TTS] 语音播报失败，已回退为纯文本输出: {exc}")

    @staticmethod
    def _build_yes_no_reply(step_key: str, intent: str) -> str:
        if step_key == "service_satisfied":
            if intent == "yes":
                return "好的，记录到您比较满意。"
            return "抱歉，给您造成了不愉快的体验。"

        return "好的，已经为您记录。" if intent == "yes" else "好的，已经记录您这边还没有。"
