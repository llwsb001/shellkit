"""Screenshot the page. Not part of the build: a human wants a picture, CI does
not. Uses playwright if it is installed, and tells you how to install it if not.
"""
import os, subprocess, sys, tempfile, time, urllib.request

PORT = 3451
HOME = tempfile.mkdtemp()
env = dict(os.environ, SHELLKIT_HOME=HOME, SHELLKIT_ADDR=f"127.0.0.1:{PORT}")

subprocess.run(["./bin/shellkit", "add", "-t", "screenshot", "capture from the CLI"],
               env=env, check=True, stdout=subprocess.DEVNULL)
subprocess.run(["./bin/shellkit", "add", "-t", "screenshot", "the same file the page reads"],
               env=env, check=True, stdout=subprocess.DEVNULL)

proc = subprocess.Popen(["./bin/shellkit", "web"], env=env,
                        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    for _ in range(60):
        try:
            urllib.request.urlopen(f"http://127.0.0.1:{PORT}/healthz", timeout=1)
            break
        except Exception:
            time.sleep(0.2)
    from playwright.sync_api import sync_playwright
    with sync_playwright() as p:
        b = p.chromium.launch()
        page = b.new_page(viewport={"width": 900, "height": 760})
        page.goto(f"http://127.0.0.1:{PORT}/")
        page.wait_for_timeout(800)
        out = sys.argv[1] if len(sys.argv) > 1 else "page.png"
        page.screenshot(path=out)
        b.close()
        print(out)
except ImportError:
    print("playwright not installed: pip install playwright && playwright install chromium", file=sys.stderr)
    sys.exit(2)
finally:
    proc.terminate()
