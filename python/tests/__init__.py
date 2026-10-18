"""Tests for the carragent package. They run offline, against a server on this machine: ``python3 -m unittest``.

The package is imported from ../src, so nothing has to be installed first.
"""

import os
import sys

_SRC = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "src")
if _SRC not in sys.path:
    sys.path.insert(0, _SRC)

# The tests only talk to 127.0.0.1. A proxy set in the environment must not get in the way.
for _name in ("no_proxy", "NO_PROXY"):
    _bypass = [part for part in os.environ.get(_name, "").split(",") if part]
    os.environ[_name] = ",".join([*_bypass, "127.0.0.1", "localhost"])
