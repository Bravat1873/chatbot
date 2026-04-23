import unittest

from src.core.session import SessionManager, SessionState


class SessionManagerTestCase(unittest.TestCase):
    def test_get_or_create_returns_same_state_for_same_session(self) -> None:
        manager = SessionManager()

        first = manager.get_or_create("session-1")
        second = manager.get_or_create("session-1")

        self.assertIs(first, second)
        self.assertIsInstance(first, SessionState)

    def test_remove_deletes_session(self) -> None:
        manager = SessionManager()
        first = manager.get_or_create("session-2")

        manager.remove("session-2")
        second = manager.get_or_create("session-2")

        self.assertIsNot(first, second)


if __name__ == "__main__":
    unittest.main()
