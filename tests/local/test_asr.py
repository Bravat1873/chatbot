import unittest
from unittest.mock import patch

from src.config import Settings
from src.local.asr import ASRClient


class ASRClientTestCase(unittest.TestCase):
    def test_validate_audio_settings_rejects_non_mono(self) -> None:
        client = ASRClient(
            Settings(
                dashscope_api_key="",
                amap_key="",
                channels=2,
                debug=False,
            )
        )

        with self.assertRaisesRegex(RuntimeError, "单声道"):
            client._validate_audio_settings()

    def test_validate_audio_settings_rejects_unsupported_sample_rate(self) -> None:
        client = ASRClient(
            Settings(
                dashscope_api_key="",
                amap_key="",
                sample_rate=22_050,
                debug=False,
            )
        )

        with self.assertRaisesRegex(RuntimeError, "8kHz 或 16kHz"):
            client._validate_audio_settings()

    def test_ensure_session_completed_requires_session_finished(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "session.finished"):
            ASRClient._ensure_session_completed(
                True,
                {"error": None, "session_finished": False},
            )

    def test_ensure_session_completed_raises_asr_error(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "语音识别失败: boom"):
            ASRClient._ensure_session_completed(
                True,
                {"error": "boom", "session_finished": True},
            )

    @patch("src.local.asr.sd")
    @patch("src.local.asr.np")
    def test_listen_once_times_out_without_speech(self, np_mock, sd_mock) -> None:
        class _FakeChunk:
            size = 1

            def reshape(self, _):
                return self

            def astype(self, _):
                return self

            def copy(self):
                return self

            def tobytes(self):
                return b"\x00\x00"

        class _FakeStream:
            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

            def read(self, _):
                return _FakeChunk(), False

        settings = Settings(
            dashscope_api_key="test-key",
            amap_key="",
            vad_no_response_seconds=0.0,
            debug=False,
        )
        client = ASRClient(settings)

        np_mock.sqrt.return_value = 0.0
        np_mock.mean.return_value = 0.0
        np_mock.square.return_value = 0.0
        sd_mock.InputStream.return_value = _FakeStream()

        result = client.listen_once()

        self.assertEqual(result, {"timed_out": True, "text": ""})


if __name__ == "__main__":
    unittest.main()
