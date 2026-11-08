"""Exclusive temporary files for private derived reports."""
import os
from pathlib import Path
import tempfile


def write_atomic(output, data):
    """Replace a derived file without colliding with inputs or concurrent exporters."""
    output = Path(output)
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(dir=output.parent, prefix=".ebird-", suffix=".tmp")
    try:
        with os.fdopen(descriptor, "wb") as target:
            target.write(data)
        os.replace(temporary, output)
    finally:
        Path(temporary).unlink(missing_ok=True)
