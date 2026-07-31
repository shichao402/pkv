#!/usr/bin/env python3
# PKV uninstall helper — Python 3 stdlib only (no third-party packages).
"""Thoroughly remove local PKV installation artifacts on Windows, macOS, and Linux.

Can be invoked by `pkv uninstall` (embedded) or standalone:

  python3 uninstall.py --yes
  python3 uninstall.py --exe /path/to/pkv --pid 12345 --yes --log /tmp/pkv-uninstall.log

Output is best-effort: when the process has no usable stdout (for example a
detached Windows process), messages still land in the --log file.
"""

from __future__ import print_function

import argparse
import glob
import json
import os
import shutil
import sys
import time
import traceback

IS_WIN = sys.platform == "win32"

PKV_SSH_PREFIX = "pkv_"
MARKER_START_PREFIX = "# >>> PKV MANAGED START"
MARKER_END_PREFIX = "# >>> PKV MANAGED END"
LEGACY_RC_NEEDLES = (
    ".pkv/env",
    "pkv/env.sh",
    "source ~/.pkv",
    "source $HOME/.pkv",
)
MCP_SERVER_NAME = "dec-pkv-mcp"

_log_fh = None


def open_log(path):
    global _log_fh
    if not path:
        return
    try:
        _log_fh = open(path, "a", encoding="utf-8")
    except Exception:
        _log_fh = None


def close_log():
    global _log_fh
    if _log_fh is not None:
        try:
            _log_fh.close()
        except Exception:
            pass
        _log_fh = None


def emit(level, msg, stream=None):
    """Write a message to the log file and stdout/stderr, never raising.

    A detached Windows process inherits console handles it cannot use, so any
    write here may fail. Losing output must never abort the uninstall.
    """
    line = "[%s] %s" % (level, msg)
    if _log_fh is not None:
        try:
            _log_fh.write(line + "\n")
            _log_fh.flush()
        except Exception:
            pass
    try:
        target = stream or sys.stdout
        if target is not None:
            print(line, file=target)
            target.flush()
    except Exception:
        pass


def info(msg):
    emit("INFO", msg)


def warn(msg):
    emit("WARN", msg)


def error(msg):
    emit("ERROR", msg, stream=sys.stderr)


def home_dir():
    return os.path.expanduser("~")


def pkv_home():
    return os.path.join(home_dir(), ".pkv")


def default_install_dir():
    if IS_WIN:
        local = os.environ.get("LOCALAPPDATA") or os.path.join(home_dir(), "AppData", "Local")
        return os.path.join(local, "pkv")
    return os.path.join(home_dir(), ".local", "bin")


def default_binary_path():
    if IS_WIN:
        return os.path.join(default_install_dir(), "pkv.exe")
    return os.path.join(default_install_dir(), "pkv")


def run_capture(argv):
    """Run a helper command and return stdout, or None on any failure."""
    import subprocess

    kwargs = {
        "stdin": subprocess.DEVNULL,
        "stderr": subprocess.DEVNULL,
        "universal_newlines": True,
    }
    if IS_WIN and hasattr(subprocess, "CREATE_NO_WINDOW"):
        kwargs["creationflags"] = subprocess.CREATE_NO_WINDOW
    try:
        return subprocess.check_output(argv, **kwargs)
    except Exception:
        return None


def pid_alive(pid):
    if not pid or pid <= 0:
        return False
    if IS_WIN:
        import ctypes

        SYNCHRONIZE = 0x00100000
        handle = ctypes.windll.kernel32.OpenProcess(SYNCHRONIZE, False, int(pid))
        if handle:
            ctypes.windll.kernel32.CloseHandle(handle)
            return True
        return False
    try:
        os.kill(int(pid), 0)
    except OSError:
        return False
    return True


def wait_for_pid(pid, timeout=60.0, interval=0.25):
    if not pid or pid <= 0:
        return True
    deadline = time.time() + timeout
    while pid_alive(pid):
        if time.time() >= deadline:
            return False
        time.sleep(interval)
    # Brief settle so Windows releases file handles.
    time.sleep(0.5)
    return True


