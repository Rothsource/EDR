import re
from typing import Optional

from . import register, DecodedFields, PARSER_VERSION

SSH_FAILED_PASSWORD = re.compile(
    r"^Failed password for (invalid user )?(\S+) from (\S+) port (\d+) ssh2$"
)
SSH_ACCEPTED = re.compile(
    r"^Accepted (password|publickey) for (\S+) from (\S+) port (\d+) ssh2$"
)
SUDO_AUTH_FAILURE = re.compile(
    r"^pam_unix\(sudo:auth\): authentication failure; "
    r"logname=(\S*) uid=(\d+) euid=(\d+) tty=(\S+) ruser=(\S*) rhost=(\S*)\s+user=(\S+)$"
)
SSH_PAM_SUMMARY = re.compile(
    r"^PAM (\d+) more authentication failures?; "
    r"logname=(\S*) uid=(\d+) euid=(\d+) tty=(\S+) ruser=(\S*) rhost=(\S*)(?:\s+user=(\S+))?$"
)

CLASS_UID = 3002
CATEGORY_UID = 3
ACTIVITY_ID = 1
SEVERITY_ID = 2
TYPE_UID = CLASS_UID * 100 + ACTIVITY_ID

# D4 visibility: counts of raw records that matched no pattern, by
# identifier. Not persisted — process-lifetime only. Whatever exposes
# server metrics (logging on an interval, a /metrics endpoint, etc.)
# should read this; decode() itself just increments it.
UNMATCHED_COUNTS: dict[str, int] = {}


def _clamp(s: Optional[str], n: int = 64) -> Optional[str]:
    return s[:n] if s else s


def _record_unmatched(identifier: str) -> None:
    UNMATCHED_COUNTS[identifier] = UNMATCHED_COUNTS.get(identifier, 0) + 1


@register("journald")
def decode(raw: dict) -> Optional[DecodedFields]:
    # Matches the agent's actual raw shape: {"identifier": ..., "message": ..., "pid": ...}
    # (see internal/modules/auth/reader_linux.go's event.Params.Data["raw"]).
    msg = raw.get("message", "")
    identifier = raw.get("identifier", "")

    if identifier in ("sshd-session", "sshd"):
        if m := SSH_FAILED_PASSWORD.match(msg):
            reason = "unknown_user" if m.group(1) else "bad_password"
            return DecodedFields(
                class_uid=CLASS_UID,
                category_uid=CATEGORY_UID,
                activity_id=ACTIVITY_ID,
                type_uid=TYPE_UID,
                severity_id=SEVERITY_ID,
                username=_clamp(m.group(2)),
                data={
                    "src_ip": m.group(3),
                    "src_port": m.group(4),
                    "status": "failure",
                    "reason": reason,
                    "service": "sshd",
                },
            )

        if m := SSH_ACCEPTED.match(msg):
            return DecodedFields(
                class_uid=CLASS_UID,
                category_uid=CATEGORY_UID,
                activity_id=ACTIVITY_ID,
                type_uid=TYPE_UID,
                severity_id=SEVERITY_ID,
                username=_clamp(m.group(2)),
                data={
                    "src_ip": m.group(3),
                    "src_port": m.group(4),
                    "status": "success",
                    "reason": m.group(1),  # "password" or "publickey"
                    "service": "sshd",
                },
            )

        if m := SSH_PAM_SUMMARY.match(msg):
            username = _clamp(m.group(8)) if m.group(8) else None
            return DecodedFields(
                class_uid=CLASS_UID,
                category_uid=CATEGORY_UID,
                activity_id=ACTIVITY_ID,
                type_uid=TYPE_UID,
                severity_id=SEVERITY_ID,
                username=username,
                data={
                    "src_ip": m.group(7),
                    "status": "failure",
                    "reason": "auth_failures_summary",
                    "service": "sshd",
                    "count": int(m.group(1)),
                },
            )

    elif identifier == "sudo":
        if m := SUDO_AUTH_FAILURE.match(msg):
            return DecodedFields(
                class_uid=CLASS_UID,
                category_uid=CATEGORY_UID,
                activity_id=ACTIVITY_ID,
                type_uid=TYPE_UID,
                severity_id=SEVERITY_ID,
                username=_clamp(m.group(7)),
                data={
                    "src_ip": _clamp(m.group(6)) or None,
                    "status": "failure",
                    "reason": "sudo_bad_password",
                    "service": "sudo",
                },
            )

    _record_unmatched(identifier)
    return None