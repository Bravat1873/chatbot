import unittest
from contextlib import redirect_stdout
from io import StringIO

from src.config import Settings
from src.core.dialogue import (
    ADDRESS_ONLY_STEPS,
    DEFAULT_FLOW_STEPS,
    ADDRESS_RETRY_PROMPT,
    END_MESSAGE,
    TIMEOUT_END_MESSAGE,
    TIMEOUT_RETRY_PROMPT,
    DialogueEngine,
)
from src.core.geocode import AMapGeocoder
from src.core.intent import IntentClassifier
from src.local.asr import ASRClient


class FakeGeocoder(AMapGeocoder):
    def __init__(self, settings: Settings) -> None:
        super().__init__(settings)

    def resolve_place(self, query: str, city: str | None = None):  # type: ignore[override]
        if "88号" in query or "100号" in query:
            return {
                "found": True,
                "best": {
                    "name": query,
                    "address": query,
                    "location": "116.1,39.9",
                    "cityname": "北京市",
                    "adname": "朝阳区",
                    "display_text": query,
                    "compare_text": query,
                    "precision_ok": True,
                },
                "candidates": [{
                    "name": query,
                    "address": query,
                    "location": "116.1,39.9",
                    "cityname": "北京市",
                    "adname": "朝阳区",
                    "display_text": query,
                    "compare_text": query,
                    "precision_ok": True,
                }],
            }
        if "轮头村" in query:
            return {
                "found": True,
                "best": {
                    "name": "小家公寓",
                    "address": "仑头村仑头路82号",
                    "location": "113.4,23.0",
                    "cityname": "广州市",
                    "adname": "海珠区",
                    "display_text": "海珠区仑头村仑头路82号小家公寓",
                    "compare_text": "广州市海珠区仑头村仑头路82号小家公寓",
                    "precision_ok": True,
                },
                "candidates": [{
                    "name": "小家公寓",
                    "address": "仑头村仑头路82号",
                    "location": "113.4,23.0",
                    "cityname": "广州市",
                    "adname": "海珠区",
                    "display_text": "海珠区仑头村仑头路82号小家公寓",
                    "compare_text": "广州市海珠区仑头村仑头路82号小家公寓",
                    "precision_ok": True,
                }],
            }
        return {"found": False, "candidates": []}


class FakeSpeaker:
    def __init__(self) -> None:
        self.spoken: list[str] = []

    def speak(self, text: str) -> None:
        self.spoken.append(text)


class FakeStreamingASRClient:
    def __init__(self, text: str = "有的，已经预约了", timed_out: bool = False) -> None:
        self.text = text
        self.timed_out = timed_out
        self.called = False

    def listen_once(self) -> dict[str, object]:
        self.called = True
        return {"timed_out": self.timed_out, "text": self.text}


