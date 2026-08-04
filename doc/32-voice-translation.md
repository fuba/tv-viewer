# VoiceTranslate live translation

TV Viewer can send the selected broadcast audio to VoiceTranslate, show a Japanese translation log over the picture, and replace WebRTC audio with translated speech. The player bar exposes an independent `翻訳 入/切` toggle.

## Data flow

```text
Mirakurun MPEG-TS
  -> native AAC decoder (48 kHz PCM)
     +-> FIR downmix/resample (16 kHz mono, 100 ms frames)
     |   -> configured VoiceTranslate WSS endpoint
     |      -> partial/final source and translation -> WebRTC DataChannel -> bounded chat overlay
     |      -> VOICEVOX WAV -> validate/decode/resample -> Opus
     +-> original Opus until VoiceTranslate is ready or after a connection failure
         translated Opus/silence while translation is active
```

The shared access token is read only by the backend. It is never included in browser JavaScript, a URL, logs, or DataChannel messages. The backend validates the WSS URL, Origin, token length, ready contract, input frame size, event size, Base64, RIFF/WAVE structure, PCM format, the 4 MiB WAV limit, and an eight-second decoded-speech limit. Upstream error text is replaced with allow-listed stages and fixed public messages before it reaches the browser.

Only WebRTC signaling connections with a non-empty browser `Origin` that matches `ALLOWED_ORIGINS` may enable translation. Keep the backend bound behind the trusted reverse proxy or private network as well: an Origin check is a browser boundary, not user authentication, because a custom non-browser client can forge the header.

The translation stream is part of a shared channel session. Multiple viewers on the same encoded channel therefore use the same translation, audio-mode, and ARIB-caption settings. As with audio mode changes, those settings cannot be changed while another viewer is attached.

## Configure the secret

Obtain the VoiceTranslate shared token through a secure channel and store it in a host file readable only by the operator:

```sh
umask 077
install -m 600 /dev/null /absolute/private/path/voicetranslate-token
# Write the token using the secret-management method for the host.
```

Set only the file path in `.env`:

```dotenv
VOICETRANSLATE_TOKEN_FILE=/absolute/private/path/voicetranslate-token
```

Start the production stack with the secret-only override:

```sh
docker compose -f docker-compose.yml -f compose.translation.yml up -d --build
```

For containerized development:

```sh
docker compose -f docker-compose.dev.yml -f compose.translation.yml up -d --build
```

The override mounts the token at `/run/secrets/voicetranslate-access-token`; it does not copy the token into an image or environment value. Set `VOICETRANSLATE_URL`, `VOICETRANSLATE_ORIGIN`, and any private browser hostname in `ALLOWED_ORIGINS` only in the untracked deployment `.env`. The repository intentionally has no private deployment-domain defaults.

The production backend runs as UID `10001` (development Compose uses the invoking UID/GID). Before starting the long-running service, verify that production Compose presents the secret as readable without printing it:

```sh
docker compose -f docker-compose.yml -f compose.translation.yml run --rm --no-deps \
  --entrypoint sh backend -c 'test "$(id -u)" = 10001 && test -r "$VOICETRANSLATE_TOKEN_FILE"'
```

If this fails, grant only UID `10001` read access with the host's ACL or secret manager. Do not make the host token world-readable.

For a backend process run directly on the host, set `VOICETRANSLATE_TOKEN_FILE` to the readable token path. `VOICETRANSLATE_ACCESS_TOKEN` is supported for constrained environments, but a token file is preferred because environment values are easier to expose through process inspection.

## Runtime behavior

- VoiceTranslate receives 16 kHz, mono, signed PCM16 little-endian in 3,200-byte frames paced every 100 ms.
- Interim translations overwrite one draft at the bottom of the log. Each final translation appends its source text, Japanese translation, and browser receipt time.
- The right-side translucent overlay is drawn inside the video frame without resizing the picture. It follows the newest entry until the viewer scrolls up, then offers a `最新へ` button instead of stealing the scroll position.
- Final entries are retained in memory per channel for one hour, including across translation off/on cycles. They are not persisted across a page reload. Defensive limits allow 2,000 entries per channel, 4,000 entries total, and 16 recent channels.
- Every translation event carries its backend channel and stream identity. The browser rejects delayed events from retired streams so a channel change cannot mix two histories.
- ARIB captions remain independently controlled by the existing subtitle toggle and may be shown with the translation log.
- While translation is ready, original audio is replaced with synthesized audio or silence on the same 48 kHz stereo WebRTC timeline. If WSS setup fails or disconnects, TV Viewer immediately falls back to broadcast audio.
- Each WebRTC subscriber has a bounded FIFO overflow reserve during encoder startup, capped at 32 MiB for video, 8 MiB for audio, and 2 MiB for each subtitle queue. The translation pipeline's initial A/V timestamp offset therefore cannot be mistaken for a stalled viewer, while one genuinely slow viewer cannot block the shared stream.
- Synthesized utterances stay in FIFO order and are never evicted to catch up. TV Viewer uses pitch-preserving WSOLA time compression as the unplayed backlog grows: 1.1x at 1.5 seconds, 1.2x at 4 seconds, 1.3x at 8 seconds, 1.5x at 12 seconds, 1.75x at 20 seconds, and 2x at 30 seconds. It returns to normal speed as the backlog clears.
- The speech queue applies backpressure at two minutes instead of discarding audio. If live PCM upload cannot keep pace for ten seconds, translation stops and broadcast audio resumes rather than silently omitting input frames.
- A late `speech_cancelled` event does not remove synthesized audio that TV Viewer has already received.
- The current VoiceTranslate service synthesizes speech only for Japanese output and uses Zundamon. The UI displays `VOICEVOX:ずんだもん` credit with translated captions.
- The upstream default allows one processing session. Capacity failure is reported in the player and does not expose the token.

## Latency limitation

VoiceTranslate finalizes an utterance after silence or a four-second split, then performs ASR, translation, and TTS. Its events contain no source timestamp. TV Viewer therefore plays translated speech as soon as it arrives and keeps the live video moving; it cannot provide exact lip synchronization without deliberately buffering the video by the full inference latency.

## Verification

Verify the configured service's unauthenticated liveness endpoint before deployment:

```sh
curl -fsS "${VOICETRANSLATE_ORIGIN}/healthz"
# {"status":"ok"}
```

`/healthz` covers gateway process liveness only. An authenticated end-to-end check requires the shared token and an available processing slot. Unit tests use a real local WebSocket handshake and exercise start authentication, binary PCM, event handling, FIR resampling, WAV validation, speech replacement, fallback, signaling settings, and bounded caption state.
