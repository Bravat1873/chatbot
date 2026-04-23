from __future__ import annotations

import difflib
import json
import re
from dataclasses import dataclass
from typing import Any, Callable, Protocol
from uuid import uuid4

from src.core.geocode import AMapGeocoder
from src.core.intent import IntentClassifier
from src.core.pinyin_utils import normalize_text
from src.core.session import SessionManager, SessionState


END_MESSAGE = "本次回访结束，感谢您的配合。"
TIMEOUT_END_MESSAGE = "长时间未收到回应，本次回访先结束。"
TIMEOUT_RETRY_PROMPT = "您好，请问您还在吗？"
ADDRESS_RETRY_PROMPT = "我还没完全核对到这个地址，请再详细说一下，尽量包含小区、路名和门牌号。"


@dataclass(frozen=True)
class DialogueStep:
    key: str
    question: str
    expected_intent: str
    retry_prompt: str


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


class Listener(Protocol):
    def listen_once(self) -> dict[str, Any]: ...


class DialogueEngine:
    def __init__(
        self,
        intent_classifier: IntentClassifier,
        geocoder: AMapGeocoder,
        *,
        asr_client: Listener | None = None,
        speaker: Speaker | None = None,
        input_mode: str | None = None,
        debug: bool = True,
        input_func: Callable[[str], str] | None = None,
        steps: tuple[DialogueStep, ...] | list[DialogueStep] | None = None,
        max_unclear_retries: int = 2,
        max_timeout_retries: int = 1,
        max_address_retries: int = 2,
    ) -> None:
        self.asr_client = asr_client
        self.intent_classifier = intent_classifier
        self.geocoder = geocoder
        self.speaker = speaker
        self.input_mode = input_mode or "microphone"
        self.debug = debug
        self.input_func = input_func or input
        self.steps = tuple(steps) if steps is not None else DEFAULT_FLOW_STEPS
        self.max_unclear_retries = max_unclear_retries
        self.max_timeout_retries = max_timeout_retries
        self.max_address_retries = max_address_retries
        self.session_manager = SessionManager()

    def process_turn(self, session_id: str, user_text: str, biz_params: dict[str, Any] | None = None) -> str:
        state = self.session_manager.get_or_create(session_id)
        if biz_params is not None:
            state.biz_params = dict(biz_params)
        if state.finished:
            return END_MESSAGE

        step = self._current_step(state)
        if step is None:
            state.finished = True
            state.status = "completed"
            return END_MESSAGE

        cleaned_text = user_text.strip()
        if not cleaned_text:
            prompt = self._current_prompt(state)
            return self._record_bot_reply(state, prompt)

        if state.awaiting_address_confirm:
            return self._handle_address_confirmation(state, cleaned_text)

        prompt = self._current_prompt(state)
        self._ensure_prompt_recorded(state, prompt)
        self._record_user_turn(state, cleaned_text)

        if cleaned_text == "用户没有说话":
            return self._handle_silence(state, step)

        context = {
            "stage": step.key,
            "expected_intent": step.expected_intent,
            "question": prompt,
        }
        classified = self.intent_classifier.classify(cleaned_text, context)
        if self.debug:
            print(f"[INTENT] {json.dumps(classified, ensure_ascii=False)}")

        if step.expected_intent == "yes_no":
            return self._handle_yes_no_step(state, step, cleaned_text, classified)
        if step.expected_intent == "address":
            return self._handle_address_step(state, cleaned_text, classified)

        return self._complete_unclear_step(state, step, cleaned_text, classified.get("intent", "unclear"))

    def run(self) -> dict[str, Any]:
        session_id = f"local:{uuid4().hex}"
        try:
            self._say(self.process_turn(session_id, ""))
            while True:
                state = self.session_manager.get_or_create(session_id)
                if state.finished:
                    return self._build_summary(state)

                listen_result = self._listen()
                user_text = listen_result["text"].strip()
                if listen_result["timed_out"]:
                    user_text = "用户没有说话"
                elif self.input_mode != "text":
                    if self.debug:
                        print(f"[ASR] {user_text}")
                    else:
                        print(f"用户：{user_text}")

                reply = self.process_turn(session_id, user_text)
                self._say(reply)
        finally:
            self.session_manager.remove(session_id)

    def _handle_yes_no_step(
        self,
        state: SessionState,
        step: DialogueStep,
        user_text: str,
        classified: dict[str, Any],
    ) -> str:
        intent = classified.get("intent", "unclear")
        if intent in {"yes", "no"}:
            state.results[step.key] = {
                "status": "ok",
                "intent": intent,
                "text": user_text,
            }
            return self._advance_with_reply(state, self._build_yes_no_reply(step.key, intent))

        if state.unclear_retries < self.max_unclear_retries:
            state.unclear_retries += 1
            return self._record_bot_reply(state, step.retry_prompt)

        return self._complete_unclear_step(state, step, user_text, intent)

    def _handle_address_step(
        self,
        state: SessionState,
        user_text: str,
        classified: dict[str, Any],
    ) -> str:
        search_text = classified.get("address") or user_text
        verifying_hint = "好的，我现在帮您核实一下地址。"
        self._record_bot_reply(state, verifying_hint)
        self._say(verifying_hint)
        search_result = self.geocoder.resolve_place(search_text)
        if self.debug:
            print(f"[ADDRESS] {json.dumps(search_result, ensure_ascii=False)}")

        error = search_result.get("error", "")
        if error.startswith(("未配置 AMAP_KEY", "未安装 requests")):
            state.results["address"] = {
                "status": "unverified",
                "text": user_text,
                "intent": "address",
            }
            return self._advance_with_reply(state, "好的，地址已记录。当前环境暂时无法完成地点搜索。")

        if search_result.get("found") and search_result.get("best"):
            candidate = search_result["best"]
            if self._address_needs_confirmation(search_text, candidate):
                state.awaiting_address_confirm = True
                state.pending_address_candidate = candidate
                state.pending_address_text = user_text
                prompt = self._build_address_confirmation_turn(search_text, candidate)
                return self._record_bot_reply(state, prompt)

            state.results["address"] = {
                "status": "ok",
                "text": user_text,
                "intent": "address",
                "place": candidate,
            }
            return self._advance_with_reply(state, "好的，地址已记录。")

        if state.address_retries < self.max_address_retries:
            state.address_retries += 1
            return self._record_bot_reply(state, ADDRESS_RETRY_PROMPT)

        state.results["address"] = {
            "status": "not_found",
            "text": user_text,
            "intent": "address",
        }
        return self._advance_with_reply(state, "好的，地址已记录，后续会由人工进一步核实。")

    def _handle_address_confirmation(self, state: SessionState, user_text: str) -> str:
        prompt = self._current_prompt(state)
        self._ensure_prompt_recorded(state, prompt)
        self._record_user_turn(state, user_text)

        candidate = state.pending_address_candidate
        original_text = state.pending_address_text
        if candidate is None:
            state.awaiting_address_confirm = False
            state.pending_address_text = ""
            return self._record_bot_reply(state, ADDRESS_RETRY_PROMPT)

        if user_text == "用户没有说话":
            return self._handle_address_confirmation_rejected(state, original_text)

        classified = self.intent_classifier.classify(
            user_text,
            {"stage": "address_confirm", "expected_intent": "yes_no", "question": prompt},
        )
        if self.debug:
            print(f"[INTENT] {json.dumps(classified, ensure_ascii=False)}")

        state.awaiting_address_confirm = False
        state.pending_address_candidate = None
        state.pending_address_text = ""
        if classified.get("intent") == "yes":
            state.results["address"] = {
                "status": "ok",
                "text": original_text,
                "intent": "address",
                "place": candidate,
            }
            return self._advance_with_reply(state, "好的，地址已记录。")

        return self._handle_address_confirmation_rejected(state, original_text)

    def _handle_address_confirmation_rejected(self, state: SessionState, original_text: str) -> str:
        state.awaiting_address_confirm = False
        state.pending_address_candidate = None
        state.pending_address_text = ""
        if state.address_retries < self.max_address_retries:
            state.address_retries += 1
            return self._record_bot_reply(state, ADDRESS_RETRY_PROMPT)

        state.results["address"] = {
            "status": "not_found",
            "text": original_text,
            "intent": "address",
        }
        return self._advance_with_reply(state, "好的，地址已记录，后续会由人工进一步核实。")

    def _handle_silence(self, state: SessionState, step: DialogueStep) -> str:
        if state.timeout_retries < self.max_timeout_retries:
            state.timeout_retries += 1
            return self._record_bot_reply(state, TIMEOUT_RETRY_PROMPT)

        state.results[step.key] = {"status": "timeout"}
        state.finished = True
        state.status = "terminated"
        return self._record_bot_reply(state, TIMEOUT_END_MESSAGE)

    def _complete_unclear_step(
        self,
        state: SessionState,
        step: DialogueStep,
        user_text: str,
        intent: str,
    ) -> str:
        state.results[step.key] = {
            "status": "unclear",
            "text": user_text,
            "intent": intent,
        }
        return self._advance_with_reply(state, "好的，这一项我先为您标记为待确认。")

    def _advance_with_reply(self, state: SessionState, prefix: str) -> str:
        state.step_index += 1
        state.unclear_retries = 0
        state.timeout_retries = 0
        state.address_retries = 0
        state.awaiting_address_confirm = False
        state.pending_address_candidate = None
        state.pending_address_text = ""

        if state.step_index >= len(self.steps):
            state.finished = True
            state.status = "completed"
            reply = f"{prefix}{END_MESSAGE}"
        else:
            state.status = "in_progress"
            reply = f"{prefix}{self.steps[state.step_index].question}"
        return self._record_bot_reply(state, reply)

    def _build_summary(self, state: SessionState) -> dict[str, Any]:
        return {
            "status": "completed" if state.status == "completed" else "terminated",
            "results": state.results,
            "transcript": state.transcript,
        }

    def _current_step(self, state: SessionState) -> DialogueStep | None:
        if state.step_index >= len(self.steps):
            return None
        return self.steps[state.step_index]

    def _current_prompt(self, state: SessionState) -> str:
        if state.awaiting_address_confirm and state.pending_address_candidate is not None:
            return self._last_bot_text(state) or self._build_address_confirmation_turn(
                state.pending_address_text,
                state.pending_address_candidate,
            )

        step = self._current_step(state)
        if step is None:
            return END_MESSAGE
        if state.timeout_retries > 0:
            return TIMEOUT_RETRY_PROMPT
        if step.expected_intent == "address" and state.address_retries > 0:
            return ADDRESS_RETRY_PROMPT
        if state.unclear_retries > 0:
            return step.retry_prompt
        return step.question

    @staticmethod
    def _last_bot_text(state: SessionState) -> str:
        for item in reversed(state.transcript):
            if item.get("speaker") == "bot":
                return item.get("text", "")
        return ""

    @staticmethod
    def _record_bot_reply(state: SessionState, text: str) -> str:
        state.transcript.append({"speaker": "bot", "text": text})
        return text

    @staticmethod
    def _record_user_turn(state: SessionState, text: str) -> None:
        state.transcript.append({"speaker": "user", "text": text})

    def _ensure_prompt_recorded(self, state: SessionState, prompt: str) -> None:
        if not state.transcript or state.transcript[-1] != {"speaker": "bot", "text": prompt}:
            state.transcript.append({"speaker": "bot", "text": prompt})

    def _build_address_confirmation_turn(self, original_text: str, candidate: dict[str, Any]) -> str:
        focus_text = self._extract_address_difference(
            original_text,
            self._meaningful_candidate_text(candidate),
        )
        return self.intent_classifier.generate_address_confirmation_prompt(
            original_text=original_text,
            matched_text=self._meaningful_candidate_text(candidate),
            matched_name=candidate.get("name", ""),
            focus_text=focus_text,
            fallback_prompt=self._build_address_confirmation_prompt(original_text, candidate),
        )

    def _listen(self) -> dict[str, Any]:
        if self.input_mode == "text":
            text = self.input_func("用户：").strip()
            return {"timed_out": not bool(text), "text": text}

        if self.asr_client is None:
            raise RuntimeError("未提供 asr_client，无法在 microphone 模式下监听。")
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
            trimmed = text[prefix_end:].strip()
            if trimmed:
                return trimmed
        return text