def list_pkv_pids(exclude=None):
    """Best-effort discovery of other running pkv processes."""
    exclude = set(exclude or [])
    exclude.add(os.getpid())
    found = set()

    if IS_WIN:
        out = run_capture(["tasklist", "/FI", "IMAGENAME eq pkv.exe", "/FO", "CSV", "/NH"])
        if out is None:
            warn("could not enumerate pkv.exe processes")
        else:
            for line in out.splitlines():
                line = line.strip()
                if not line or line.lower().startswith("info:"):
                    continue
                parts = [p.strip().strip('"') for p in line.split(",")]
                if len(parts) >= 2 and parts[0].strip('"').lower() == "pkv.exe":
                    try:
                        found.add(int(parts[1]))
                    except ValueError:
                        pass
    else:
        out = run_capture(["pgrep", "-x", "pkv"])
        if out is not None:
            for line in out.splitlines():
                line = line.strip()
                if line.isdigit():
                    found.add(int(line))
        else:
            proc_root = "/proc"
            if os.path.isdir(proc_root):
                for name in os.listdir(proc_root):
                    if not name.isdigit():
                        continue
                    try:
                        with open(os.path.join(proc_root, name, "cmdline"), "rb") as fh:
                            raw = fh.read().replace(b"\x00", b" ")
                        text = raw.decode("utf-8", "replace")
                    except Exception:
                        continue
                    argv0 = text.split(" ", 1)[0]
                    if os.path.basename(argv0) == "pkv":
                        found.add(int(name))

    return sorted(pid for pid in found if pid not in exclude)


def wait_for_other_pkv(exclude, timeout=30.0):
    deadline = time.time() + timeout
    while True:
        leftovers = list_pkv_pids(exclude=exclude)
        if not leftovers:
            return True
        if time.time() >= deadline:
            warn("other pkv process(es) still running: %s" % leftovers)
            return False
        info("waiting for other pkv process(es) to exit: %s" % leftovers)
        time.sleep(1.0)


def safe_remove(path):
    if not path:
        return False
    try:
        if os.path.isdir(path) and not os.path.islink(path):
            shutil.rmtree(path)
        elif os.path.lexists(path):
            os.remove(path)
        else:
            return False
        info("removed %s" % path)
        return True
    except Exception as exc:
        warn("failed to remove %s: %s" % (path, exc))
        return False


def collapse_blank_lines(text):
    lines = text.splitlines()
    out = []
    prev_blank = False
    for line in lines:
        blank = line.strip() == ""
        if blank and prev_blank:
            continue
        out.append(line)
        prev_blank = blank
    result = "\n".join(out)
    if text.endswith("\n") and result and not result.endswith("\n"):
        result += "\n"
    return result


def strip_pkv_managed_blocks(content):
    lines = content.splitlines(True)
    out = []
    in_block = False
    changed = False
    for line in lines:
        trimmed = line.strip()
        if trimmed.startswith(MARKER_START_PREFIX):
            in_block = True
            changed = True
            continue
        if trimmed.startswith(MARKER_END_PREFIX):
            in_block = False
            changed = True
            continue
        if not in_block:
            out.append(line)
    if not changed:
        return content, False
    return collapse_blank_lines("".join(out)), True


def rewrite_file(path, transform):
    if not os.path.isfile(path):
        return False
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as fh:
            original = fh.read()
    except Exception as exc:
        warn("failed to read %s: %s" % (path, exc))
        return False
    updated, changed = transform(original)
    if not changed:
        return False
    try:
        with open(path, "w", encoding="utf-8", newline="") as fh:
            fh.write(updated)
        info("cleaned %s" % path)
        return True
    except Exception as exc:
        warn("failed to write %s: %s" % (path, exc))
        return False


