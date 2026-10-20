  import { streamErrorPresentation } from '/embed-status.js';
  import { SAMPLE_MS, createStallWatch, retryDelay } from '/embed-watchdog.js';

  const p = new URLSearchParams(location.search);
  const src = p.get('src');
  const label = p.get('label') || 'Kamera';
  const mode = p.get('mode') || 'mse,mp4,mjpeg';
  let muted = true;
  document.documentElement.style.setProperty('--fit', p.get('fit') === 'contain' ? 'contain' : 'cover');
  if (src) {
    const notice = document.createElement('div');
    notice.className = 'stream-error';
    notice.hidden = true;
    notice.setAttribute('role', 'status');
    notice.innerHTML = '<div class="stream-error-card"><div class="stream-error-mark" aria-hidden="true"></div><div class="stream-error-label"></div><div class="stream-error-title"></div><div class="stream-error-detail"></div></div>';
    notice.querySelector('.stream-error-label').textContent = label;
    document.body.appendChild(notice);

    // States for the viewer: connecting | playing | stalled | error.
    // camera-stream-ready stays for parents that only know the old protocol.
    let reported = '';
    const report = (state) => {
      if (state === reported) return;
      reported = state;
      window.parent?.postMessage({ type: 'camera-stream-state', src, state }, location.origin);
      if (state === 'playing') window.parent?.postMessage({ type: 'camera-stream-ready', src }, location.origin);
    };
    const showNotice = (raw) => {
      const presentation = streamErrorPresentation(raw);
      const title = notice.querySelector('.stream-error-title');
      const detail = notice.querySelector('.stream-error-detail');
      if (title.textContent !== presentation.title) title.textContent = presentation.title;
      if (detail.textContent !== presentation.detail) detail.textContent = presentation.detail;
      notice.hidden = false;
    };
    // Listen before the player loads, so the parent's first audio state is not lost.
    let el;
    let strip = () => undefined;
    window.addEventListener('message', (event) => {
      if (event.origin !== location.origin || event.data?.type !== 'camera-audio') return;
      muted = event.data.muted !== false;
      if (el) el.muted = muted;
      strip();
    });
    report('connecting');

    // go2rtc may be down while the kiosk starts; a cache-busting URL retries the import.
    for (let attempt = 0; !customElements.get('video-stream'); attempt++) {
      try {
        await import(attempt ? `/go2rtc/video-stream.js?retry=${attempt}` : '/go2rtc/video-stream.js');
        break;
      } catch {
        report('error');
        showNotice('');
        await new Promise((resolve) => setTimeout(resolve, retryDelay(attempt)));
      }
    }
    notice.hidden = true;

    const createPlayer = () => {
      const node = document.createElement('video-stream');
      node.background = true;            // Stream nicht pausieren, wenn iframe versteckt
      node.muted = muted;
      node.mode = mode;
      node.src = '/go2rtc/api/ws?src=' + encodeURIComponent(src);
      document.body.insertBefore(node, notice);
      return node;
    };
    // With background=true go2rtc keeps the socket open after removal, so close it explicitly.
    const destroyPlayer = (node) => {
      node.background = false;
      if (node.reconnectTID) clearTimeout(node.reconnectTID);
      try { node.ondisconnect?.(); } catch { /* already closed */ }
      node.remove();
    };

    el = createPlayer();
    const watch = createStallWatch(Date.now());
    const prepared = new WeakSet();

    const sample = () => {
      const v = el.video || el.querySelector?.('video');
      const playerMode = el.querySelector?.('.mode')?.textContent?.trim().toLowerCase() || '';
      const error = playerMode === 'error';
      const result = watch.sample(Date.now(), { currentTime: v?.currentTime ?? 0, poster: v?.poster ?? '', error });
      if (error) showNotice(el.querySelector?.('.status')?.textContent?.trim() || '');
      else if (result.state === 'playing') notice.hidden = true;
      report(result.state);
      if (result.restart) {
        destroyPlayer(el);
        el = createPlayer();
        watch.restarted(Date.now());
        strip();
      }
    };

    // Native Controls cross-browser zuverlässig abschalten (auch Firefox):
    strip = () => {
      const v = el.video || el.querySelector?.('video');
      if (v) {
        v.controls = false;
        v.muted = muted;
        v.removeAttribute('controls');
        v.style.pointerEvents = 'none';
        if (!muted) v.play?.().catch?.(() => undefined);
        if (!prepared.has(v)) {
          prepared.add(v);
          v.addEventListener('playing', sample);
          v.addEventListener('loadeddata', sample);
        }
      }
      for (const node of el.querySelectorAll?.('*') || []) {
        if (node.textContent?.trim() === 'MSE') node.style.display = 'none';
      }
    };
    strip();
    new MutationObserver(strip).observe(document.body, { childList: true, subtree: true });
    setInterval(sample, SAMPLE_MS);
  }
