"""tmux session lifecycle for the test-evo harness."""

from __future__ import annotations

import os
import signal
import subprocess
import time


def session_exists(name: str) -> bool:
    result = subprocess.run(
        ["tmux", "has-session", "-t", name],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    )
    return result.returncode == 0


def pane_is_dead(name: str) -> bool:
    try:
        out = subprocess.check_output(
            ["tmux", "list-panes", "-t", name, "-F", "#{pane_dead}"],
            text=True,
            stderr=subprocess.DEVNULL,
        )
    except (subprocess.CalledProcessError, FileNotFoundError):
        return True
    line = out.strip().splitlines()[0] if out.strip() else "1"
    return line == "1"


def pane_pid(name: str) -> int | None:
    try:
        out = subprocess.check_output(
            ["tmux", "list-panes", "-t", name, "-F", "#{pane_pid}"],
            text=True,
            stderr=subprocess.DEVNULL,
        )
    except (subprocess.CalledProcessError, FileNotFoundError):
        return None
    line = out.strip().splitlines()[0] if out.strip() else ""
    if not line or line == "-1":
        return None
    try:
        return int(line)
    except ValueError:
        return None


def kill_process_group(pid: int) -> None:
    """Signal the process group so timeout --foreground leftovers die too."""
    try:
        pgid = os.getpgid(pid)
    except ProcessLookupError:
        return
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(pgid, sig)
        except ProcessLookupError:
            return
        except PermissionError:
            return
        time.sleep(0.05)


def kill_session(name: str, *, command_pid: int | None = None) -> None:
    """Best-effort teardown of the harness tmux session and command process group."""
    if command_pid is not None:
        kill_process_group(command_pid)
    live_pid = pane_pid(name)
    if live_pid is not None and live_pid != command_pid:
        kill_process_group(live_pid)
    if not session_exists(name):
        return
    subprocess.run(
        ["tmux", "kill-session", "-t", name],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    )


def capture_pane(name: str) -> str:
    return subprocess.check_output(
        ["tmux", "capture-pane", "-t", name, "-e", "-p"],
        text=True,
        stderr=subprocess.DEVNULL,
    )


def create_idle_session(
    *,
    session: str,
    cols: int,
    rows: int,
    start_dir: str,
    path_env: str,
) -> None:
    """Create a detached pane that waits, so we can photograph it before submit."""
    idle = [
        "tmux",
        "new-session",
        "-d",
        "-s",
        session,
        "-x",
        str(cols),
        "-y",
        str(rows),
        "-c",
        start_dir,
        "-e",
        f"PATH={path_env}",
        "--",
        "bash",
        "--noprofile",
        "--norc",
        "-c",
        "exec sleep 999",
    ]
    subprocess.run(idle, check=True)
    subprocess.run(
        ["tmux", "set-option", "-t", session, "remain-on-exit", "on"],
        check=True,
    )


def submit_runner(session: str, runner_path: str) -> None:
    """Replace the idle pane with the timeout-wrapped command."""
    subprocess.run(
        [
            "tmux",
            "respawn-pane",
            "-k",
            "-t",
            session,
            "--",
            "bash",
            "--noprofile",
            "--norc",
            runner_path,
        ],
        check=True,
    )
