#!/usr/bin/env python3
"""
Records an asciinema v2 .cast file by driving otel-explorer on a
properly-sized pty. No asciinema binary needed — we create the pty
and capture output ourselves.

Usage: python3 docs/demo-driver.py [output.cast]
"""

import fcntl
import json
import os
import pty
import select
import struct
import sys
import termios
import time

COLS = 120
ROWS = 45
BINARY = "otel-explorer"
URL = "https://github.com/stefanpenner/gha-analyzer/pull/44"
OUTPUT = sys.argv[1] if len(sys.argv) > 1 else "docs/demo.cast"


def set_pty_size(fd, rows, cols):
    """Set the pty dimensions via ioctl."""
    winsize = struct.pack("HHHH", rows, cols, 0, 0)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, winsize)


def drain(master_fd, cast_file, start_time, timeout=0.1):
    """Read all available output from the pty and write to cast file."""
    output = b""
    while True:
        ready, _, _ = select.select([master_fd], [], [], timeout)
        if not ready:
            break
        try:
            chunk = os.read(master_fd, 65536)
            if not chunk:
                break
            output += chunk
        except OSError:
            break
    if output:
        t = round(time.time() - start_time, 6)
        text = output.decode("utf-8", errors="replace")
        cast_file.write(json.dumps([t, "o", text]) + "\n")
    return len(output) > 0


def send_key(master_fd, key, cast_file, start_time, pause=0.2):
    """Send a keystroke and drain output."""
    os.write(master_fd, key.encode() if isinstance(key, str) else key)
    time.sleep(pause)
    drain(master_fd, cast_file, start_time)


def main():
    # Open cast file and write header
    cast = open(OUTPUT, "w")
    cast.write(
        json.dumps(
            {
                "version": 2,
                "width": COLS,
                "height": ROWS,
                "timestamp": int(time.time()),
                "env": {"TERM": "xterm-256color", "SHELL": "/bin/zsh"},
            }
        )
        + "\n"
    )

    # Create pty at the correct size
    master_fd, slave_fd = pty.openpty()
    set_pty_size(master_fd, ROWS, COLS)
    set_pty_size(slave_fd, ROWS, COLS)

    # Spawn otel-explorer on the pty
    env = os.environ.copy()
    env["TERM"] = "xterm-256color"
    env["COLUMNS"] = str(COLS)
    env["LINES"] = str(ROWS)

    pid = os.fork()
    if pid == 0:
        # Child: attach to slave pty and exec
        os.close(master_fd)
        os.setsid()
        fcntl.ioctl(slave_fd, termios.TIOCSCTTY, 0)
        os.dup2(slave_fd, 0)
        os.dup2(slave_fd, 1)
        os.dup2(slave_fd, 2)
        if slave_fd > 2:
            os.close(slave_fd)
        os.execvpe(BINARY, [BINARY, URL], env)
        sys.exit(1)

    # Parent: drive the TUI
    os.close(slave_fd)
    start_time = time.time()

    # Wait for TUI to load — keep draining until we see "otel-explorer"
    loaded = False
    for _ in range(100):
        time.sleep(0.2)
        drain(master_fd, cast, start_time, timeout=0.2)
        # Check recent output by peeking at cast file content
        cast.flush()
        with open(OUTPUT) as f:
            content = f.read()
        if "otel-explorer" in content:
            loaded = True
            break

    if not loaded:
        print("WARNING: TUI may not have loaded", file=sys.stderr)

    # Let the initial view settle
    time.sleep(1)
    drain(master_fd, cast, start_time)

    # --- Scripted interaction ---

    # Navigate down through the tree
    for _ in range(3):
        send_key(master_fd, "j", cast, start_time, pause=0.2)

    # Expand a node
    send_key(master_fd, "\r", cast, start_time, pause=0.6)

    # Navigate into children
    for _ in range(3):
        send_key(master_fd, "j", cast, start_time, pause=0.2)

    # Expand another node
    send_key(master_fd, "\r", cast, start_time, pause=0.6)

    # Navigate down to see steps
    for _ in range(4):
        send_key(master_fd, "j", cast, start_time, pause=0.2)

    # Pause to show the tree + timeline
    time.sleep(1.5)
    drain(master_fd, cast, start_time)

    # Open detail/inspector view
    send_key(master_fd, "i", cast, start_time, pause=1.5)

    # Navigate the inspector
    for _ in range(3):
        send_key(master_fd, "j", cast, start_time, pause=0.2)

    # Expand an inspector section
    send_key(master_fd, "\r", cast, start_time, pause=0.5)
    send_key(master_fd, "j", cast, start_time, pause=0.2)
    send_key(master_fd, "j", cast, start_time, pause=0.2)

    # Show the detail view
    time.sleep(1.5)
    drain(master_fd, cast, start_time)

    # Close the inspector
    send_key(master_fd, "q", cast, start_time, pause=0.5)

    # Demo search: type /build
    send_key(master_fd, "/", cast, start_time, pause=0.3)
    for ch in "build":
        send_key(master_fd, ch, cast, start_time, pause=0.1)

    time.sleep(1)
    drain(master_fd, cast, start_time)

    # Exit search (Escape), keeping filter
    send_key(master_fd, "\x1b", cast, start_time, pause=1)

    # Clear search (Escape)
    send_key(master_fd, "\x1b", cast, start_time, pause=0.5)

    # Final pause on the full view
    time.sleep(1.5)
    drain(master_fd, cast, start_time)

    # Quit
    os.write(master_fd, b"q")
    time.sleep(0.5)
    drain(master_fd, cast, start_time)

    # Clean up
    cast.close()
    try:
        os.waitpid(pid, 0)
    except ChildProcessError:
        pass
    os.close(master_fd)

    print(f"Recorded to {OUTPUT}")


if __name__ == "__main__":
    main()
