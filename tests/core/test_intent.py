import unittest
from unittest.mock import patch

from dashscope.api_entities.dashscope_response import (
    DashScopeAPIResponse,
    GenerationResponse,
    MultiModalConversationResponse,
)

from src.config import Settings
from src.core.intent import IntentClassifier


class IntentTestCase(unittest.TestCase):
    def setUp(self) -> None:
        self.classifier = IntentClassifier(
            Settings(
                dashscope_api_key="",
                amap_key="",
                debug=False,
            ),
            use_llm=False,
        )

    def test_yes_and_no_detection(self) -> None:
        yes_result = self.classifier.classify("嗯对，已经约过了", {"expected_intent": "yes_no"})
        no_result = self.classifier.classify("还没呢，没有预约", {"expected_intent": "yes_no"})

        self.assertEqual(yes_result["intent"], "yes")
        self.assertEqual(no_result["intent"], "no")

    def test_address_detection(self) -> None:
        result = self.classifier.classify(
            "北京市朝阳区建国路88号SOHO现代城",
            {"expected_intent": "address"},
        )
        self.assertEqual(result["intent"], "address")
        self.assertIn("88号", result["address"])

    def test_generate_address_confirmation_prompt_falls_back_without_key(self) -> None:
        prompt = self.classifier.generate_address_confirmation_prompt(
            original_text="广州市海珠区官洲街道龙影大到8节9好",
            matched_text="广东省广州市海珠区龙吟大街8",
            focus_text="龙吟大街8",
        )
        self.assertIn("龙吟大街8", prompt)

    def test_generate_address_confirmation_prompt_prefers_named_place_in_fallback(self) -> None:
        prompt = self.classifier.generate_address_confirmation_prompt(
            original_text="广州市海珠区贝朗公司",
            matched_text="琶洲大道东1号保利国际广场南塔21层贝朗(中国)卫浴有限公司",
            matched_name="贝朗(中国)卫浴有限公司",
            focus_text="琶洲大道东1号保利国际广场南塔21层",
        )
        self.assertIn("贝朗(中国)卫浴有限公司", prompt)
        self.assertIn("琶洲大道东1号保利国际广场南塔21层", prompt)

    @patch("dashscope.Generation.call")
    def test_qwen_generation_models_use_generation_api(self, generation_call) -> None:
        generation_call.return_value = GenerationResponse.from_api_response(
            DashScopeAPIResponse(
                status_code=200,
                output={
                    "choices": [
                        {
                            "message": {
                                "role": "assistant",
                                "content": "{\"intent\":\"yes\",\"address\":\"\"}"
                            }
                        }
                    ]
                },
                usage={"input_tokens": 10, "output_tokens": 5},
            )
        )
        classifier = IntentClassifier(
            Settings(
                dashscope_api_key="test-key",
                amap_key="",
                llm_model="qwen-max",
                debug=False,
            ),
            use_llm=True,
        )

        result = classifier.classify("已经约了", {"expected_intent": "yes_no"})

        self.assertEqual(result["intent"], "yes")
        generation_call.assert_called_once()
        self.assertFalse(generation_call.call_args.kwargs["enable_thinking"])
        self.assertEqual(generation_call.call_args.kwargs["max_tokens"], 20)
        self.assertIn("最短 JSON", generation_call.call_args.kwargs["messages"][0]["content"])

    @patch("dashscope.MultiModalConversation.call")
    def test_qwen35_models_use_multimodal_api(self, multimodal_call) -> None:
        multimodal_call.return_value = MultiModalConversationResponse.from_api_response(
            DashScopeAPIResponse(
                status_code=200,
                output={
                    "choices": [
                        {
                            "message": {
                                "role": "assistant",
                                "content": [
                                    {"text": "{\"intent\":\"no\",\"address\":\"\"}"}
                                ]
                            }
                        }
                    ]
                },
                usage={"input_tokens": 11, "output_tokens": 4},
            )
        )
        classifier = IntentClassifier(
            Settings(
                dashscope_api_key="test-key",
                amap_key="",
                llm_model="qwen3.5-flash",
                debug=False,
            ),
            use_llm=True,
        )

        result = classifier.classify("还没有呢", {"expected_intent": "yes_no"})

        self.assertEqual(result["intent"], "no")
        multimodal_call.assert_called_once()
        self.assertFalse(multimodal_call.call_args.kwargs["enable_thinking"])
        self.assertEqual(multimodal_call.call_args.kwargs["max_tokens"], 20)
        self.assertIn("最短 JSON", multimodal_call.call_args.kwargs["messages"][0]["content"][0]["text"])

    @patch("dashscope.MultiModalConversation.call")
    def test_usage_tracker_reads_output_tokens(self, multimodal_call) -> None:
        tracker_records: list[dict[str, int | str]] = []

        class _Tracker:
            def add(self, service: str, **kwargs) -> None:
                tracker_records.append({"service": service, **kwargs})

        multimodal_call.return_value = MultiModalConversationResponse.from_api_response(
            DashScopeAPIResponse(
                status_code=200,
                output={
                    "choices": [
                        {
                            "message": {
                                "role": "assistant",
                                "content": [
                                    {"text": "{\"intent\":\"yes\",\"address\":\"\"}"}
                                ]
                            }
                        }
                    ]
                },
                usage={"input_tokens": 12, "output_tokens": 3},
            )
        )
        classifier = IntentClassifier(
            Settings(
                dashscope_api_key="test-key",
                amap_key="",
                llm_model="qwen3.5-flash",
                debug=False,
            ),
            use_llm=True,
            tracker=_Tracker(),
        )

        classifier.classify("已经预约了", {"expected_intent": "yes_no"})

        self.assertEqual(
            tracker_records,
            [
                {
                    "service": "llm",
                    "model": "qwen3.5-flash",
                    "input_tokens": 12,
                    "output_tokens": 3,
                }
            ],
        )

    @patch("dashscope.MultiModalConversation.call")
    def test_generate_address_confirmation_prompt_uses_llm(self, multimodal_call) -> None:
        multimodal_call.return_value = MultiModalConversationResponse.from_api_response(
            DashScopeAPIResponse(
                status_code=200,
                output={
                    "choices": [
                        {
                            "message": {
                                "role": "assistant",
                                "content": [{"text": "您说的是龙吟大街8号对吗？"}],
                            }
                        }
                    ]
                },
                usage={"input_tokens": 21, "output_tokens": 8},
            )
        )
        classifier = IntentClassifier(
            Settings(
                dashscope_api_key="test-key",
                amap_key="",
                llm_model="qwen3.5-flash",
                debug=False,
            ),
            use_llm=True,
        )

        prompt = classifier.generate_address_confirmation_prompt(
            original_text="广州市海珠区官洲街道龙影大到8节9好",
            matched_text="广东省广州市海珠区龙吟大街8",
            focus_text="龙吟大街8",
            fallback_prompt="回退话术",
        )

        self.assertEqual(prompt, "您说的是龙吟大街8号对吗？")
        multimodal_call.assert_called_once()
        self.assertIn("龙吟大街8", multimodal_call.call_args.kwargs["messages"][1]["content"][0]["text"])

    @patch("dashscope.MultiModalConversation.call")
    def test_generate_address_confirmation_prompt_adds_named_place_when_llm_omits_it(self, multimodal_call) -> None:
        multimodal_call.return_value = MultiModalConversationResponse.from_api_response(
            DashScopeAPIResponse(
                status_code=200,
                output={
                    "choices": [
                        {
                            "message": {
                                "role": "assistant",
                                "content": [{"text": "您是指琶洲大道东1号保利国际广场南塔吗？"}],
                            }
                        }
                    ]
                },
                usage={"input_tokens": 21, "output_tokens": 8},
            )
        )
        classifier = IntentClassifier(
            Settings(
                dashscope_api_key="test-key",
                amap_key="",
                llm_model="qwen3.5-flash",
                debug=False,
            ),
            use_llm=True,
        )

        prompt = classifier.generate_address_confirmation_prompt(
            original_text="广州市海珠区贝朗公司",
            matched_text="琶洲大道东1号保利国际广场南塔21层贝朗(中国)卫浴有限公司",
            matched_name="贝朗(中国)卫浴有限公司",
            focus_text="琶洲大道东1号保利国际广场南塔21层",
            fallback_prompt="回退话术",
        )

        self.assertIn("贝朗(中国)卫浴有限公司", prompt)
        self.assertIn("琶洲大道东1号保利国际广场南塔21层", prompt)


if __name__ == "__main__":
    unittest.main()
