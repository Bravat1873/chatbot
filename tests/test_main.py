import unittest

from dialogue import ADDRESS_ONLY_STEPS, DEFAULT_FLOW_STEPS
from main import build_parser, select_steps


class MainTestCase(unittest.TestCase):
    def test_parser_defaults_to_normal_flow(self) -> None:
        args = build_parser().parse_args([])
        self.assertFalse(args.address)
        self.assertFalse(args.tts)
        self.assertEqual(select_steps(args.address), DEFAULT_FLOW_STEPS)

    def test_parser_address_flag_switches_flow(self) -> None:
        args = build_parser().parse_args(["--address"])
        self.assertTrue(args.address)
        self.assertEqual(select_steps(args.address), ADDRESS_ONLY_STEPS)

    def test_parser_accepts_tts_options(self) -> None:
        args = build_parser().parse_args(["--tts", "--tts-voice", "longxiaochun"])
        self.assertTrue(args.tts)
        self.assertEqual(args.tts_voice, "longxiaochun")


if __name__ == "__main__":
    unittest.main()
