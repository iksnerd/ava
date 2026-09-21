"""Tests for the two pieces of realtime.py that are pure computation.

Both matter and neither was covered. A high-pass bug degrades every
transcription silently — the audio still sounds fine to a human. And device
resolution is the first thing a user hits, because sounddevice indices shift
whenever an audio device is added or removed.
"""

import json

import numpy as np
import pytest


class TestHighPassFilter:
    def test_passes_a_tone_well_above_the_cutoff(self, realtime):
        rt = realtime()
        f = rt.HighPassFilter(cutoff_hz=80)
        t = np.arange(16000, dtype=np.float32) / 16000
        tone = np.sin(2 * np.pi * 1000 * t).astype(np.float32)

        out = f(tone)
        # Skip the transient while filter state settles.
        kept = np.abs(out[2000:]).max() / np.abs(tone[2000:]).max()
        assert kept > 0.9, f"a 1kHz tone should survive an 80Hz high-pass, kept {kept:.2f}"

    def test_attenuates_a_tone_well_below_the_cutoff(self, realtime):
        rt = realtime()
        f = rt.HighPassFilter(cutoff_hz=80)
        t = np.arange(16000, dtype=np.float32) / 16000
        rumble = np.sin(2 * np.pi * 20 * t).astype(np.float32)

        out = f(rumble)
        kept = np.abs(out[2000:]).max() / np.abs(rumble[2000:]).max()
        assert kept < 0.2, f"20Hz rumble should be cut by an 80Hz high-pass, kept {kept:.2f}"

    def test_carries_state_across_chunks(self, realtime):
        """The filter is applied per chunk. If state did not carry, each chunk
        would restart the transient and produce a click at every boundary —
        audible, and noise the STT has to cope with.
        """
        rt = realtime()
        t = np.arange(4096, dtype=np.float32) / 16000
        signal = np.sin(2 * np.pi * 500 * t).astype(np.float32)

        whole = rt.HighPassFilter(cutoff_hz=80)(signal)

        chunked_filter = rt.HighPassFilter(cutoff_hz=80)
        chunked = np.concatenate([chunked_filter(c) for c in np.array_split(signal, 8)])

        assert np.allclose(whole, chunked, atol=1e-5), (
            "filtering in chunks must equal filtering the whole signal; "
            "a mismatch means filter state is not carried across chunks"
        )

    def test_empty_chunk_is_returned_untouched(self, realtime):
        rt = realtime()
        f = rt.HighPassFilter(cutoff_hz=80)
        empty = np.array([], dtype=np.float32)
        assert f(empty).size == 0

    def test_output_stays_float32(self, realtime):
        """scipy's sosfilt promotes to float64. Handing float64 to the STT path
        would double every buffer downstream."""
        rt = realtime()
        f = rt.HighPassFilter(cutoff_hz=80)
        out = f(np.ones(512, dtype=np.float32))
        assert out.dtype == np.float32


class TestResolveDevice:
    def test_none_means_the_system_default(self, realtime):
        rt = realtime()
        assert rt.resolve_device(None) is None

    def test_an_index_is_passed_through(self, realtime, make_devices):
        rt = realtime(devices=[make_devices("Mic"), make_devices("BlackHole 2ch")])
        assert rt.resolve_device("1") == 1

    def test_matches_a_name_substring_case_insensitively(self, realtime, make_devices):
        rt = realtime(
            devices=[make_devices("MacBook Pro Microphone"), make_devices("BlackHole 2ch")]
        )
        assert rt.resolve_device("blackhole") == 1
        assert rt.resolve_device("BLACKHOLE") == 1

    def test_ignores_output_only_devices(self, realtime, make_devices):
        """An output device with a matching name must not be selected — that is
        how you end up recording bit-exact silence."""
        rt = realtime(
            devices=[
                make_devices("BlackHole 2ch", in_ch=0),  # output side
                make_devices("BlackHole 2ch", in_ch=2),  # input side
            ]
        )
        assert rt.resolve_device("blackhole") == 1

    def test_no_match_exits_with_a_pointer_to_list_devices(self, realtime, make_devices):
        rt = realtime(devices=[make_devices("Mic")])
        with pytest.raises(SystemExit) as caught:
            rt.resolve_device("nonexistent")
        msg = str(caught.value)
        assert "nonexistent" in msg
        assert "--list-devices" in msg, "the error should tell the user how to see their options"

    def test_ambiguous_match_names_the_candidates(self, realtime, make_devices):
        """Two devices can genuinely share a substring. Silently taking the
        first would record the wrong one."""
        rt = realtime(devices=[make_devices("BlackHole 2ch"), make_devices("BlackHole 16ch")])
        with pytest.raises(SystemExit) as caught:
            rt.resolve_device("blackhole")
        msg = str(caught.value)
        assert "BlackHole 2ch" in msg and "BlackHole 16ch" in msg
        assert "index" in msg.lower(), "the error should say how to disambiguate"


class TestEmit:
    def test_emits_one_json_object_per_line(self, realtime, capsys):
        """The Go side reads this stream line by line, so one event must be
        exactly one line of JSON — a pretty-printed object would desynchronise
        the reader."""
        rt = realtime()
        rt.emit("delta", text="hello", speaker="Speaker 1")
        out = capsys.readouterr().out
        assert out.count("\n") == 1
        assert json.loads(out) == {"event": "delta", "text": "hello", "speaker": "Speaker 1"}

    def test_event_name_survives_a_field_called_event(self, realtime, capsys):
        rt = realtime()
        rt.emit("ready")
        assert json.loads(capsys.readouterr().out)["event"] == "ready"
