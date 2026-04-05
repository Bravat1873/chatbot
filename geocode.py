from __future__ import annotations

import difflib
import re
from dataclasses import dataclass
from typing import Any

import requests

from config import Settings
from pinyin_utils import normalize_text, pinyin_distance


PRECISE_LEVEL_KEYWORDS = {
    "门牌号",
    "兴趣点",
    "楼栋",
    "单元",
    "房间号",
    "道路交叉口",
    "道路",
    "村庄",
}
BROAD_LEVEL_KEYWORDS = {"国家", "省", "市", "区县", "开发区", "乡镇"}


@dataclass
class GeocodeResult:
    success: bool
    formatted: str = ""
    level: str = ""
    location: str = ""
    precision_ok: bool = False
    error: str = ""
    raw: dict[str, Any] | None = None

    def to_dict(self) -> dict[str, Any]:
        return {
            "success": self.success,
            "formatted": self.formatted,
            "level": self.level,
            "location": self.location,
            "precision_ok": self.precision_ok,
            "error": self.error,
            "raw": self.raw or {},
        }


class AMapGeocoder:
    api_url = "https://restapi.amap.com/v3/geocode/geo"
    inputtips_url = "https://restapi.amap.com/v3/assistant/inputtips"
    place_search_url = "https://restapi.amap.com/v3/place/text"

    def __init__(self, settings: Settings, timeout: float = 10.0) -> None:
        self.settings = settings
        self.timeout = timeout

    def verify_address(self, address_text: str, city: str | None = None) -> dict[str, Any]:
        cleaned_text = address_text.strip()
        if not cleaned_text:
            return GeocodeResult(
                success=False,
                error="地址为空，无法复核。",
            ).to_dict()

        if not self.settings.amap_key:
            return GeocodeResult(
                success=False,
                error="未配置 AMAP_KEY，无法调用高德地址复核。",
            ).to_dict()

        if not requests:
            return GeocodeResult(
                success=False,
                error="未安装 requests，无法调用高德地址复核。",
            ).to_dict()

        try:
            response = requests.get(
                self.api_url,
                params={
                    "key": self.settings.amap_key,
                    "address": cleaned_text,
                    "city": city or self.settings.default_city,
                    "output": "json",
                },
                timeout=self.timeout,
            )
            response.raise_for_status()
            payload = response.json()
        except requests.RequestException as exc:
            return GeocodeResult(
                success=False,
                error=f"高德地址复核请求失败: {exc}",
            ).to_dict()
        except ValueError as exc:
            return GeocodeResult(
                success=False,
                error=f"高德地址复核返回无法解析: {exc}",
            ).to_dict()
        if payload.get("status") != "1":
            return GeocodeResult(
                success=False,
                error=payload.get("info", "高德地址复核失败。"),
                raw=payload,
            ).to_dict()

        geocodes = payload.get("geocodes") or []
        if not geocodes:
            return GeocodeResult(
                success=False,
                error="未匹配到有效地址，请补充更详细的门牌信息。",
                raw=payload,
            ).to_dict()

        first_result = geocodes[0]
        formatted = first_result.get("formatted_address", "")
        level = first_result.get("level", "")
        location = first_result.get("location", "")
        # level 是高德返回的精度等级，结合标准化地址决定是否继续追问。
        precision_ok = self.is_precise_enough(level, formatted)

        return GeocodeResult(
            success=True,
            formatted=formatted,
            level=level,
            location=location,
            precision_ok=precision_ok,
            raw=payload,
        ).to_dict()

    def get_input_tips(self, keywords: str, city: str | None = None) -> dict[str, Any]:
        # InputTips 偏“联想/纠错”，适合处理 ASR 把“龙吟”听成“龙影”这类情况。
        city = city or self.settings.default_city
        cleaned = keywords.strip()
        if not cleaned:
            return {"found": False, "tips": [], "error": "联想关键词为空"}

        if not self.settings.amap_key:
            return {"found": False, "tips": [], "error": "未配置 AMAP_KEY"}

        if not requests:
            return {"found": False, "tips": [], "error": "未安装 requests"}

        try:
            resp = requests.get(
                self.inputtips_url,
                params={
                    "key": self.settings.amap_key,
                    "keywords": cleaned,
                    "city": city,
                    "citylimit": "true",
                    "datatype": "all",
                    "output": "json",
                },
                timeout=self.timeout,
            )
            resp.raise_for_status()
            payload = resp.json()
        except requests.RequestException as exc:
            return {"found": False, "tips": [], "error": f"高德 InputTips 请求失败: {exc}"}
        except ValueError as exc:
            return {"found": False, "tips": [], "error": f"高德 InputTips 返回无法解析: {exc}"}

        if payload.get("status") != "1":
            return {"found": False, "tips": [], "error": payload.get("info", "InputTips 请求失败"), "raw": payload}

        tips = payload.get("tips") or []
        results = []
        for tip in tips[:5]:
            name = tip.get("name", "")
            address = tip.get("address", "")
            district = tip.get("district", "")
            location = tip.get("location", "")
            display_text = self._build_display_text(name=name, address=address, district=district)
            if not display_text:
                continue
            results.append({
                "name": name,
                "address": address,
                "district": district,
                "location": location,
                "display_text": display_text,
                "source": "input_tips",
            })

        return {"found": bool(results), "tips": results, "raw": payload}

    def search_place(self, query: str, city: str | None = None) -> dict[str, Any]:
        # POI 搜索偏“找明确地点/公司/门店”，和 InputTips 的能力互补。
        city = city or self.settings.default_city
        cleaned = query.strip()
        if not cleaned:
            return {"found": False, "pois": [], "error": "搜索关键词为空"}

        if not self.settings.amap_key:
            return {"found": False, "pois": [], "error": "未配置 AMAP_KEY"}

        if requests is None:
            return {"found": False, "pois": [], "error": "未安装 requests"}

        try:
            resp = requests.get(
                self.place_search_url,
                params={
                    "key": self.settings.amap_key,
                    "keywords": cleaned,
                    "city": city,
                    "citylimit": "true",
                    "output": "json",
                },
                timeout=self.timeout,
            )
            resp.raise_for_status()
            payload = resp.json()
        except requests.RequestException as exc:
            return {"found": False, "pois": [], "error": f"高德 POI 搜索失败: {exc}"}
        except ValueError as exc:
            return {"found": False, "pois": [], "error": f"高德 POI 返回无法解析: {exc}"}

        if payload.get("status") != "1":
            return {"found": False, "pois": [], "error": payload.get("info", "搜索失败"), "raw": payload}

        pois = payload.get("pois") or []
        if not pois:
            return {"found": False, "pois": [], "raw": payload}

        results = []
        for poi in pois[:3]:
            display_text = self._build_display_text(
                name=poi.get("name", ""),
                address=poi.get("address", ""),
                district=poi.get("adname", "") or poi.get("cityname", ""),
            )
            results.append({
                "name": poi.get("name", ""),
                "address": poi.get("address", ""),
                "location": poi.get("location", ""),
                "cityname": poi.get("cityname", ""),
                "adname": poi.get("adname", ""),
                "display_text": display_text,
                "source": "poi_search",
            })

        return {"found": True, "pois": results, "raw": payload}

    def resolve_place(self, query: str, city: str | None = None) -> dict[str, Any]:
        # 这里的目标不是“只要高德给了结果就算成功”，
        # 而是把多路候选统一拉平后，再做一轮更偏业务视角的筛选。
        city = city or self.settings.default_city
        cleaned = query.strip()
        if not cleaned:
            return {"found": False, "candidates": [], "error": "地址关键词为空"}

        tips_result = self.get_input_tips(cleaned, city)
        pois_result = self.search_place(cleaned, city)
        shared_error = self._pick_shared_error(tips_result, pois_result)
        if shared_error:
            return {"found": False, "candidates": [], "error": shared_error}

        candidates = self._merge_candidates(
            query=cleaned,
            tips=tips_result.get("tips", []),
            pois=pois_result.get("pois", []),
            city=city,
        )
        if not candidates:
            return {
                "found": False,
                "candidates": [],
                "error": tips_result.get("error") or pois_result.get("error", ""),
                "tips": tips_result.get("tips", []),
                "pois": pois_result.get("pois", []),
            }

        return {
            "found": True,
            "best": candidates[0],
            "candidates": candidates,
            "tips": tips_result.get("tips", []),
            "pois": pois_result.get("pois", []),
        }

    def _merge_candidates(
        self,
        *,
        query: str,
        tips: list[dict[str, Any]],
        pois: list[dict[str, Any]],
        city: str,
    ) -> list[dict[str, Any]]:
        deduped: dict[str, dict[str, Any]] = {}
        for candidate in [*tips, *pois]:
            display_text = candidate.get("display_text", "").strip()
            if not display_text:
                continue

            key = normalize_text(display_text)
            ranked = dict(candidate)
            local_query = self._meaningful_query_text(query)
            local_display = self._meaningful_query_text(display_text)
            verify_result = self.verify_address(display_text, city=city)
            compare_text = verify_result.get("formatted") or display_text
            local_compare = self._meaningful_query_text(compare_text)

            ranked["distance"] = pinyin_distance(local_query, local_display)
            ranked["similarity"] = self._sequence_similarity(local_query, local_compare or local_display)
            ranked["number_score"] = self._number_match_score(local_query, local_compare or local_display)
            ranked["org_score"] = self._organization_match_score(local_query, local_compare or local_display)
            ranked["road_score"] = self._road_match_score(local_query, local_compare or local_display)
            ranked["verify"] = verify_result
            ranked["precision_ok"] = bool(verify_result.get("precision_ok"))
            ranked["formatted"] = verify_result.get("formatted", "")
            ranked["compare_text"] = verify_result.get("formatted") or display_text
            # 综合分表达“候选像不像用户说的目标地址”。
            ranked["score"] = self._candidate_score(ranked)

            existing = deduped.get(key)
            if existing is None or self._candidate_rank_tuple(ranked) < self._candidate_rank_tuple(existing):
                deduped[key] = ranked

        return sorted(deduped.values(), key=self._candidate_rank_tuple)

    @staticmethod
    def _candidate_rank_tuple(candidate: dict[str, Any]) -> tuple[int, int, int, int]:
        # 分数一致时，再用精度、拼音距离、来源做稳定 tie-break。
        return (
            -int(candidate.get("score", 0)),
            0 if candidate.get("precision_ok") else 1,
            candidate.get("distance", 9999),
            0 if candidate.get("source") == "input_tips" else 1,
            -len(candidate.get("display_text", "")),
        )

    @staticmethod
    def _candidate_score(candidate: dict[str, Any]) -> int:
        # 这是经验分，不是严格概率：
        # 命中数字、组织名、路名都会加分；拼音距离越大越扣分。
        score = 0
        score += 30 if candidate.get("precision_ok") else 0
        score += int(candidate.get("similarity", 0.0) * 100)
        score += int(candidate.get("number_score", 0) * 18)
        score += int(candidate.get("org_score", 0) * 35)
        score += int(candidate.get("road_score", 0) * 25)
        score -= int(candidate.get("distance", 9999) * 0.6)
        score += 4 if candidate.get("source") == "input_tips" else 0
        return score

    @staticmethod
    def _pick_shared_error(*results: dict[str, Any]) -> str:
        # 只有当所有底层链路都属于“根本不可用”时，才直接把错误抛给上层。
        errors = [result.get("error", "") for result in results if result.get("error")]
        if not errors:
            return ""
        if all(error.startswith(("未配置 AMAP_KEY", "未安装 requests")) for error in errors):
            return errors[0]
        return ""

    @staticmethod
    def _build_display_text(*, name: str = "", address: str = "", district: str = "") -> str:
        parts = [part.strip() for part in (district, address, name) if part and part.strip()]
        deduped_parts: list[str] = []
        for part in parts:
            if deduped_parts and part == deduped_parts[-1]:
                continue
            deduped_parts.append(part)
        return "".join(deduped_parts)

    @staticmethod
    def _meaningful_query_text(text: str) -> str:
        # 排序时弱化省/市/区/街道等公共前缀，让比对更关注真正区分地点的部分。
        raw = text.strip()
        if not raw:
            return ""

        prefix_pattern = re.compile(r"^.*?(?:省|市|区|县|镇|乡|街道)")
        match = prefix_pattern.match(raw)
        if match and match.end() < len(raw):
            trimmed = raw[match.end():].strip()
            if trimmed:
                raw = trimmed
        return raw

    @staticmethod
    def _sequence_similarity(text_a: str, text_b: str) -> float:
        normalized_a = normalize_text(text_a)
        normalized_b = normalize_text(text_b)
        if not normalized_a or not normalized_b:
            return 0.0
        return difflib.SequenceMatcher(a=normalized_a, b=normalized_b).ratio()

    @staticmethod
    def _extract_numbers(text: str) -> list[str]:
        return re.findall(r"\d+", text)

    @classmethod
    def _number_match_score(cls, query: str, candidate: str) -> float:
        query_numbers = cls._extract_numbers(query)
        candidate_numbers = cls._extract_numbers(candidate)
        if not query_numbers:
            return 0.0
        if not candidate_numbers:
            return -0.5

        matched = sum(1 for number in query_numbers if number in candidate_numbers)
        return matched / max(len(query_numbers), 1)

    @staticmethod
    def _organization_core(text: str) -> str:
        # “贝朗(中国)卫浴有限公司”这类名称先去壳，再比较核心词，降低后缀噪声。
        cleaned = re.sub(r"[()（）]", "", text)
        cleaned = re.sub(
            r"(有限责任公司|有限公司|公司|集团|店|中心|分店|分公司|营业部)$",
            "",
            cleaned,
        )
        cleaned = re.sub(r"[^\u4e00-\u9fffA-Za-z0-9]", "", cleaned)
        return cleaned

    @classmethod
    def _organization_match_score(cls, query: str, candidate: str) -> float:
        if "公司" not in query and "店" not in query and "中心" not in query:
            return 0.0

        query_core = cls._organization_core(query)
        candidate_core = cls._organization_core(candidate)
        if not query_core or not candidate_core:
            return 0.0
        if query_core in candidate_core:
            return 1.0
        if len(query_core) >= 2 and query_core[:2] in candidate_core:
            return 0.6
        return 0.0

    @staticmethod
    def _extract_road_tokens(text: str) -> list[str]:
        return re.findall(r"[\u4e00-\u9fffA-Za-z0-9]+(?:路|街|巷|道|弄|大道|大街|村)", text)

    @classmethod
    def _road_match_score(cls, query: str, candidate: str) -> float:
        # 路名比对允许轻微 ASR 误差，只要片段相近就算部分命中。
        query_tokens = cls._extract_road_tokens(query)
        candidate_tokens = cls._extract_road_tokens(candidate)
        if not query_tokens:
            return 0.0
        if not candidate_tokens:
            return 0.0

        matched = 0
        for token in query_tokens:
            if any(
                token in candidate_token
                or candidate_token in token
                or cls._sequence_similarity(token, candidate_token) >= 0.55
                for candidate_token in candidate_tokens
            ):
                matched += 1
        return matched / max(len(query_tokens), 1)

    @staticmethod
    def is_precise_enough(level: str, formatted_address: str = "") -> bool:
        if any(keyword in level for keyword in PRECISE_LEVEL_KEYWORDS):
            return True
        if any(keyword in level for keyword in BROAD_LEVEL_KEYWORDS):
            return False

        # 某些返回的 level 不稳定时，退回到地址文本细粒度特征判断。
        has_detail_number = any(token in formatted_address for token in {"号", "栋", "单元", "室"})
        return has_detail_number