def clean_ssh_artifacts():
    ssh_dir = os.path.join(home_dir(), ".ssh")
    if not os.path.isdir(ssh_dir):
        return

    for path in glob.glob(os.path.join(ssh_dir, PKV_SSH_PREFIX + "*")):
        safe_remove(path)

    rewrite_file(os.path.join(ssh_dir, "config"), strip_pkv_managed_blocks)
    rewrite_file(os.path.join(ssh_dir, "known_hosts"), strip_pkv_managed_blocks)


def load_state():
    path = os.path.join(pkv_home(), "state.json")
    if not os.path.isfile(path):
        return None, path
    try:
        with open(path, "r", encoding="utf-8") as fh:
            return json.load(fh), path
    except Exception as exc:
        warn("failed to parse %s: %s" % (path, exc))
        return None, path


def clean_tracked_from_state(state):
    if not state:
        return

    for entry in state.get("ssh_keys") or []:
        for key in ("key_file", "pub_file"):
            safe_remove(entry.get(key))

    for entry in state.get("envs") or []:
        for key in ("json_path", "shell_path", "powershell_path"):
            safe_remove(entry.get(key))
        # Legacy Windows persistent user env vars (pre artifact-file era).
        if IS_WIN:
            for name in entry.get("keys") or []:
                remove_windows_user_env(name)

    for entry in state.get("notes") or []:
        path = entry.get("file_path")
        if path:
            safe_remove(path)
            prune_empty_parents(path, entry.get("target_dir"))

    for entry in state.get("workspaces") or []:
        root = entry.get("root_path")
        if root:
            safe_remove(os.path.join(root, ".pkv"))
            clean_mcp_configs_under(root)


def prune_empty_parents(file_path, stop_dir):
    parent = os.path.dirname(file_path)
    stop = os.path.abspath(stop_dir) if stop_dir else None
    while parent and os.path.isdir(parent):
        abs_parent = os.path.abspath(parent)
        if stop and abs_parent == stop:
            break
        if abs_parent == os.path.abspath(home_dir()):
            break
        try:
            os.rmdir(parent)
            info("removed empty directory %s" % parent)
        except OSError:
            break
        parent = os.path.dirname(parent)


def clean_shell_rc_files():
    for name in (".bashrc", ".zshrc", ".bash_profile", ".zprofile", ".profile"):
        path = os.path.join(home_dir(), name)
        if not os.path.isfile(path):
            continue

        def transform(content, _needles=LEGACY_RC_NEEDLES):
            kept = []
            changed = False
            for line in content.splitlines(True):
                if any(needle in line for needle in _needles):
                    changed = True
                    continue
                kept.append(line)
            if not changed:
                return content, False
            return "".join(kept), True

        rewrite_file(path, transform)


def remove_windows_user_env(name):
    if not name or not IS_WIN:
        return
    try:
        import winreg

        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, "Environment", 0, winreg.KEY_ALL_ACCESS) as key:
            try:
                winreg.DeleteValue(key, name)
                info("removed user env var %s" % name)
            except FileNotFoundError:
                pass
    except Exception as exc:
        warn("failed to remove user env var %s: %s" % (name, exc))


def clean_windows_path(install_dir):
    if not IS_WIN or not install_dir:
        return
    install_dir = os.path.normpath(install_dir)
    try:
        import ctypes
        import winreg

        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, "Environment", 0, winreg.KEY_ALL_ACCESS) as key:
            try:
                current, reg_type = winreg.QueryValueEx(key, "Path")
            except FileNotFoundError:
                return
            parts = [p for p in str(current).split(";") if p]
            kept = []
            removed = False
            for part in parts:
                if os.path.normpath(os.path.expandvars(part)) == install_dir:
                    removed = True
                    continue
                kept.append(part)
            if not removed:
                return
            winreg.SetValueEx(key, "Path", 0, reg_type, ";".join(kept))
            info("removed %s from user PATH" % install_dir)

        HWND_BROADCAST = 0xFFFF
        WM_SETTINGCHANGE = 0x001A
        SMTO_ABORTIFHUNG = 0x0002
        ctypes.windll.user32.SendMessageTimeoutW(
            HWND_BROADCAST, WM_SETTINGCHANGE, 0, "Environment", SMTO_ABORTIFHUNG, 5000, None
        )
    except Exception as exc:
        warn("failed to clean user PATH: %s" % exc)


