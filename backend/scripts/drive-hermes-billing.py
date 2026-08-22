"""Drive the REAL hermes billing surfaces against a live Inferno.

Only the credential lookup is stubbed. build_billing_state(), the HTTP request,
the parse and the render all run for real -- this is the CLI's own code path,
not a parser-level substitute.
"""
import io, os, sys, contextlib
FORK = "/private/tmp/claude-501/-Users-saksham/c3a80d4a-8972-4b31-bcab-00317d53a8f7/scratchpad/hermes-fork"
sys.path.insert(0, FORK)
os.environ["HERMES_PORTAL_BASE_URL"] = "http://127.0.0.1:18480"
os.environ.setdefault("NO_COLOR", "1")

TOKEN = open("/tmp/t6-token.txt").read().strip()

import hermes_cli.auth as auth
auth.get_provider_auth_state = lambda pid: (
    {"access_token": TOKEN, "provider": "nous",
     "portal_base_url": "http://127.0.0.1:18480"} if pid == "nous" else None)
# resolve_nous_access_token owns refresh; we already hold a live token.
auth.resolve_nous_access_token = lambda *a, **k: TOKEN
import hermes_cli.nous_billing as nb
if hasattr(nb, "get_provider_auth_state"):
    nb.get_provider_auth_state = auth.get_provider_auth_state

from hermes_cli.cli_billing_mixin import CLIBillingMixin

class Harness(CLIBillingMixin):
    """Minimal host: the mixin only needs a few attrs for non-interactive render."""
    def __init__(self):
        self.app = None
        self.session = None
        self.is_interactive = False

def run(label, fn):
    buf = io.StringIO()
    print(f"\n{'='*64}\n### {label}\n{'='*64}")
    try:
        with contextlib.redirect_stdout(buf):
            fn()
        out = buf.getvalue()
    except Exception as e:
        out = buf.getvalue() + f"\n[EXCEPTION] {type(e).__name__}: {e}"
    print(out if out.strip() else "[no output]")
    return out

h = Harness()
run("/topup  -> _show_billing()", lambda: h._show_billing("/topup"))
if hasattr(h, "_show_subscription"):
    run("/subscription -> _show_subscription()", lambda: h._show_subscription())
else:
    print("\n[note] _show_subscription not on this mixin; searching...")
