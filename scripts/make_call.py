# 命令行工具：发起外呼

from __future__ import annotations

import argparse
import sys
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parents[1]
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))

from src.caller.aiccs import make_call


def main() -> int:
    """CLI 入口：接收被叫号码，调用 AICCS 发起外呼。"""
    parser = argparse.ArgumentParser(description="发起外呼")
    parser.add_argument("phone", help="被叫号码")
    args = parser.parse_args()
    try:
        call_id = make_call(args.phone)
        print(f"呼叫已发起，CallId: {call_id}")
    except Exception as exc:
        print(f"呼叫失败: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