def remove_json_mcp_server(path):
    if not os.path.isfile(path):
        return False
    try:
        with open(path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
    except Exception as exc:
        warn("failed to parse MCP config %s: %s" % (path, exc))
        return False

    servers = data.get("mcpServers") if isinstance(data, dict) else None
    if not isinstance(servers, dict) or MCP_SERVER_NAME not in servers:
        return False
    del servers[MCP_SERVER_NAME]
    try:
        with open(path, "w", encoding="utf-8", newline="\n") as fh:
            json.dump(data, fh, indent=2)
            fh.write("\n")
        info("removed %s from %s" % (MCP_SERVER_NAME, path))
        return True
    except Exception as exc:
        warn("failed to update MCP config %s: %s" % (path, exc))
        return False


def clean_mcp_configs_under(root):
    if not root:
        return
    for rel in (
        os.path.join(".cursor", "mcp.json"),
        os.path.join(".claude", "mcp.json"),
        ".mcp.json",
        "mcp.json",
    ):
        remove_json_mcp_server(os.path.join(root, rel))


def clean_user_mcp_configs():
    candidates = [
        os.path.join(home_dir(), ".cursor", "mcp.json"),
        os.path.join(home_dir(), ".claude", "mcp.json"),
        os.path.join(home_dir(), ".mcp.json"),
    ]
    if IS_WIN:
        appdata = os.environ.get("APPDATA")
        if appdata:
            candidates.append(os.path.join(appdata, "Cursor", "User", "mcp.json"))
    for path in candidates:
        remove_json_mcp_server(path)


def remove_binary(exe_path):
    if not exe_path:
        return
    exe_path = os.path.abspath(exe_path)
    safe_remove(exe_path + ".bak")

    for attempt in range(20):
        if safe_remove(exe_path):
            break
        if not os.path.lexists(exe_path):
            break
        if attempt == 0:
            info("binary still locked, retrying: %s" % exe_path)
        time.sleep(0.5)
    else:
        if os.path.lexists(exe_path):
            warn("could not delete binary (still locked): %s" % exe_path)
            if IS_WIN:
                schedule_windows_delete_on_reboot(exe_path)

    parent = os.path.dirname(exe_path)
    if IS_WIN and os.path.abspath(parent) == os.path.abspath(default_install_dir()):
        try:
            if os.path.isdir(parent) and not os.listdir(parent):
                os.rmdir(parent)
                info("removed empty install directory %s" % parent)
        except Exception as exc:
            warn("failed to remove install directory %s: %s" % (parent, exc))


def schedule_windows_delete_on_reboot(path):
    try:
        import ctypes

        MOVEFILE_DELAY_UNTIL_REBOOT = 0x4
        if ctypes.windll.kernel32.MoveFileExW(str(path), None, MOVEFILE_DELAY_UNTIL_REBOOT):
            warn("scheduled delete-on-reboot for %s" % path)
        else:
            warn("MoveFileExW failed for %s" % path)
    except Exception as exc:
        warn("failed to schedule reboot deletion for %s: %s" % (path, exc))


def clean_stale_helper_files(current_script):
    """Remove helper scripts left behind by earlier uninstall attempts."""
    current = os.path.abspath(current_script) if current_script else ""
    tmp_dir = os.environ.get("TEMP") or os.environ.get("TMPDIR") or "/tmp"
    for path in glob.glob(os.path.join(tmp_dir, "pkv-uninstall-*.py")):
        if os.path.abspath(path) == current:
            continue
        try:
            os.remove(path)
            info("removed stale helper script %s" % path)
        except Exception:
            pass


def self_delete_script(script_path):
    if not script_path:
        return
    script_path = os.path.abspath(script_path)
    if "pkv-uninstall-" not in os.path.basename(script_path):
        return
    try:
        os.remove(script_path)
    except Exception:
        if IS_WIN:
            schedule_windows_delete_on_reboot(script_path)


def resolve_exe(explicit):
    if explicit:
        return os.path.abspath(explicit)
    from shutil import which

    found = which("pkv") or which("pkv.exe")
    if found:
        return os.path.abspath(found)
    candidate = default_binary_path()
    if os.path.lexists(candidate):
        return os.path.abspath(candidate)
    return None


def confirm_or_exit(assume_yes):
    if assume_yes:
        return
    try:
        reply = input("This will permanently remove local PKV data and the binary. Continue? [y/N] ")
    except EOFError:
        reply = ""
    if reply.strip().lower() not in ("y", "yes"):
        print("Aborted.")
        sys.exit(1)


def run(args):
    confirm_or_exit(args.yes)

    info("PKV uninstall started (pid=%d, platform=%s)" % (os.getpid(), sys.platform))

    exclude = {os.getpid()}
    exclude.update(int(pid) for pid in (args.ignore_pid or []))

    if args.pid:
        exclude.add(int(args.pid))
        info("waiting for caller process pid=%s to exit..." % args.pid)
        if not wait_for_pid(int(args.pid), timeout=args.wait_timeout):
            warn("timed out waiting for pid %s; continuing anyway" % args.pid)

    if not wait_for_other_pkv(exclude=exclude, timeout=min(30.0, args.wait_timeout)):
        warn("continuing uninstall while other pkv processes may still be active")

    state, _ = load_state()
    info("cleaning tracked PKV resources...")
    clean_tracked_from_state(state)

    info("cleaning SSH managed files and markers...")
    clean_ssh_artifacts()

    info("cleaning legacy shell rc references...")
    clean_shell_rc_files()

    info("cleaning MCP server entries...")
    clean_user_mcp_configs()

    exe_path = resolve_exe(args.exe)
    install_dir = os.path.dirname(exe_path) if exe_path else default_install_dir()

    if IS_WIN:
        clean_windows_path(install_dir)

    info("removing %s ..." % pkv_home())
    safe_remove(pkv_home())

    if exe_path and os.path.lexists(exe_path):
        info("removing binary %s ..." % exe_path)
        remove_binary(exe_path)
    else:
        warn("pkv binary not found; skipped binary deletion")

    clean_stale_helper_files(args.script or __file__)

    info("PKV uninstall finished.")
    if IS_WIN:
        info("Open a new terminal so PATH changes take effect.")

    close_log()
    self_delete_script(args.script or __file__)
    return 0


def build_parser():
    parser = argparse.ArgumentParser(description="Uninstall PKV local artifacts (stdlib only).")
    parser.add_argument("--yes", "-y", action="store_true", help="Skip confirmation prompt")
    parser.add_argument("--exe", help="Absolute path to the pkv binary to delete")
    parser.add_argument("--pid", type=int, default=0, help="Caller PID to wait for before deleting binary")
    parser.add_argument(
        "--ignore-pid",
        type=int,
        action="append",
        default=[],
        help="PID to ignore when waiting for running pkv processes (repeatable)",
    )
    parser.add_argument(
        "--wait-timeout",
        type=float,
        default=60.0,
        help="Seconds to wait for processes before continuing",
    )
    parser.add_argument("--log", help="Append progress to this log file")
    parser.add_argument("--script", help="Path of this helper script (used for temp self-cleanup)")
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    open_log(args.log)
    try:
        return run(args)
    except KeyboardInterrupt:
        error("interrupted")
        return 130
    except Exception:
        error("uninstall failed:\n" + traceback.format_exc())
        return 1
    finally:
        close_log()


if __name__ == "__main__":
    sys.exit(main())
