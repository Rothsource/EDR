import logging
from typing import Any, Callable, Optional, TypedDict
from . import unmatched

logger = logging.getLogger(__name__)

PARSER_VERSION = 2  # bump whenever any decoder's logic changes


class DecodedFields(TypedDict):
    class_uid: int
    category_uid: int
    activity_id: int
    type_uid: int
    severity_id: int
    username: Optional[str]
    data: dict[str, Any]  # normalized, replaces the raw envelope


DECODERS: dict[str, Callable[[dict], Optional[DecodedFields]]] = {}


def register(source: str):
    """Decorator for decoder modules: @register("journald")"""
    def _wrap(fn: Callable[[dict], Optional[DecodedFields]]):
        DECODERS[source] = fn
        return fn
    return _wrap


async def decode_event(source, raw, db=None):
    if not source:
        logger.warning("event has data.raw but no data.source")
        return None

    decoder = DECODERS.get(source)
    if decoder is None:
        logger.warning("no decoder registered for source=%r", source)
        return None

    try:
        result = decoder(raw)
    except Exception:
        logger.exception("decoder for source=%r raised on raw record", source)
        return None

    if result is None and db is not None:
        identifier = raw.get("identifier") if isinstance(raw, dict) else None
        await unmatched.store_unmatched(db, source, identifier, raw)

    return result


# Decoder modules must be imported AFTER register/DecodedFields/PARSER_VERSION
# are defined, because they import those names back from this module.
# Importing them is what runs their @register(...) decorators.
from . import journald           # noqa: E402, F401
from . import windows_security   # noqa: E402, F401