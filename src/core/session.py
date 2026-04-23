from dataclasses import dataclass, field
from typing import Any


@dataclass
class SessionState:
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
    def __init__(self) -> None:
        self._sessions: dict[str, SessionState] = {}

    def get_or_create(self, session_id: str) -> SessionState:
        if session_id not in self._sessions:
            self._sessions[session_id] = SessionState()
        return self._sessions[session_id]

    def remove(self, session_id: str) -> None:
        self._sessions.pop(session_id, None)
