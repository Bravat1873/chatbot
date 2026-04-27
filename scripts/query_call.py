# 命令行工具：按 CallId 查询外呼通话详情

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

from src.caller.aiccs import get_call_dialog_content


def main() -> int:
    """CLI 入口：接收 CallId 和日期，查询通话对话内容。"""
    parser = argparse.ArgumentParser(description="按 CallId 查询外呼通话详情")
    parser.add_argument("call_id", help="发起外呼时返回的 CallId")
    parser.add_argument(
        "--date",
        dest="call_date",
        default=None,
        help="通话发生的日期，格式 YYYY-MM-DD，默认今天",
    )
    args = parser.parse_args()
    try:
        result = get_call_dialog_content(args.call_id, args.call_date)
        print(json.dumps(result, ensure_ascii=False, indent=2, default=str))
    except Exception as exc:
        print(f"查询失败: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
