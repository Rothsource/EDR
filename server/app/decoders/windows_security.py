from typing import Optional
from . import register, DecodedFields

PARSER_VERSION = 3

CATEGORY_UID = 3
AUTH_CLASS = 3002    # authentication events
OTHER_CLASS = 3001   # placeholder: account / privilege / group activity. Set to your own class.

# Keys are lowercase; lookups are lowercased too.
SUBSTATUS_REASONS = {
    "0xc0000064": "unknown_user",
    "0xc000006a": "bad_password",
    "0xc0000234": "account_locked",
    "0xc0000072": "account_disabled",
    "0xc0000070": "bad_workstation",
    "0xc0000193": "account_expired",
    "0xc0000071": "password_expired",
}

# event_id -> (activity_id, label)
EVENTS = {
    4624: (1, "logon"),             4625: (1, "logon_failed"),
    4648: (1, "explicit_creds"),    4776: (1, "ntlm_validation"),
    4634: (2, "logoff"),            4647: (2, "user_logoff"),
    4768: (3, "kerberos_tgt"),      4771: (3, "kerberos_preauth_failed"),
    4769: (4, "kerberos_service_ticket"),
    4672: (99, "special_privileges"),
    4778: (99, "rdp_reconnect"),    4779: (99, "rdp_disconnect"),
    4800: (99, "workstation_locked"), 4801: (99, "workstation_unlocked"),
    4720: (99, "account_created"),  4722: (99, "account_enabled"),
    4723: (99, "password_change"),  4724: (99, "password_reset"),
    4725: (99, "account_disabled"), 4726: (99, "account_deleted"),
    4740: (99, "account_locked_out"), 4767: (99, "account_unlocked"),
    4728: (99, "group_member_added"), 4732: (99, "group_member_added"),
    4756: (99, "group_member_added"),
}

# Only these are authentication events (class 3002). Everything else uses OTHER_CLASS.
AUTH_IDS = {4624, 4625, 4634, 4647, 4648, 4768, 4769, 4771, 4776, 4778, 4779}

USER_FIELDS = {
    4672: ("SubjectUserName",),
    4648: ("TargetUserName", "SubjectUserName"),
    4778: ("AccountName",), 4779: ("AccountName",),
    4728: ("MemberName", "MemberSid", "SubjectUserName"),
    4732: ("MemberName", "MemberSid", "SubjectUserName"),
    4756: ("MemberName", "MemberSid", "SubjectUserName"),
}

# The only IDs that mean "a login failed". Detection rules should count these.
FAILURE_IDS = {4625, 4771}
ALERT_IDS = {4740, 4728, 4732, 4756, 4720, 4726}
NOISE_IDS = {4624, 4634, 4672}
NOISE_USERS = {"SYSTEM", "LOCAL SERVICE", "NETWORK SERVICE", "ANONYMOUS LOGON"}


def _clamp(s, n: int = 64):
    return s[:n] if isinstance(s, str) else s


def _event_id(raw: dict) -> Optional[int]:
    try:
        return int(raw.get("EventID"))
    except (TypeError, ValueError):
        return None


def _is_noise(eid: int, raw: dict, username) -> bool:
    if eid not in NOISE_IDS:
        return False
    if raw.get("LogonType") == "5":
        return True
    if isinstance(username, str):
        return username.upper() in NOISE_USERS or username.endswith("$")
    return False


@register("windows-security")
def decode(raw: dict) -> Optional[DecodedFields]:
    eid = _event_id(raw)
    if eid not in EVENTS:
        return None

    activity_id, label = EVENTS[eid]

    username = None
    for f in USER_FIELDS.get(eid, ("TargetUserName", "SubjectUserName")):
        v = raw.get(f)
        if v and v != "-":
            username = _clamp(v)
            break

    if _is_noise(eid, raw, username):
        return None

    # Only real login outcomes get a status. 4648 and 4776 are steps in an
    # attempt, not outcomes, so they never count as failures.
    if eid in FAILURE_IDS:
        status = "failure"
    elif eid == 4624:
        status = "success"
    else:
        status = None

    data = {
        "service": "windows_logon",
        "event_id": eid,
        "event": label,
        "domain": raw.get("TargetDomainName") or raw.get("SubjectDomainName"),
        "src_ip": raw.get("IpAddress") or raw.get("ClientAddress"),
        "src_port": raw.get("IpPort"),
        "logon_type": raw.get("LogonType"),
    }
    if status:
        data["status"] = status

    if eid == 4625:
        sub = (raw.get("SubStatus") or "").lower()
        data["reason"] = SUBSTATUS_REASONS.get(sub, "other")
        if data["reason"] == "other":
            data["substatus_raw"] = sub
    elif eid == 4776:
        data["ntlm_status"] = (raw.get("Status") or "").lower()
    elif eid == 4672:
        data["privileges"] = _clamp(raw.get("PrivilegeList"), 256)

    class_uid = AUTH_CLASS if eid in AUTH_IDS else OTHER_CLASS
    severity = 3 if (status == "failure" or eid in ALERT_IDS) else 1

    return DecodedFields(
        class_uid=class_uid,
        category_uid=CATEGORY_UID,
        activity_id=activity_id,
        type_uid=class_uid * 100 + activity_id,
        severity_id=severity,
        username=username,
        data={k: v for k, v in data.items() if v not in (None, "", "-")},
    )