class DialogueEngineTestCase(unittest.TestCase):
    def setUp(self) -> None:
        self.settings = Settings(
            dashscope_api_key="",
            amap_key="",
            debug=False,
        )

    def make_engine(self, **kwargs) -> DialogueEngine:
        return DialogueEngine(
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            debug=False,
            **kwargs,
        )

    def test_process_turn_happy_path(self) -> None:
        engine = self.make_engine(steps=DEFAULT_FLOW_STEPS)

        first_reply = engine.process_turn("session-1", "")
        second_reply = engine.process_turn("session-1", "有的，已经预约了")
        final_reply = engine.process_turn("session-1", "满意，已经解决了")

        state = engine.session_manager.get_or_create("session-1")

        self.assertEqual(first_reply, DEFAULT_FLOW_STEPS[0].question)
        self.assertIn("好的，已经为您记录。", second_reply)
        self.assertIn(DEFAULT_FLOW_STEPS[1].question, second_reply)
        self.assertIn("好的，记录到您比较满意。", final_reply)
        self.assertIn(END_MESSAGE, final_reply)
        self.assertEqual(state.status, "completed")
        self.assertEqual(state.results["appointment_confirmed"]["intent"], "yes")
        self.assertEqual(state.results["service_satisfied"]["intent"], "yes")

    def test_process_turn_stores_biz_params_on_session(self) -> None:
        engine = self.make_engine(steps=DEFAULT_FLOW_STEPS)

        engine.process_turn("session-biz", "", biz_params={"customer_name": "张三", "order_id": "BL-001"})

        state = engine.session_manager.get_or_create("session-biz")

        self.assertEqual(state.biz_params, {"customer_name": "张三", "order_id": "BL-001"})

    def test_process_turn_handles_silence_then_termination(self) -> None:
        engine = self.make_engine(steps=DEFAULT_FLOW_STEPS)

        engine.process_turn("session-2", "")
        retry_reply = engine.process_turn("session-2", "用户没有说话")
        final_reply = engine.process_turn("session-2", "用户没有说话")

        state = engine.session_manager.get_or_create("session-2")

        self.assertEqual(retry_reply, TIMEOUT_RETRY_PROMPT)
        self.assertEqual(final_reply, TIMEOUT_END_MESSAGE)
        self.assertTrue(state.finished)
        self.assertEqual(state.status, "terminated")
        self.assertEqual(state.results["appointment_confirmed"]["status"], "timeout")

    def test_address_confirmation_is_non_blocking(self) -> None:
        engine = self.make_engine(steps=ADDRESS_ONLY_STEPS)

        first_reply = engine.process_turn("session-3", "")
        confirm_reply = engine.process_turn("session-3", "广州海珠区轮头村八二路小家公寓")
        final_reply = engine.process_turn("session-3", "是的")

        state = engine.session_manager.get_or_create("session-3")

        self.assertEqual(first_reply, ADDRESS_ONLY_STEPS[0].question)
        self.assertIn("小家公寓", confirm_reply)
        self.assertIn("仑头村仑头路82号", confirm_reply)
        self.assertFalse(state.awaiting_address_confirm)
        self.assertIn(END_MESSAGE, final_reply)
        self.assertEqual(state.results["address"]["status"], "ok")
        self.assertEqual(state.results["address"]["place"]["name"], "小家公寓")

    def test_address_confirmation_rejection_retries_address(self) -> None:
        engine = self.make_engine(steps=ADDRESS_ONLY_STEPS)

        engine.process_turn("session-4", "")
        engine.process_turn("session-4", "广州海珠区轮头村八二路小家公寓")
        retry_reply = engine.process_turn("session-4", "不是")

        state = engine.session_manager.get_or_create("session-4")

        self.assertEqual(retry_reply, ADDRESS_RETRY_PROMPT)
        self.assertEqual(state.address_retries, 1)
        self.assertFalse(state.awaiting_address_confirm)

    def test_process_turn_returns_end_message_after_finish(self) -> None:
        engine = self.make_engine(steps=(DEFAULT_FLOW_STEPS[0],))

        engine.process_turn("session-5", "")
        engine.process_turn("session-5", "有的，已经预约了")

        self.assertEqual(engine.process_turn("session-5", "继续"), END_MESSAGE)

    def test_service_satisfied_reply_is_customized(self) -> None:
        self.assertEqual(
            DialogueEngine._build_yes_no_reply("service_satisfied", "yes"),
            "好的，记录到您比较满意。",
        )
        self.assertEqual(
            DialogueEngine._build_yes_no_reply("service_satisfied", "no"),
            "抱歉，给您造成了不愉快的体验。",
        )

    def test_say_prints_and_invokes_speaker(self) -> None:
        speaker = FakeSpeaker()
        engine = self.make_engine(
            asr_client=ASRClient(self.settings),
            input_mode="text",
            input_func=lambda _: "",
            speaker=speaker,
        )

        output = StringIO()
        with redirect_stdout(output):
            engine._say("测试播报")

        self.assertIn("机器人：测试播报", output.getvalue())
        self.assertEqual(speaker.spoken, ["测试播报"])

    def test_run_in_text_mode(self) -> None:
        inputs = iter(["有的，已经预约了", "满意，已经解决了"])
        engine = self.make_engine(
            asr_client=ASRClient(self.settings),
            input_mode="text",
            input_func=lambda _: next(inputs),
            steps=DEFAULT_FLOW_STEPS,
        )

        summary = engine.run()

        self.assertEqual(summary["status"], "completed")
        self.assertEqual(summary["results"]["appointment_confirmed"]["intent"], "yes")
        self.assertEqual(summary["results"]["service_satisfied"]["intent"], "yes")

    def test_microphone_mode_uses_streaming_asr(self) -> None:
        asr_client = FakeStreamingASRClient()
        engine = self.make_engine(
            asr_client=asr_client,  # type: ignore[arg-type]
            input_mode="microphone",
            steps=(DEFAULT_FLOW_STEPS[0],),
        )

        summary = engine.run()

        self.assertTrue(asr_client.called)
        self.assertEqual(summary["status"], "completed")
        self.assertEqual(summary["results"]["appointment_confirmed"]["intent"], "yes")

    def test_address_confirmation_ignores_admin_region_prefix(self) -> None:
        prompt = DialogueEngine._build_address_confirmation_prompt(
            "广州市海珠区官洲街道龙影大到8节9好",
            {
                "display_text": "海珠区龙吟大街8巷10号",
                "formatted": "广东省广州市海珠区龙吟大街8",
                "compare_text": "广东省广州市海珠区龙吟大街8",
            },
        )

        self.assertIn("龙吟大街", prompt)
        self.assertNotIn("广东省", prompt)

    def test_address_confirmation_prefers_named_place(self) -> None:
        prompt = DialogueEngine._build_address_confirmation_prompt(
            "广州市海珠区贝朗公司",
            {
                "name": "贝朗(中国)卫浴有限公司",
                "display_text": "海珠区琶洲大道东1号保利国际广场南塔21层贝朗(中国)卫浴有限公司",
                "formatted": "广东省广州市海珠区琶洲大道东1号保利国际广场贝朗(中国)卫浴有限公司",
                "compare_text": "广东省广州市海珠区琶洲大道东1号保利国际广场贝朗(中国)卫浴有限公司",
            },
        )

        self.assertIn("贝朗(中国)卫浴有限公司", prompt)
        self.assertIn("琶洲大道东1号保利国际广场", prompt)


if __name__ == "__main__":
    unittest.main()
