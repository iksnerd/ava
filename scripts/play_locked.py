"""Plays a command under the cross-process playback lock, for speak.sh.

Usage: play_locked.py LOCK TIMEOUT PIDFILE STOPPED CMD...

Concurrent speaks queue on LOCK (flock) instead of talking over each other,
and TIMEOUT bounds the held command so one wedged player cannot block every
other session's audio. It follows the same stop protocol as
internal/speaker's playLocked, and a contract test there runs both:

- PIDFILE only ever names the player. A stop SIGTERMs whatever it names, and
  killing this wrapper once the player had started left the player orphaned
  and playing.
- A stop writes STOPPED before it signals, so a speak still queued, or one
  stopped before its PID was published, gives up on seeing it.
"""

import contextlib
import errno
import fcntl
import os
import subprocess
import sys
import time

LOCK_POLL_SEC = 0.05


def main():
    lock_path, timeout, pidfile, stopped_path = sys.argv[1:5]
    cmd = sys.argv[5:]

    def stopped():
        return os.path.exists(stopped_path)

    with open(lock_path, "w") as lock:
        while True:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except OSError as e:
                if e.errno not in (errno.EWOULDBLOCK, errno.EAGAIN):
                    raise
            if stopped():
                return
            time.sleep(LOCK_POLL_SEC)

        # A stop between the last poll and the PID file below finds no player
        # to kill, so check on both sides of that gap.
        if stopped():
            return
        proc = subprocess.Popen(cmd)
        try:
            with open(pidfile, "w") as pf:
                pf.write(str(proc.pid))
            if stopped():
                proc.kill()
            proc.wait(timeout=float(timeout))
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
        finally:
            with contextlib.suppress(OSError):
                os.remove(pidfile)


if __name__ == "__main__":
    main()
