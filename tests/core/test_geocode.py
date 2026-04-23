import unittest

from src.core.geocode import AMapGeocoder
from src.core.pinyin_utils import pinyin_distance


class GeocodeTestCase(unittest.TestCase):
    def test_precise_level_keywords_are_accepted(self) -> None:
        self.assertTrue(AMapGeocoder.is_precise_enough("门牌号", "北京市朝阳区建国路88号"))
        self.assertTrue(AMapGeocoder.is_precise_enough("兴趣点", "上海市世纪大道100号"))

    def test_broad_level_keywords_are_rejected(self) -> None:
        self.assertFalse(AMapGeocoder.is_precise_enough("市", "北京市"))
        self.assertFalse(AMapGeocoder.is_precise_enough("区县", "北京市朝阳区"))

    def test_build_display_text_deduplicates_adjacent_parts(self) -> None:
        self.assertEqual(
            AMapGeocoder._build_display_text(
                name="小家公寓",
                address="仑头村仑头路82号",
                district="海珠区",
            ),
            "海珠区仑头村仑头路82号小家公寓",
        )

    def test_pinyin_distance_treats_similar_sounds_as_close(self) -> None:
        self.assertLess(
            pinyin_distance("轮头村八二路", "仑头村仑头路"),
            pinyin_distance("轮头村八二路", "天河体育西"),
        )


if __name__ == "__main__":
    unittest.main()
