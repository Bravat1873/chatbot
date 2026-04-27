# 会话管理：内存持有的对话状态（步骤指针、重试计数、结果暂存）。

from dataclasses import dataclass, field
from typing import Any


@dataclass
class SessionState:
    """单次对话的全部状态，无外部持久化。"""
    step_index: int = 0
    unclear_retries: int = 0
    timeout_retries: int = 0
    address_retries: int = 0
    results: dict[str, Any] = field(default_factory=dict)
    biz_params: dict[str, Any] = field(default_factory=dict)
    transcript: list[dict[str, str]] = field(default_factory=list)
    finished: bool = False
    status: str = "in_progress"
    awaiting_address_confirm: bool = False
    pending_address_candidate: dict[str, Any] | None = None
    pending_address_text: str = ""


class SessionManager:
    """简单的会话管理器：支持按 session_id 存取和删除状态。"""
    def __init__(self) -> None:
        self._sessions: dict[str, SessionState] = {}

    def get_or_create(self, session_id: str) -> SessionState:
        if session_id not in self._sessions:
            self._sessions[session_id] = SessionState()
        return self._sessions[session_id]

    def remove(self, session_id: str) -> None:
        self._sessions.pop(session_id, None)
