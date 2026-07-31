#!/usr/bin/env python3
"""Sandboxed end-to-end check for uninstall.py.

Builds a fake HOME + install dir, runs the uninstall helper against it, and
asserts every artifact is gone. Touches nothing outside the temp sandbox.
"""

import json
import os
import subprocess
import sys
import tempfile

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCRIPT = os.path.join(REPO, "uninstall.py")
IS_WIN = sys.platform == "win32"


def write(path, content):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(content)
    return path


def main():
    sandbox = tempfile.mkdtemp(prefix="pkv-uninstall-sandbox-")
    home = os.path.join(sandbox, "home")
    install_dir = os.path.join(sandbox, "home", "AppData", "Local", "pkv") if IS_WIN else os.path.join(home, ".local", "bin")
    project = os.path.join(sandbox, "project")

    exe = write(os.path.join(install_dir, "pkv.exe" if IS_WIN else "pkv"), "binary")
    note = write(os.path.join(project, "nested", "app.secrets.json"), "{}")
    key = write(os.path.join(home, ".ssh", "pkv_demo"), "PRIVATE")
    pub = write(os.path.join(home, ".ssh", "pkv_demo.pub"), "PUBLIC")
    env_json = write(os.path.join(home, ".pkv", "env", "dev.json"), "{}")

    ssh_config = write(
        os.path.join(home, ".ssh", "config"),
        "Host keepme\n    HostName keep.example\n\n"
        "# >>> PKV MANAGED START: demo <<<\n"
        "Host gone\n    IdentityFile ~/.ssh/pkv_demo\n"
        "# >>> PKV MANAGED END: demo <<<\n",
    )
    known_hosts = write(
        os.path.join(home, ".ssh", "known_hosts"),
        "keep.example ssh-ed25519 AAAA\n"
        "# >>> PKV MANAGED START <<<\ngone.example ssh-ed25519 BBBB\n# >>> PKV MANAGED END <<<\n",
    )
    mcp = write(
        os.path.join(home, ".cursor", "mcp.json"),
        json.dumps({"mcpServers": {"dec-pkv-mcp": {"command": "pkv"}, "other": {"command": "x"}}}, indent=2),
    )
    rc = write(os.path.join(home, ".zshrc"), "export EDITOR=vim\nsource ~/.pkv/env/dev.sh\n")

    write(
        os.path.join(home, ".pkv", "state.json"),
        json.dumps(
            {
                "ssh_keys": [{"item_id": "1", "key_name": "demo", "key_file": key, "pub_file": pub}],
                "envs": [{"item_id": "2", "folder": "dev", "json_path": env_json}],
                "notes": [{"item_id": "3", "file_path": note, "target_dir": project}],
                "workspaces": [{"root_path": project, "folder": "dev"}],
            }
        ),
    )
    write(os.path.join(project, ".pkv", "workspace.yaml"), "folder: dev\n")
    write(os.path.join(project, ".cursor", "mcp.json"), json.dumps({"mcpServers": {"dec-pkv-mcp": {}}}))

    log = os.path.join(sandbox, "uninstall.log")
    env = dict(os.environ)
    env["HOME"] = home
    env["USERPROFILE"] = home
    env["LOCALAPPDATA"] = os.path.join(home, "AppData", "Local")
    env.pop("APPDATA", None)

    proc = subprocess.run(
        [sys.executable, SCRIPT, "--yes", "--exe", exe, "--log", log, "--wait-timeout", "5"],
        env=env,
        capture_output=True,
        text=True,
    )

    failures = []
    if proc.returncode != 0:
        failures.append("exit code %s\nstdout:\n%s\nstderr:\n%s" % (proc.returncode, proc.stdout, proc.stderr))

    for label, path in (("binary", exe), ("pkv home", os.path.join(home, ".pkv")),
                        ("ssh key", key), ("ssh pub", pub), ("note", note),
                        ("workspace .pkv", os.path.join(project, ".pkv"))):
        if os.path.exists(path):
            failures.append("%s still exists: %s" % (label, path))

    config_text = open(ssh_config, encoding="utf-8").read()
    if "PKV MANAGED" in config_text or "Host gone" in config_text:
        failures.append("ssh config not cleaned:\n" + config_text)
    if "Host keepme" not in config_text:
        failures.append("ssh config lost unrelated entry:\n" + config_text)

    kh_text = open(known_hosts, encoding="utf-8").read()
    if "gone.example" in kh_text or "PKV MANAGED" in kh_text:
        failures.append("known_hosts not cleaned:\n" + kh_text)
    if "keep.example" not in kh_text:
        failures.append("known_hosts lost unrelated entry:\n" + kh_text)

    mcp_data = json.load(open(mcp, encoding="utf-8"))
    if "dec-pkv-mcp" in mcp_data["mcpServers"]:
        failures.append("user MCP entry not removed")
    if "other" not in mcp_data["mcpServers"]:
        failures.append("unrelated MCP entry removed")

    proj_mcp = json.load(open(os.path.join(project, ".cursor", "mcp.json"), encoding="utf-8"))
    if "dec-pkv-mcp" in proj_mcp["mcpServers"]:
        failures.append("workspace MCP entry not removed")

    rc_text = open(rc, encoding="utf-8").read()
    if ".pkv/env" in rc_text:
        failures.append("shell rc not cleaned:\n" + rc_text)
    if "EDITOR=vim" not in rc_text:
        failures.append("shell rc lost unrelated line:\n" + rc_text)

    if not os.path.isfile(log) or "PKV uninstall finished." not in open(log, encoding="utf-8").read():
        failures.append("log file missing completion marker: %s" % log)

    if failures:
        print("FAIL")
        for item in failures:
            print(" - " + item)
        return 1

    print("PASS: sandbox uninstall cleaned every artifact")
    print("sandbox: %s" % sandbox)
    return 0


if __name__ == "__main__":
    sys.exit(main())
