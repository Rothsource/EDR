import json
import logging
from sqlalchemy import text

logger = logging.getLogger(__name__)

MAX_ROWS = 10_000


async def store_unmatched(db, source: str, identifier, raw: dict) -> None:
    try:
        await db.execute(
            text("INSERT INTO unmatched_raw (source, identifier, raw) VALUES (:source, :identifier, :raw)"),
            {"source": source, "identifier": identifier, "raw": json.dumps(raw)},
        )
        await db.execute(
            text("""
                DELETE FROM unmatched_raw
                WHERE id IN (
                    SELECT id FROM unmatched_raw ORDER BY id DESC OFFSET :max_rows
                )
            """),
            {"max_rows": MAX_ROWS},
        )
    except Exception:
        logger.exception("failed to store unmatched raw record (source=%r)", source)
        raise