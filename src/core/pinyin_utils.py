from __future__ import annotations

import re
from typing import Iterable

try:
    from pypinyin import lazy_pinyin
except ImportError:  # pragma: no cover - exercised only when dependency is absent
    lazy_pinyin = None


def normalize_text(text: str) -> str:
    # 地址匹配前先去掉口语停顿和常见标点，减少无意义差异。
    return re.sub(r"[\s，。、“”‘’,.!！？；;:：\-（）()\[\]【】/\\]+", "", text).lower()


def pronunciation_key(text: str) -> str:
    normalized = normalize_text(text)
    # 没装 pypinyin 时退回原文本，保证功能降级但不阻塞主流程。
    if lazy_pinyin is None:
        return normalized
    return "".join(lazy_pinyin(normalized))


def levenshtein_distance(text_a: str, text_b: str) -> int:
    if text_a == text_b:
        return 0
    if not text_a:
        return len(text_b)
    if not text_b:
        return len(text_a)

    prev_row = list(range(len(text_b) + 1))
    for i, char_a in enumerate(text_a, start=1):
        curr_row = [i]
        for j, char_b in enumerate(text_b, start=1):
            insert_cost = curr_row[j - 1] + 1
            delete_cost = prev_row[j] + 1
            replace_cost = prev_row[j - 1] + (0 if char_a == char_b else 1)
            curr_row.append(min(insert_cost, delete_cost, replace_cost))
        prev_row = curr_row
    return prev_row[-1]


def pinyin_distance(text_a: str, text_b: str) -> int:
    # 让“龙影/龙吟”“轮头/仑头”这类同音错字能有更接近的距离。
    return levenshtein_distance(pronunciation_key(text_a), pronunciation_key(text_b))


def best_text_match(query: str, candidates: Iterable[str]) -> list[tuple[str, int]]:
    scored = [(candidate, pinyin_distance(query, candidate)) for candidate in candidates if candidate]
    return sorted(scored, key=lambda item: (item[1], len(item[0])))
