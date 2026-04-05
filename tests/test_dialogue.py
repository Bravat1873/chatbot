import unittest
from contextlib import redirect_stdout
from io import StringIO

from asr import ASRClient
from config import Settings
from dialogue import ADDRESS_ONLY_STEPS, DEFAULT_FLOW_STEPS, DialogueEngine
from geocode import AMapGeocoder
from intent import IntentClassifier


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

    def test_happy_path_in_text_mode(self) -> None:
        inputs = iter(
            [
                "有的，已经预约了",
                "满意，已经解决了",
            ]
        )
        engine = DialogueEngine(
            asr_client=ASRClient(self.settings),
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            input_mode="text",
            debug=False,
            input_func=lambda _: next(inputs),
            steps=DEFAULT_FLOW_STEPS,
        )

        summary = engine.run()

        self.assertEqual(summary["status"], "completed")
        self.assertEqual(summary["results"]["appointment_confirmed"]["intent"], "yes")
        self.assertEqual(summary["results"]["service_satisfied"]["intent"], "yes")
        self.assertNotIn("address", summary["results"])

    def test_timeout_and_address_retry(self) -> None:
        inputs = iter(
            [
                "",
                "还没呢",
                "满意",
            ]
        )
        engine = DialogueEngine(
            asr_client=ASRClient(self.settings),
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            input_mode="text",
            debug=False,
            input_func=lambda _: next(inputs),
            steps=DEFAULT_FLOW_STEPS,
        )

        summary = engine.run()

        self.assertEqual(summary["status"], "completed")
        self.assertEqual(summary["results"]["appointment_confirmed"]["intent"], "no")
        self.assertEqual(summary["results"]["service_satisfied"]["intent"], "yes")
        self.assertNotIn("address", summary["results"])

    def test_service_satisfied_reply_is_customized(self) -> None:
        self.assertEqual(
            DialogueEngine._build_yes_no_reply("service_satisfied", "yes"),
            "好的，记录到您比较满意。",
        )
        self.assertEqual(
            DialogueEngine._build_yes_no_reply("service_satisfied", "no"),
            "抱歉，给您造成了不愉快的体验。",
        )

    def test_happy_path_prints_custom_satisfaction_reply(self) -> None:
        inputs = iter(
            [
                "有的，已经预约了",
                "满意，已经解决了",
            ]
        )
        engine = DialogueEngine(
            asr_client=ASRClient(self.settings),
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            input_mode="text",
            debug=False,
            input_func=lambda _: next(inputs),
            steps=DEFAULT_FLOW_STEPS,
        )

        output = StringIO()
        with redirect_stdout(output):
            engine.run()

        self.assertIn("好的，记录到您比较满意。", output.getvalue())

    def test_address_only_flow_runs_only_address_question(self) -> None:
        inputs = iter(
            [
                "北京市朝阳区建国路88号SOHO现代城",
                "是的",
            ]
        )
        engine = DialogueEngine(
            asr_client=ASRClient(self.settings),
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            input_mode="text",
            debug=False,
            input_func=lambda _: next(inputs),
            steps=ADDRESS_ONLY_STEPS,
        )

        summary = engine.run()

        self.assertEqual(summary["status"], "completed")
        self.assertEqual(set(summary["results"]), {"address"})
        self.assertEqual(summary["results"]["address"]["status"], "ok")

    def test_say_prints_and_invokes_speaker(self) -> None:
        speaker = FakeSpeaker()
        engine = DialogueEngine(
            asr_client=ASRClient(self.settings),
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            input_mode="text",
            debug=False,
            input_func=lambda _: "",
            speaker=speaker,
            steps=DEFAULT_FLOW_STEPS,
        )

        output = StringIO()
        with redirect_stdout(output):
            engine._say("测试播报")

        self.assertIn("机器人：测试播报", output.getvalue())
        self.assertEqual(speaker.spoken, ["测试播报"])

    def test_address_flow_confirms_corrected_candidate(self) -> None:
        inputs = iter(
            [
                "广州海珠区轮头村八二路小家公寓",
                "是的",
            ]
        )
        engine = DialogueEngine(
            asr_client=ASRClient(self.settings),
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            input_mode="text",
            debug=False,
            input_func=lambda _: next(inputs),
            steps=ADDRESS_ONLY_STEPS,
        )

        output = StringIO()
        with redirect_stdout(output):
            summary = engine.run()

        self.assertEqual(summary["status"], "completed")
        self.assertEqual(summary["results"]["address"]["status"], "ok")
        self.assertIn("小家公寓", output.getvalue())
        self.assertIn("仑头村仑头路82号", output.getvalue())

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

    def test_microphone_mode_uses_streaming_asr(self) -> None:
        asr_client = FakeStreamingASRClient()
        engine = DialogueEngine(
            asr_client=asr_client,  # type: ignore[arg-type]
            intent_classifier=IntentClassifier(self.settings, use_llm=False),
            geocoder=FakeGeocoder(self.settings),
            input_mode="microphone",
            debug=False,
            steps=(DEFAULT_FLOW_STEPS[0],),
        )

        summary = engine.run()

        self.assertTrue(asr_client.called)
        self.assertEqual(summary["status"], "completed")
        self.assertEqual(summary["results"]["appointment_confirmed"]["intent"], "yes")


if __name__ == "__main__":
    unittest.main()
